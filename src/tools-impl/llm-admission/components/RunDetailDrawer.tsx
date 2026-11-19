import { useCallback, useEffect, useMemo, useState } from "react";
import {
  addToast,
  Button,
  Chip,
  Divider,
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerHeader,
  Input,
  Progress,
  Select,
  SelectItem,
  Spinner,
} from "@heroui/react";

import {
  abortRun,
  exportFailedEvidence,
  exportRun,
  pauseRun,
  resumeRun,
  runDetail,
  type CaseRow,
  type ExportDetail,
  type ExportFormat,
  type ExportType,
  type Run,
} from "../api";
import EvidenceDetail from "../EvidenceDetail";
import {
  caseColor,
  caseLabel,
  displayTime,
  gradeColor,
  gradeOf,
  hostOf,
  levelNote,
  statusColor,
  statusLabel,
} from "../lib/report";

export default function RunDetailDrawer({
  run,
  onClose,
  onChanged,
}: {
  run: Run | null;
  onClose: () => void;
  onChanged: () => void;
}) {
  const [detail, setDetail] = useState<{ run: Run; cases: CaseRow[] } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [caseQuery, setCaseQuery] = useState("");
  const [onlyFailed, setOnlyFailed] = useState(false);
  const [exportDetail, setExportDetail] = useState<ExportDetail>("brief");
  const [exportType, setExportType] = useState<ExportType>("masked");
  const [exportFormat, setExportFormat] = useState<ExportFormat>("pdf");
  const [watermark, setWatermark] = useState(true);

  const refresh = useCallback(async (id: string) => {
    try {
      setLoading(true);
      setDetail(await runDetail(id));
      setError("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setExpanded(new Set());
    setCaseQuery("");
    setOnlyFailed(false);
    if (!run) {
      setDetail(null);
      return;
    }
    refresh(run.id);
  }, [run, refresh]);

  useEffect(() => {
    if (!run || !["queued", "running", "pausing"].includes(run.status)) return;
    const timer = setInterval(() => refresh(run.id), 3000);
    return () => clearInterval(timer);
  }, [run, refresh]);

  const act = async (fn: (id: string) => Promise<unknown>, id: string, message: string) => {
    try {
      await fn(id);
      addToast({ title: message });
      await refresh(id);
      onChanged();
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    }
  };

  const cases = useMemo(() => {
    if (!detail) return [];
    const q = caseQuery.trim().toLowerCase();
    return detail.cases.filter((row) => {
      if (onlyFailed && row.status !== "failed") return false;
      if (!q) return true;
      return `${row.case_id} ${row.name} ${row.summary}`.toLowerCase().includes(q);
    });
  }, [detail, caseQuery, onlyFailed]);

  const failed = detail?.cases.filter((row) => row.status === "failed").length ?? 0;
  const counts = useMemo(() => {
    const by = { passed: 0, failed: 0, skipped: 0, exempt: 0 };
    for (const row of detail?.cases ?? []) {
      if (row.status === "passed") by.passed++;
      else if (row.status === "failed") by.failed++;
      else if (row.status === "skipped") by.skipped++;
      else if (row.status === "exempt") by.exempt++;
    }
    return by;
  }, [detail]);

  return (
    <Drawer isOpen={!!run} onOpenChange={(open) => { if (!open) onClose(); }} placement="right" size="5xl" scrollBehavior="inside" classNames={{ base: "w-full sm:max-w-6xl", body: "gap-4" }}>
      <DrawerContent>
        {(close) => (
          <>
            <DrawerHeader className="flex flex-col gap-2 border-b border-default-200 pb-3">
              <div className="flex w-full flex-wrap items-start gap-2">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs text-default-400">任务详情 · {detail?.run.channel || "未命名任务"}</p>
                  <div className="mt-0.5 flex flex-wrap items-baseline gap-2">
                    <span className="break-all font-mono text-lg font-semibold sm:text-xl">{detail ? hostOf(detail.run.base_url) : "任务详情"}</span>
                    <span className="text-sm text-default-500">{detail?.run.model}</span>
                    {detail && <Chip size="sm" variant="flat" color={statusColor[detail.run.status]} className="h-5 text-[10px]">{statusLabel[detail.run.status]}</Chip>}
                    {detail && <Chip size="sm" variant="flat" color={gradeColor(gradeOf(detail.run))} className="h-5 text-[10px]">评级 {gradeOf(detail.run)}{typeof detail.run.pass_rate === "number" ? ` · ${detail.run.pass_rate.toFixed(1)}%` : ""}</Chip>}
                  </div>
                  {detail && (
                    <p className="mt-1 text-xs text-default-400">
                      任务 ID：<span className="select-all font-mono text-default-500">{detail.run.id}</span> · {displayTime(detail.run.created_at)} · {detail.run.done_cases}/{detail.run.total_cases} 项
                    </p>
                  )}
                </div>
                <Button size="sm" variant="light" onPress={close}>关闭</Button>
              </div>
              {detail && (
                <div className="flex w-full flex-wrap items-center gap-2">
                  {detail.run.verdict && (
                    <span className={`text-sm font-medium ${detail.run.verdict.startsWith("0 项") ? "text-success" : "text-danger"}`}>{detail.run.verdict}</span>
                  )}
                  {detail.run.status === "running" && <Button size="sm" variant="flat" color="warning" onPress={() => act(pauseRun, detail.run.id, "已请求暂停")}>暂停</Button>}
                  {["paused", "aborted"].includes(detail.run.status) && <Button size="sm" color="primary" variant="flat" onPress={() => act(resumeRun, detail.run.id, "已继续")}>继续跑</Button>}
                  {["queued", "running", "pausing", "paused"].includes(detail.run.status) && (
                    <Button size="sm" variant="flat" color="danger" onPress={() => act(abortRun, detail.run.id, detail.run.status === "queued" ? "已取消排队" : "已终止")}>
                      {detail.run.status === "queued" ? "取消排队" : "终止"}
                    </Button>
                  )}
                  {detail.run.status === "failed" && (
                    <>
                      <Button size="sm" variant="flat" onPress={() => exportFailedEvidence(detail.run.id, "md")}>未通过 MD</Button>
                      <Button size="sm" variant="flat" onPress={() => exportFailedEvidence(detail.run.id, "txt")}>未通过 TXT</Button>
                    </>
                  )}
                  {detail.run.stop_reason && <span className="text-xs text-default-400">{detail.run.stop_reason}</span>}
                </div>
              )}
            </DrawerHeader>
            <DrawerBody>
              {loading && <div className="flex items-center gap-2 py-8 text-sm text-default-500"><Spinner size="sm" /> 正在加载任务详情…</div>}
              {error && <p className="rounded-medium bg-danger/10 p-3 text-sm text-danger">{error}</p>}
              {detail && (
                <>
                  <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                    {[
                      { label: "通过率", value: typeof detail.run.pass_rate === "number" ? `${detail.run.pass_rate.toFixed(1)}%` : "—", note: `评级 ${gradeOf(detail.run)}` },
                      { label: "通过", value: counts.passed, note: `考核 ${detail.run.assessed ?? counts.passed + counts.failed} 项` },
                      { label: "失败", value: counts.failed, note: "P0/P1 需处理" },
                      { label: "跳过/豁免", value: counts.skipped + counts.exempt, note: "不计通过率" },
                    ].map((item) => (
                      <div key={item.label} className="rounded-medium border border-default-200 px-3 py-2">
                        <p className="text-xs text-default-400">{item.label}</p>
                        <p className="mt-0.5 text-lg font-semibold tabular-nums">{item.value}</p>
                        <p className="text-[11px] text-default-400">{item.note}</p>
                      </div>
                    ))}
                  </div>
                  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
                    <Progress size="sm" value={detail.run.total_cases ? (detail.run.done_cases / detail.run.total_cases) * 100 : 0} label={`进度 ${detail.run.done_cases}/${detail.run.total_cases} 项`} />
                    <details className="rounded-medium border border-default-200 px-3 py-2">
                      <summary className="cursor-pointer text-sm font-medium text-primary">导出报告</summary>
                      <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-[110px_100px_90px_90px_auto]">
                        <Select aria-label="内容" size="sm" selectedKeys={[exportDetail]} onChange={(e) => setExportDetail(e.target.value as ExportDetail)}>
                          <SelectItem key="brief">精简版</SelectItem>
                          <SelectItem key="full">详细版</SelectItem>
                        </Select>
                        <Select aria-label="脱敏" size="sm" selectedKeys={[exportType]} onChange={(e) => setExportType(e.target.value as ExportType)}>
                          <SelectItem key="masked">脱敏</SelectItem>
                          <SelectItem key="full">不脱敏</SelectItem>
                        </Select>
                        <Select aria-label="格式" size="sm" selectedKeys={[exportFormat]} onChange={(e) => setExportFormat(e.target.value as ExportFormat)}>
                          <SelectItem key="pdf">PDF</SelectItem>
                          <SelectItem key="md">MD</SelectItem>
                          <SelectItem key="json">JSON</SelectItem>
                        </Select>
                        <Select aria-label="水印" size="sm" selectedKeys={[watermark ? "on" : "off"]} onChange={(e) => setWatermark(e.target.value === "on")}>
                          <SelectItem key="on">带水印</SelectItem>
                          <SelectItem key="off">无水印</SelectItem>
                        </Select>
                        <Button size="sm" color="primary" variant="flat" onPress={() => exportRun(detail.run.id, exportType, exportFormat, exportDetail, undefined, undefined, watermark)}>导出</Button>
                      </div>
                    </details>
                  </div>
                  <Divider />
                  <div className="flex flex-wrap items-center gap-2">
                    <Input size="sm" className="min-w-[220px] flex-1" label="筛选用例" placeholder="ID / 名称 / 说明" value={caseQuery} onValueChange={setCaseQuery} isClearable />
                    <Button size="sm" variant={onlyFailed ? "solid" : "flat"} color={onlyFailed ? "danger" : "default"} onPress={() => setOnlyFailed((v) => !v)}>
                      仅看失败 {failed}/{detail.cases.length}
                    </Button>
                  </div>
                  <div className="flex flex-col gap-1">
                    {cases.length === 0 && <p className="py-8 text-center text-sm text-default-400">没有匹配的用例</p>}
                    {cases.map((row) => (
                      <div key={row.id} className="rounded-medium border border-default-100">
                        <div
                          className="grid cursor-pointer grid-cols-[68px_76px_minmax(0,1fr)_auto] items-center gap-2 px-2 py-2 text-sm hover:bg-default-50 max-sm:grid-cols-[68px_minmax(0,1fr)_auto]"
                          onClick={() => setExpanded((prev) => {
                            const next = new Set(prev);
                            if (next.has(row.id)) next.delete(row.id);
                            else next.add(row.id);
                            return next;
                          })}
                        >
                          <Chip size="sm" variant="flat" color={caseColor[row.status]} className="h-5 justify-center text-[10px]">{caseLabel[row.status]}</Chip>
                          <span className="font-mono text-xs text-default-400">{row.case_id}</span>
                          <span className="min-w-0 truncate">{row.name}</span>
                          <div className="flex items-center gap-1">
                            <Chip size="sm" variant="bordered" color={row.level === "P0" ? "danger" : "default"} className="h-5 text-[10px]">{levelNote[row.level] ?? row.level}</Chip>
                            {row.duration_ms > 0 && <span className="text-xs text-default-300">{Math.round(row.duration_ms / 1000)}s</span>}
                            <span className="text-xs text-default-400">{expanded.has(row.id) ? "▲" : "▼"}</span>
                          </div>
                          {row.summary && <p className="col-span-full truncate text-xs text-default-400">{row.summary}</p>}
                        </div>
                        {expanded.has(row.id) && <EvidenceDetail runId={detail.run.id} row={row} />}
                      </div>
                    ))}
                  </div>
                </>
              )}
            </DrawerBody>
          </>
        )}
      </DrawerContent>
    </Drawer>
  );
}
