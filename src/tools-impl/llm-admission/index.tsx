import { useCallback, useEffect, useState } from "react";
import {
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
  const [tier, setTier] = useState("F");
  const [declarations, setDeclarations] = useState<Record<string, boolean>>({});
  const [submitting, setSubmitting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [exportChannel, setExportChannel] = useState("");

  const suite = suites.find((s) => s.id === suiteId) ?? suites[0];
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
      const run = await createRun({ base_url: baseURL, api_key: apiKey, model, channel, suite: suite.id, tier, declarations });
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
                <Chip size="sm" color={detail.run.verdict === "通过" ? "success" : "danger"} variant="solid">
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
                <Button size="sm" variant="bordered" onPress={() => exportRun(detail.run.id, "full", "md", exportChannel || undefined)}>详细 MD</Button>
                <Button size="sm" variant="bordered" onPress={() => exportRun(detail.run.id, "full", "json", exportChannel || undefined)}>详细 JSON</Button>
                <Button size="sm" variant="bordered" onPress={() => exportRun(detail.run.id, "masked", "md", exportChannel || undefined)}>脱敏 MD</Button>
                <Button size="sm" variant="bordered" onPress={() => exportRun(detail.run.id, "masked", "json", exportChannel || undefined)}>脱敏 JSON</Button>
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
                <div key={row.id} className="flex items-center gap-2 py-1 text-sm">
                  <Chip size="sm" variant="flat" color={caseColor[row.status]} className="w-16 justify-center">
                    {caseLabel[row.status]}
                  </Chip>
                  <span className="w-20 shrink-0 font-mono text-xs text-default-400">{row.tier}-{row.case_id}</span>
                  <span className="shrink-0">{row.name}</span>
                  {row.level === "P0" && <Chip size="sm" color="danger" variant="bordered">P0</Chip>}
                  <span className="min-w-0 flex-1 truncate text-xs text-default-400">{row.summary}</span>
                  {row.duration_ms > 0 && <span className="text-xs text-default-300">{(row.duration_ms / 1000).toFixed(1)}s</span>}
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
                <Button size="sm" variant="bordered" onPress={() => exportMerged([...selected], "full", "md", exportChannel || undefined)}>合并详细 MD</Button>
                <Button size="sm" variant="bordered" onPress={() => exportMerged([...selected], "full", "json", exportChannel || undefined)}>合并详细 JSON</Button>
                <Button size="sm" variant="bordered" onPress={() => exportMerged([...selected], "masked", "md", exportChannel || undefined)}>合并脱敏 MD</Button>
                <Button size="sm" variant="bordered" onPress={() => exportMerged([...selected], "masked", "json", exportChannel || undefined)}>合并脱敏 JSON</Button>
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
