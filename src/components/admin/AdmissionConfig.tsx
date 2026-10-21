import { useEffect, useState } from "react";
import {
  addToast,
  Button,
  Card,
  CardBody,
  Chip,
  Select,
  SelectItem,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
  Switch,
} from "@heroui/react";

import { json, toolApi } from "../../api/client";

// 与 tools-impl/llm-admission/api.ts 同构，但管理后台不能引用 tools-impl
// （懒加载隔离：工具代码只进工具 chunk），这里内联最小接口集。
const TOOL = "llm-admission";

interface SuiteCase {
  id: string;
  name: string;
  level: string;
  capability?: string;
  doc: { purpose: string; method: string; expect: string; ref: string };
}

interface SuiteInfo {
  id: string;
  name: string;
  default_model: string;
  capabilities: { key: string; label: string }[];
  tiers: { id: string; name: string; hint: string; cases: SuiteCase[] }[];
}

interface CaseCfg {
  case_id: string;
  mode: "" | "skip" | "exempt";
  level: "" | "P0" | "P1" | "P2" | "P3";
}

const listSuites = () => toolApi<SuiteInfo[]>(TOOL, "/suites");
const listCaseCfg = () => toolApi<CaseCfg[]>(TOOL, "/config");
const saveCaseCfg = (suite: string, cfgs: CaseCfg[]) => toolApi(TOOL, "/config", json("PUT", { suite, cfgs }));

// 准入测试用例永久配置（管理员级默认）：级别改写 + 默认不测/豁免。
// 任务创建时的自定义优先级高于这里（规范 §6 三层模型）。
export default function AdmissionConfig() {
  const [suite, setSuite] = useState<SuiteInfo | null>(null);
  const [cfg, setCfg] = useState<Record<string, CaseCfg>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    Promise.all([listSuites(), listCaseCfg()])
      .then(([suites, cfgs]) => {
        setSuite(suites[0] ?? null);
        const map: Record<string, CaseCfg> = {};
        for (const item of cfgs) {
          map[item.case_id] = { case_id: item.case_id, mode: item.mode, level: item.level };
        }
        setCfg(map);
      })
      .catch((e) => addToast({ title: e.message, color: "danger" }))
      .finally(() => setLoading(false));
  }, []);

  const update = (caseId: string, patch: Partial<CaseCfg>) =>
    setCfg((prev) => {
      const base = prev[caseId] ?? { case_id: caseId, mode: "", level: "" };
      return { ...prev, [caseId]: { ...base, ...patch } };
    });

  const dirtyCount = Object.values(cfg).filter((c) => c.mode || c.level).length;

  const save = async () => {
    if (!suite) return;
    setSaving(true);
    try {
      await saveCaseCfg(suite.id, Object.values(cfg).filter((c) => c.mode || c.level));
      addToast({ title: "配置已保存，对之后的新任务生效" });
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="flex justify-center py-16"><Spinner /></div>;
  }
  if (!suite) {
    return <p className="py-8 text-sm text-default-500">没有可用的测试标准（llm-admission 未启用或无权限）。</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm text-default-500">{suite.name} · 已配置 {dirtyCount} 项 · 任务级自定义优先于此处默认值</p>
        <Button color="primary" size="sm" className="ml-auto" isLoading={saving} onPress={save}>
          保存配置
        </Button>
      </div>
      {suite.tiers.map((tier) => (
        <Card key={tier.id} shadow="none" className="border border-default-200">
          <CardBody className="gap-2">
            <p className="text-sm font-semibold">{tier.name}</p>
            <Table aria-label={tier.name} removeWrapper>
              <TableHeader>
                <TableColumn>用例</TableColumn>
                <TableColumn>说明</TableColumn>
                <TableColumn className="w-36">级别</TableColumn>
                <TableColumn className="w-44">启用 / 豁免</TableColumn>
              </TableHeader>
              <TableBody>
                {tier.cases.map((cs) => {
                  const current = cfg[cs.id] ?? { case_id: cs.id, mode: "", level: "" };
                  return (
                    <TableRow key={cs.id}>
                      <TableCell className="whitespace-nowrap">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-default-400">{tier.id}-{cs.id}</span>
                          <span className="text-sm">{cs.name}</span>
                          {(cs.level === "P0" || cs.level === "P1") && (
                            <Chip size="sm" variant="bordered" color={cs.level === "P0" ? "danger" : "default"}>{cs.level}</Chip>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="max-w-md"><span className="line-clamp-2 text-xs text-default-400">{cs.doc.purpose}</span></TableCell>
                      <TableCell>
                        <Select
                          aria-label={`${cs.name} 级别`}
                          size="sm"
                          selectedKeys={[current.level || "default"]}
                          onChange={(e) => update(cs.id, { level: e.target.value === "default" ? "" : (e.target.value as CaseCfg["level"]) })}
                        >
                          <SelectItem key="default">默认（{cs.level === "gate" ? "闸门" : cs.level}）</SelectItem>
                          <SelectItem key="P0">P0 否决</SelectItem>
                          <SelectItem key="P1">P1 必过</SelectItem>
                          <SelectItem key="P2">P2 记录</SelectItem>
                          <SelectItem key="P3">P3 可选</SelectItem>
                        </Select>
                      </TableCell>
                      <TableCell>
                        {/* 关闭即永久不测（mode=skip）；豁免只在启用时有意义 */}
                        <div className="flex items-center gap-4">
                          <Switch
                            aria-label={`${cs.name} 启用`}
                            size="sm"
                            isSelected={current.mode !== "skip"}
                            onValueChange={(on) => update(cs.id, { mode: on ? (current.mode === "skip" ? "" : current.mode) : "skip" })}
                          >
                            启用
                          </Switch>
                          <Switch
                            aria-label={`${cs.name} 豁免`}
                            size="sm"
                            isDisabled={current.mode === "skip"}
                            isSelected={current.mode === "exempt"}
                            onValueChange={(on) => update(cs.id, { mode: on ? "exempt" : "" })}
                          >
                            豁免
                          </Switch>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardBody>
        </Card>
      ))}
    </div>
  );
}
