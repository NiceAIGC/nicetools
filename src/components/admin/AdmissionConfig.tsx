import { useEffect, useState } from "react";
import {
  addToast,
  Button,
  Card,
  CardBody,
  Chip,
  Input,
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
  group: string;
  capability?: string;
  default_skip?: boolean;
  doc: { purpose: string; method: string; expect: string; ref: string };
}

interface SuiteInfo {
  id: string;
  name: string;
  default_model: string;
  capabilities: { key: string; label: string }[];
  cases: SuiteCase[];
}

interface CaseCfg {
  case_id: string;
  mode: "" | "skip" | "exempt";
  level: "" | "P0" | "P1" | "P2" | "P3";
}

const listSuites = () => toolApi<SuiteInfo[]>(TOOL, "/suites");
const listCaseCfg = () => toolApi<CaseCfg[]>(TOOL, "/config");
const saveCaseCfg = (suite: string, cfgs: CaseCfg[]) => toolApi(TOOL, "/config", json("PUT", { suite, cfgs }));

// 准入测试用例永久配置（管理员级默认）：级别改写 + 默认测试/不测。
// 任务创建时的自定义优先级高于这里（规范 §6 三层模型）。
export default function AdmissionConfig() {
  const [suite, setSuite] = useState<SuiteInfo | null>(null);
  const [cfg, setCfg] = useState<Record<string, CaseCfg>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");
  const [bulkLevel, setBulkLevel] = useState("");

  useEffect(() => {
    Promise.all([listSuites(), listCaseCfg()])
      .then(([suites, cfgs]) => {
        setSuite(suites[0] ?? null);
        setSelected(new Set());
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

  const bulkMode = (mode: CaseCfg["mode"]) => setCfg((prev) => {
    const next = { ...prev };
    for (const id of selected) {
      const base = next[id] ?? { case_id: id, mode: "", level: "" };
      next[id] = { ...base, mode };
    }
    return next;
  });

  const resetSelected = () => setCfg((prev) => {
    const next = { ...prev };
    for (const id of selected) delete next[id];
    return next;
  });

  const applyBulkLevel = () => setCfg((prev) => {
    const next = { ...prev };
    for (const id of selected) {
      const base = next[id] ?? { case_id: id, mode: "", level: "" };
      next[id] = { ...base, level: bulkLevel as CaseCfg["level"] };
    }
    return next;
  });

  const allIDs = suite?.cases.map((c) => c.id) ?? [];

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
      <div className="flex flex-wrap items-center gap-2 rounded-large border border-default-200 bg-default-50 px-3 py-2">
        <Input size="sm" className="w-64" placeholder="搜索用例 ID / 名称 / 说明" value={query} onValueChange={setQuery} isClearable />
        <Button size="sm" variant="flat" onPress={() => setSelected(new Set(allIDs))}>全选</Button>
        <Button size="sm" variant="flat" onPress={() => setSelected(new Set())}>全不选</Button>
        <Button size="sm" variant="flat" onPress={() => setSelected(new Set(allIDs.filter((id) => !selected.has(id))))}>反选</Button>
        <span className="mx-1 h-5 w-px bg-default-200" />
        <span className="text-xs text-default-500">已选 {selected.size} 项，批量：</span>
        <Button size="sm" color="success" variant="flat" isDisabled={selected.size === 0} onPress={() => bulkMode("")}>启用</Button>
        <Button size="sm" color="warning" variant="flat" isDisabled={selected.size === 0} onPress={() => bulkMode("skip")}>跳过</Button>
        <Button size="sm" variant="light" isDisabled={selected.size === 0} onPress={resetSelected}>恢复默认</Button>
        <span className="mx-1 h-5 w-px bg-default-200" />
        <Select aria-label="批量级别" size="sm" className="w-32" selectedKeys={[bulkLevel || "default"]} onChange={(e) => setBulkLevel(e.target.value === "default" ? "" : e.target.value)}>
          <SelectItem key="default">默认级别</SelectItem>
          <SelectItem key="P0">P0 否决</SelectItem>
          <SelectItem key="P1">P1 必过</SelectItem>
          <SelectItem key="P2">P2 记录</SelectItem>
          <SelectItem key="P3">P3 可选</SelectItem>
        </Select>
        <Button size="sm" variant="flat" isDisabled={selected.size === 0} onPress={applyBulkLevel}>应用级别</Button>
      </div>
      {[
        ["gate", "前置闸门"],
        ["format", "格式与协议"],
        ["latency", "性能基准"],
        ["cache", "缓存"],
      ].map(([group, label]) => (
        <Card key={group} shadow="none" className="border border-default-200">
          <CardBody className="gap-2">
            {(() => {
              const groupCases = suite.cases.filter((c) => c.group === group).filter((c) => {
                const q = query.trim().toLowerCase();
                return !q || `${c.id} ${c.name} ${c.doc.purpose}`.toLowerCase().includes(q);
              });
              const groupIDs = new Set(groupCases.map((c) => c.id));
              const groupSelected = new Set([...selected].filter((id) => groupIDs.has(id)));
              return (
                <>
            <p className="text-sm font-semibold">{label}（{groupCases.length} 项）</p>
            <div className="max-h-[60vh] overflow-x-auto overflow-y-auto">
            <Table
              aria-label={label}
              removeWrapper
              selectionMode="multiple"
              selectedKeys={groupSelected}
              onSelectionChange={(keys) => {
                const next = new Set(selected);
                for (const id of groupIDs) next.delete(id);
                if (keys === "all") {
                  for (const id of groupIDs) next.add(id);
                } else {
                  for (const key of keys) next.add(String(key));
                }
                setSelected(next);
              }}
            >
              <TableHeader>
                <TableColumn>用例</TableColumn>
                <TableColumn>说明</TableColumn>
                <TableColumn className="w-36">级别</TableColumn>
                <TableColumn className="w-28">测试</TableColumn>
              </TableHeader>
              <TableBody>
                {groupCases.map((cs) => {
                  const current = cfg[cs.id] ?? { case_id: cs.id, mode: "", level: "" };
                  return (
                    <TableRow key={cs.id}>
                      <TableCell className="whitespace-nowrap">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-default-400">{cs.id}</span>
                          <span className="text-sm">{cs.name}</span>
                          {(cs.level === "P0" || cs.level === "P1") && (
                            <Chip size="sm" variant="bordered" color={cs.level === "P0" ? "danger" : "default"}>{cs.level}</Chip>
                          )}
                          {cs.default_skip && <Chip size="sm" color="warning" variant="flat">默认跳过</Chip>}
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
                        <Switch
                          aria-label={`${cs.name} 测试`}
                          size="sm"
                          isSelected={current.mode !== "skip"}
                          onValueChange={(on) => update(cs.id, { mode: on ? "" : "skip" })}
                        >
                          测试
                        </Switch>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            </div>
                </>
              );
            })()}
          </CardBody>
        </Card>
      ))}
    </div>
  );
}
