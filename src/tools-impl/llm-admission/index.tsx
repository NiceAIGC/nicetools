import { useCallback, useEffect, useState } from "react";
import {
  Accordion,
  AccordionItem,
  addToast,
  Button,
  Card,
  CardBody,
  Chip,
  Divider,
  Input,
  Progress,
  Select,
  SelectItem,
  Spinner,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from "@heroui/react";

import {
  createRun,
  exportMerged,
  exportRun,
  listRuns,
  listSuites,
  pauseRun,
  resumeRun,
  abortRun,
  runDetail,
  type CaseRow,
  type ExportDetail,
  type ExportFormat,
  type ExportType,
  type Run,
  type SuiteInfo,
} from "./api";

// 目标信息本地保存（含密钥，仅存本机浏览器），刷新/重开不丢。
const TARGET_KEY = "llm-admission:target";

interface SavedTarget {
  base_url: string;
  api_key: string;
  model: string;
  channel: string;
}

function loadTarget(): SavedTarget {
  try {
    return { base_url: "", api_key: "", model: "", channel: "", ...JSON.parse(localStorage.getItem(TARGET_KEY) ?? "{}") };
  } catch {
    return { base_url: "", api_key: "", model: "", channel: "" };
  }
}

const statusColor: Record<string, "primary" | "warning" | "success" | "danger" | "default"> = {
  running: "primary",
  pausing: "warning",
  paused: "warning",
  succeeded: "success",
  failed: "danger",
  aborted: "default",
};

const statusLabel: Record<string, string> = {
  running: "运行中",
  pausing: "暂停中",
  paused: "已暂停",
  succeeded: "已完成",
  failed: "未通过",
  aborted: "已终止",
};

const caseColor: Record<string, "success" | "danger" | "primary" | "default" | "warning"> = {
  passed: "success",
  failed: "danger",
  running: "primary",
  pending: "default",
  exempt: "warning",
  skipped: "warning",
};

const caseLabel: Record<string, string> = {
  passed: "通过",
  failed: "失败",
  running: "运行中",
  pending: "待执行",
  exempt: "豁免",
  skipped: "跳过",
};

const levelNote: Record<string, string> = {
  P0: "P0 否决项",
  P1: "P1 必过项",
  P2: "P2 记录项",
  P3: "P3 可选项",
  gate: "前置闸门",
  required: "P1 必过项",
  record: "P2 记录项",
  optional: "P3 可选项",
};

// 用例行的展开详情：实测摘要 + 用例说明 + 缺陷。
function CaseDetail({ row, doc }: { row: CaseRow; doc?: { purpose: string; method: string; expect: string; ref: string } }) {
  let defects: { level: string; expect: string; actual: string; attribution: string }[] = [];
  try {
    defects = JSON.parse(row.defects || "[]");
  } catch { /* 忽略坏数据 */ }
  return (
    <div className="ml-24 flex flex-col gap-1 border-l-2 border-default-200 py-1 pl-3 text-xs text-default-500">
      <p><span className="text-default-400">级别：</span>{levelNote[row.level] ?? row.level}</p>
      {doc?.purpose && <p><span className="text-default-400">为什么测：</span>{doc.purpose}</p>}
      {doc?.method && <p><span className="text-default-400">怎么测：</span>{doc.method}</p>}
      {doc?.expect && <p><span className="text-default-400">预期：</span>{doc.expect}{doc.ref ? `（${doc.ref}）` : ""}</p>}
      {row.summary && <p><span className="text-default-400">实测：</span>{row.summary}</p>}
      {defects.map((d, i) => (
        <p key={i}><span className="text-default-400">缺陷[{d.level}]：</span>期望 {d.expect}；实测 {d.actual}；归因 {d.attribution}</p>
      ))}
    </div>
  );
}

export default function LlmAdmission() {
  const [suites, setSuites] = useState<SuiteInfo[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [detail, setDetail] = useState<{ run: Run; cases: CaseRow[] } | null>(null);
  const [loading, setLoading] = useState(true);

  const savedTarget = useState(loadTarget)[0];
  const [baseURL, setBaseURL] = useState(savedTarget.base_url);
  const [apiKey, setAPIKey] = useState(savedTarget.api_key);
  const [channel, setChannel] = useState(savedTarget.channel);
  const [suiteId, setSuiteId] = useState("");
  const [model, setModel] = useState(savedTarget.model);
  const [tier, setTier] = useState("B");
  const [declarations, setDeclarations] = useState<Record<string, boolean>>({});
  const [overrides, setOverrides] = useState<Record<string, "skip" | "exempt">>({});
  const [submitting, setSubmitting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [exportChannel, setExportChannel] = useState("");
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [exDetail, setExDetail] = useState<ExportDetail>("full");
  const [exType, setExType] = useState<ExportType>("full");
  const [exFormat, setExFormat] = useState<ExportFormat>("pdf");

  const suite = suites.find((s) => s.id === suiteId) ?? suites[0];
  // 目标档位及其前置档位的全部用例（如选 M 则显示 F+M）。
  const tierOrder = suite ? suite.tiers.map((t) => t.id) : [];
  const activeTiers = suite ? suite.tiers.slice(0, tierOrder.indexOf(tier) + 1) : [];
  const overrideCount = Object.keys(overrides).length;
  const active = runs.find((r) => ["running", "pausing", "paused"].includes(r.status));
  const detailId = detail?.run.id ?? null;

  const refreshRuns = useCallback(async () => {
    try {
      setRuns(await listRuns());
    } catch {
      // 列表失败不打断页面
    }
  }, []);

  const refreshDetail = useCallback(async (id: string) => {
    try {
      setDetail(await runDetail(id));
    } catch {
      // 同上
    }
  }, []);

  useEffect(() => {
    listSuites()
      .then((list) => {
        setSuites(list);
        if (list[0]) {
          setSuiteId(list[0].id);
          setModel((m) => m || list[0].default_model);
          setDeclarations(Object.fromEntries(list[0].capabilities.map((c) => [c.key, false])));
        }
      })
      .catch((e) => addToast({ title: e.message, color: "danger" }))
      .finally(() => setLoading(false));
    refreshRuns();
  }, [refreshRuns]);

  // 有活跃任务或正在查看详情时轮询。
  useEffect(() => {
    if (!active && !detailId) return;
    const timer = setInterval(() => {
      refreshRuns();
      if (detailId) refreshDetail(detailId);
    }, 2500);
    return () => clearInterval(timer);
  }, [active?.id, detailId, refreshRuns, refreshDetail]);

  const submit = async () => {
    if (!suite) return;
    setSubmitting(true);
    try {
      const run = await createRun({ base_url: baseURL, api_key: apiKey, model, channel, suite: suite.id, tier, declarations, case_overrides: overrides });
      localStorage.setItem(TARGET_KEY, JSON.stringify({ base_url: baseURL, api_key: apiKey, model, channel }));
      addToast({ title: "测试已启动" });
      await refreshRuns();
      setDetail({ run, cases: [] });
      refreshDetail(run.id);
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    } finally {
      setSubmitting(false);
    }
  };

  const act = async (fn: (id: string) => Promise<unknown>, id: string, done: string) => {
    try {
      await fn(id);
      addToast({ title: done });
      await refreshRuns();
      if (detail?.run.id === id) await refreshDetail(id);
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    }
  };

  if (loading) {
    return <div className="flex justify-center py-16"><Spinner /></div>;
  }
  if (!suite) {
    return <p className="py-16 text-center text-sm text-default-500">没有可用的测试标准</p>;
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardBody className="gap-4">
          <div>
            <h2 className="text-lg font-semibold">新建测试</h2>
            <p className="mt-1 text-xs text-default-400">{suite.name} · 三档顺序执行，失败不终止，最终汇总裁决</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="渠道名称" placeholder="如：张三的 K3 渠道" value={channel} onValueChange={setChannel} description="写入报告，导出时可再改" />
            <Input label="网关地址" placeholder="https://api.example.com/v1" value={baseURL} onValueChange={setBaseURL} isRequired />
            <Input label="密钥" type="password" placeholder="sk-…（仅存本机浏览器）" value={apiKey} onValueChange={setAPIKey} isRequired />
            <Input label="模型名称" value={model} onValueChange={setModel} isRequired />
            <Select
              label="测试标准"
              selectedKeys={[suite.id]}
              onChange={(e) => {
                const next = suites.find((s) => s.id === e.target.value);
                if (!next) return;
                setSuiteId(next.id);
                setModel(next.default_model);
                setTier(next.tiers[0]?.id ?? "F");
                setDeclarations(Object.fromEntries(next.capabilities.map((c) => [c.key, false])));
              }}
            >
              {suites.map((s) => (
                <SelectItem key={s.id}>{s.name}</SelectItem>
              ))}
            </Select>
            <Select label="目标档位" selectedKeys={[tier]} onChange={(e) => setTier(e.target.value)}>
              {suite.tiers.map((t) => (
                <SelectItem key={t.id} description={t.hint}>
                  {t.name}
                </SelectItem>
              ))}
            </Select>
          </div>
          <div className="flex flex-col gap-2">
            <span className="text-sm text-default-500">能力验证（勾选 = 验证该能力，对应用例必须全过；不勾 = 豁免不测）</span>
            <div className="flex flex-wrap gap-x-6 gap-y-2">
              {suite.capabilities.map((cap) => (
                <Switch
                  key={cap.key}
                  size="sm"
                  isSelected={declarations[cap.key] ?? false}
                  onValueChange={(v) => setDeclarations((d) => ({ ...d, [cap.key]: v }))}
                >
                  {cap.label}
                </Switch>
              ))}
            </div>
          </div>
          <Accordion isCompact>
            <AccordionItem
              key="overrides"
              aria-label="用例自定义"
              title={<span className="text-sm">用例自定义{overrideCount > 0 ? `（已调整 ${overrideCount} 项）` : ""}</span>}
              subtitle={<span className="text-xs text-default-400">逐项选择跳过（不测）或豁免（测但不计入失败）</span>}
            >
              <div className="flex max-h-72 flex-col gap-2 overflow-y-auto pb-2">
                {activeTiers.map((t) => (
                  <div key={t.id} className="flex flex-col gap-1">
                    <span className="pt-1 text-xs font-semibold text-default-500">{t.name}</span>
                    {t.cases.map((cs) => (
                      <div key={cs.id} className="flex items-center gap-2 text-sm">
                        <span className="w-16 shrink-0 font-mono text-xs text-default-400">{t.id}-{cs.id}</span>
                        <span className="min-w-0 flex-1 truncate" title={cs.doc.purpose}>{cs.name}</span>
                        {(cs.level === "P0" || cs.level === "required") && (
                          <Chip size="sm" variant="bordered" color={cs.level === "P0" ? "danger" : "default"}>{cs.level === "P0" ? "P0" : "必过"}</Chip>
                        )}
                        <Select
                          aria-label={`${cs.name} 执行方式`}
                          size="sm"
                          className="w-24"
                          selectedKeys={[overrides[cs.id] ?? "default"]}
                          onChange={(e) => {
                            const value = e.target.value as "skip" | "exempt" | "default";
                            setOverrides((prev) => {
                              const next = { ...prev };
                              if (value === "default") delete next[cs.id];
                              else next[cs.id] = value;
                              return next;
                            });
                          }}
                        >
                          <SelectItem key="default">默认</SelectItem>
                          <SelectItem key="skip">不测</SelectItem>
                          <SelectItem key="exempt">豁免</SelectItem>
                        </Select>
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            </AccordionItem>
          </Accordion>
          <div className="flex items-center gap-3">
            <Button
              color="primary"
              isLoading={submitting}
              isDisabled={!baseURL || !apiKey || !model || !!active}
              onPress={submit}
            >
              开始测试
            </Button>
            {active && <span className="text-xs text-warning">已有任务进行中，同一时间只能跑一个测试</span>}
          </div>
        </CardBody>
      </Card>

      {detail && (
        <Card>
          <CardBody className="gap-4">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="text-lg font-semibold">任务详情</h2>
              <Chip size="sm" color={statusColor[detail.run.status]} variant="flat">
                {statusLabel[detail.run.status]}
              </Chip>
              {detail.run.verdict && (
                <Chip size="sm" color={detail.run.verdict.startsWith("0 项") ? "success" : "danger"} variant="solid">
                  {detail.run.verdict}
                </Chip>
              )}
              <span className="text-xs text-default-400">{detail.run.model} @ {detail.run.base_url}</span>
              <div className="ml-auto flex gap-2">
                {detail.run.status === "running" && (
                  <Button size="sm" variant="flat" onPress={() => act(pauseRun, detail.run.id, "已请求暂停")}>暂停</Button>
                )}
                {detail.run.status === "paused" && (
                  <Button size="sm" color="primary" variant="flat" onPress={() => act(resumeRun, detail.run.id, "已继续")}>继续</Button>
                )}
                {["running", "pausing", "paused"].includes(detail.run.status) && (
                  <Button size="sm" color="danger" variant="light" onPress={() => act(abortRun, detail.run.id, "已终止")}>终止</Button>
                )}
                <Select aria-label="内容" size="sm" className="w-28" selectedKeys={[exDetail]} onChange={(e) => setExDetail(e.target.value as ExportDetail)}>
                  <SelectItem key="full">详细版</SelectItem>
                  <SelectItem key="brief">精简版</SelectItem>
                </Select>
                <Select aria-label="脱敏" size="sm" className="w-24" selectedKeys={[exType]} onChange={(e) => setExType(e.target.value as ExportType)}>
                  <SelectItem key="full">不脱敏</SelectItem>
                  <SelectItem key="masked">脱敏</SelectItem>
                </Select>
                <Select aria-label="格式" size="sm" className="w-24" selectedKeys={[exFormat]} onChange={(e) => setExFormat(e.target.value as ExportFormat)}>
                  <SelectItem key="pdf">PDF</SelectItem>
                  <SelectItem key="md">MD</SelectItem>
                  <SelectItem key="json">JSON</SelectItem>
                </Select>
                <Button size="sm" color="primary" variant="flat" onPress={() => exportRun(detail.run.id, exType, exFormat, exDetail, exportChannel || undefined)}>导出</Button>
              </div>
            </div>
            <Progress
              size="sm"
              value={detail.run.total_cases ? (detail.run.done_cases / detail.run.total_cases) * 100 : 0}
              label={`进度 ${detail.run.done_cases}/${detail.run.total_cases}`}
            />
            {detail.run.stop_reason && <p className="text-xs text-default-400">{detail.run.stop_reason}</p>}
            <Divider />
            <div className="flex flex-col gap-1">
              {detail.cases.map((row) => (
                <div key={row.id}>
                  <div
                    className="flex cursor-pointer items-center gap-2 py-1 text-sm hover:bg-default-50"
                    onClick={() =>
                      setExpanded((prev) => {
                        const next = new Set(prev);
                        if (next.has(row.id)) next.delete(row.id);
                        else next.add(row.id);
                        return next;
                      })
                    }
                  >
                    <Chip size="sm" variant="flat" color={caseColor[row.status]} className="w-16 justify-center">
                      {caseLabel[row.status]}
                    </Chip>
                    <span className="w-20 shrink-0 font-mono text-xs text-default-400">{row.tier}-{row.case_id}</span>
                    <span className="shrink-0">{row.name}</span>
                    <Chip size="sm" variant="bordered" color={row.level === "P0" ? "danger" : "default"}>
                      {levelNote[row.level] ?? row.level}
                    </Chip>
                    <span className="min-w-0 flex-1 truncate text-xs text-default-400">{row.summary}</span>
                    {row.duration_ms > 0 && <span className="text-xs text-default-300">{(row.duration_ms / 1000).toFixed(1)}s</span>}
                    <span className="text-xs text-default-400">{expanded.has(row.id) ? "▲" : "▼"}</span>
                  </div>
                  {expanded.has(row.id) && (
                    <CaseDetail
                      row={row}
                      doc={suite.tiers.find((t) => t.id === row.tier)?.cases.find((cs) => cs.id === row.case_id)?.doc}
                    />
                  )}
                </div>
              ))}
            </div>
          </CardBody>
        </Card>
      )}

      <Card>
        <CardBody className="gap-3">
          <h2 className="text-lg font-semibold">历史任务</h2>
          <div className="flex flex-wrap items-end gap-3">
            <Input
              label="报告渠道名称"
              size="sm"
              className="w-56"
              placeholder="默认取任务上的渠道名"
              value={exportChannel}
              onValueChange={setExportChannel}
            />
            {selected.size > 0 && (
              <>
                <span className="text-xs text-default-400">已选 {selected.size} 个任务</span>
                <Button size="sm" color="primary" variant="flat" onPress={() => exportMerged([...selected], exType, exFormat, exDetail, exportChannel || undefined)}>合并导出</Button>
              </>
            )}
          </div>
          <Table
            aria-label="历史任务"
            removeWrapper
            selectionMode="multiple"
            selectedKeys={selected}
            onSelectionChange={(keys) => setSelected(keys === "all" ? new Set(runs.map((r) => r.id)) : new Set([...keys].map(String)))}
          >
            <TableHeader>
              <TableColumn>时间</TableColumn>
              <TableColumn>渠道</TableColumn>
              <TableColumn>模型</TableColumn>
              <TableColumn>档位</TableColumn>
              <TableColumn>进度</TableColumn>
              <TableColumn>状态</TableColumn>
              <TableColumn>操作</TableColumn>
            </TableHeader>
            <TableBody emptyContent="还没有测试记录">
              {runs.map((run) => (
                <TableRow key={run.id}>
                  <TableCell className="whitespace-nowrap text-xs">{new Date(run.created_at).toLocaleString()}</TableCell>
                  <TableCell className="max-w-32 truncate">{run.channel || "—"}</TableCell>
                  <TableCell className="max-w-40 truncate">{run.model}</TableCell>
                  <TableCell>{run.tier}</TableCell>
                  <TableCell>{run.done_cases}/{run.total_cases}</TableCell>
                  <TableCell>
                    <Chip size="sm" variant="flat" color={statusColor[run.status]}>
                      {run.verdict || statusLabel[run.status]}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    <Button size="sm" variant="light" onPress={() => { setDetail({ run, cases: [] }); refreshDetail(run.id); }}>
                      查看
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardBody>
      </Card>
    </div>
  );
}
