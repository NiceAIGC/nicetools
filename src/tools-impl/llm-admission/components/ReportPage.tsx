import { useCallback, useEffect, useMemo, useState } from "react";
import {
  addToast,
  Button,
  Card,
  CardBody,
  Checkbox,
  Chip,
  Input,
  Progress,
  Select,
  SelectItem,
  Spinner,
} from "@heroui/react";

import {
  abortRun,
  exportMerged,
  listDomainGroups,
  pauseRun,
  resumeRun,
  type DomainGroup,
  type ExportDetail,
  type ExportFormat,
  type ExportType,
  type Run,
  type SuiteInfo,
} from "../api";
import {
  displayTime,
  failedLevelText,
  gradeColor,
  groupStatus,
  hostOf,
  rangeSummary,
  statusColor,
  statusLabel,
} from "../lib/report";
import RunDetailDrawer from "./RunDetailDrawer";

type SortKey = "active" | "recent" | "failed";
type GroupMode = "domain" | "host";

function readParam(key: string, fallback: string): string {
  try { return new URLSearchParams(window.location.search).get(key) || fallback; } catch { return fallback; }
}

function runActive(run: Run): boolean {
  return ["queued", "running", "pausing", "paused"].includes(run.status);
}

function regroupByHost(groups: DomainGroup[]): DomainGroup[] {
  const map = new Map<string, DomainGroup>();
  for (const group of groups) {
    for (const run of group.runs) {
      const host = hostOf(run.base_url) || group.domain;
      let item = map.get(host);
      if (!item) {
        item = {
          domain: host,
          name: run.channel || host,
          hosts: [host],
          models: [],
          channels: [],
          run_count: 0,
          active_count: 0,
          failed_by_level: {},
          runs: [],
        };
        map.set(host, item);
      }
      item.runs.push(run);
      item.run_count++;
      if (runActive(run)) item.active_count++;
      if (!item.models.includes(run.model)) item.models.push(run.model);
      if (run.channel && !item.channels.includes(run.channel)) item.channels.push(run.channel);
      if (!item.latest || run.created_at > item.latest.created_at) item.latest = run;
      const terminal = ["succeeded", "failed", "aborted"].includes(run.status);
      if (terminal && (!item.last_completed || (run.finished_at || run.created_at) > (item.last_completed.finished_at || item.last_completed.created_at))) {
        item.last_completed = run;
      }
    }
  }
  return [...map.values()].map((item) => ({
    ...item,
    name: item.latest?.channel || item.name,
    models: item.models.sort(),
    channels: item.channels.sort(),
    runs: item.runs.sort((a, b) => b.created_at.localeCompare(a.created_at)),
  }));
}

function groupMatches(group: DomainGroup, query: string, status: string, model: string): boolean {
  if (model && !group.models.includes(model)) return false;
  const gs = groupStatus(group);
  if (status === "active" && !group.active_count) return false;
  if (status === "failed" && !(gs === "failed" || group.last_completed?.status === "failed")) return false;
  if (status === "succeeded" && gs !== "succeeded") return false;
  if (status === "paused" && gs !== "paused") return false;
  if (!query) return true;
  const q = query.toLowerCase();
  return [group.domain, group.name, ...group.hosts, ...group.models, ...group.channels, ...group.runs.map((r) => r.id)]
    .join(" ")
    .toLowerCase()
    .includes(q);
}

function domainCardSummary(group: DomainGroup): string {
  const gs = groupStatus(group);
  if (gs === "running" || gs === "pausing") return "运行中";
  if (gs === "queued") return "排队中";
  if (gs === "paused") return "已暂停";
  return group.last_completed?.verdict || "暂无结论";
}

export default function ReportPage({ suites, onStats }: { suites: SuiteInfo[]; onStats?: (stats: { runs: number; active: number }) => void }) {
  const [groups, setGroups] = useState<DomainGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState(() => readParam("q", ""));
  const [status, setStatus] = useState(() => readParam("status", "all"));
  const [model, setModel] = useState(() => readParam("model", ""));
  const [sort, setSort] = useState<SortKey>(() => readParam("sort", "active") as SortKey);
  const [groupMode, setGroupMode] = useState<GroupMode>(() => readParam("group", "domain") === "host" ? "host" : "domain");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [detail, setDetail] = useState<Run | null>(null);
  const [exportFormat, setExportFormat] = useState<ExportFormat>("pdf");
  const [exportDetail, setExportDetail] = useState<ExportDetail>("brief");
  const [exportType, setExportType] = useState<ExportType>("masked");
  const [watermark, setWatermark] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const res = await listDomainGroups();
      setGroups(res.groups);
      setError("");
      const runnable = res.groups.reduce((n, g) => n + g.runs.filter((r) => ["queued", "running", "pausing"].includes(r.status)).length, 0);
      onStats?.({ runs: res.groups.reduce((n, g) => n + g.run_count, 0), active: runnable });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [onStats]);

  useEffect(() => { refresh(); }, [refresh]);

  const hasActive = useMemo(() => groups.some((g) => g.active_count > 0), [groups]);
  useEffect(() => {
    if (!hasActive) return;
    const timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  }, [hasActive, refresh]);

  const suite = suites[0];
  const sourceGroups = useMemo(() => groupMode === "host" ? regroupByHost(groups) : groups, [groups, groupMode]);
  const allModels = useMemo(() => {
    const counts = new Map<string, number>();
    for (const group of sourceGroups) for (const run of group.runs) counts.set(run.model, (counts.get(run.model) ?? 0) + 1);
    return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([name]) => name);
  }, [sourceGroups]);
  const visibleGroups = useMemo(() => {
    const list = sourceGroups.filter((g) => groupMatches(g, query.trim(), status, model));
    return [...list].sort((a, b) => {
      if (sort === "active") {
        const diff = (b.active_count > 0 ? 1 : 0) - (a.active_count > 0 ? 1 : 0);
        if (diff) return diff;
      }
      if (sort === "failed") {
        const diff = (b.last_completed?.status === "failed" ? 1 : 0) - (a.last_completed?.status === "failed" ? 1 : 0);
        if (diff) return diff;
      }
      return (b.latest?.created_at || "").localeCompare(a.latest?.created_at || "");
    });
  }, [sourceGroups, query, status, model, sort]);

  const totalRuns = useMemo(() => sourceGroups.reduce((n, g) => n + g.run_count, 0), [sourceGroups]);
  const activeRuns = useMemo(() => sourceGroups.reduce((n, g) => n + g.active_count, 0), [sourceGroups]);
  const failedDomains = useMemo(() => sourceGroups.filter((g) => g.last_completed?.status === "failed").length, [sourceGroups]);
  const selectedRuns = useMemo(() => sourceGroups.flatMap((g) => g.runs).filter((r) => selected.has(r.id)), [sourceGroups, selected]);
  const mergeable = selectedRuns.length >= 2 && selectedRuns.every((r) =>
    r.suite === selectedRuns[0].suite &&
    r.base_url === selectedRuns[0].base_url &&
    r.model === selectedRuns[0].model &&
    r.channel === selectedRuns[0].channel,
  );

  useEffect(() => {
    const p = new URLSearchParams(window.location.search);
    const set = (key: string, value: string, fallback: string) => {
      if (!value || value === fallback) p.delete(key);
      else p.set(key, value);
    };
    set("q", query, "");
    set("status", status, "all");
    set("model", model, "");
    set("sort", sort, "active");
    set("group", groupMode, "domain");
    const next = `${window.location.pathname}${p.toString() ? `?${p}` : ""}`;
    window.history.replaceState(null, "", next);
  }, [query, status, model, sort, groupMode]);

  const toggle = (domain: string) => setExpanded((prev) => {
    const next = new Set(prev);
    if (next.has(domain)) next.delete(domain);
    else next.add(domain);
    return next;
  });

  const toggleSelected = (id: string, on: boolean) => setSelected((prev) => {
    const next = new Set(prev);
    if (on) next.add(id);
    else next.delete(id);
    return next;
  });

  const act = async (fn: (id: string) => Promise<unknown>, id: string, message: string) => {
    try {
      await fn(id);
      addToast({ title: message });
      await refresh();
      if (detail?.id === id) setDetail((prev) => prev ? { ...prev } : prev);
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    }
  };

  if (loading) return <div className="flex justify-center py-16"><Spinner /></div>;

  return (
    <div className="flex flex-col gap-5">
      <div className="grid grid-cols-2 gap-2 sm:gap-3 xl:grid-cols-4">
        {[
          { label: groupMode === "host" ? "测试 host" : "测试域名", value: sourceGroups.length, note: groupMode === "host" ? "按完整 host 聚合" : "按主域名聚合" },
          { label: "任务总数", value: totalRuns, note: "全部历史任务" },
          { label: "活跃/暂停", value: activeRuns, note: "暂停任务不占队列" },
          { label: "未通过域名", value: failedDomains, note: "最近完成结论" },
        ].map((item) => (
          <div key={item.label} className="rounded-large border border-default-200 bg-default-50/60 px-4 py-3">
            <p className="text-xs text-default-400">{item.label}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">{item.value}</p>
            <p className="mt-0.5 text-xs text-default-300">{item.note}</p>
          </div>
        ))}
      </div>

      <Card shadow="none" className="border border-default-200">
        <CardBody className="gap-4">
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1fr)_120px_140px_140px_130px]">
              <Input
                size="sm"
                className="col-span-2 lg:col-span-1"
                label="搜索"
                placeholder="域名 / 名称 / 模型 / 任务 ID"
                value={query}
                onValueChange={setQuery}
                isClearable
              />
              <Select aria-label="聚合" size="sm" label="聚合" selectedKeys={[groupMode]} onChange={(e) => setGroupMode(e.target.value === "host" ? "host" : "domain")}>
                <SelectItem key="domain">主域名</SelectItem>
                <SelectItem key="host">完整 host</SelectItem>
              </Select>
              <Select aria-label="状态" size="sm" label="状态" selectedKeys={[status]} onChange={(e) => setStatus(e.target.value)}>
                <SelectItem key="all">全部</SelectItem>
                <SelectItem key="active">活跃/暂停</SelectItem>
                <SelectItem key="failed">未通过</SelectItem>
                <SelectItem key="succeeded">已完成</SelectItem>
                <SelectItem key="paused">已暂停</SelectItem>
              </Select>
              <Select
                aria-label="模型"
                size="sm"
                label="模型"
                selectedKeys={[model || "all"]}
                onChange={(e) => setModel(e.target.value === "all" ? "" : e.target.value)}
              >
                {["all", ...allModels].map((m) => (
                  <SelectItem key={m} textValue={m === "all" ? "全部模型" : m}>{m === "all" ? "全部模型" : m}</SelectItem>
                ))}
              </Select>
              <Select aria-label="排序" size="sm" label="排序" selectedKeys={[sort]} onChange={(e) => setSort(e.target.value as SortKey)}>
                <SelectItem key="active">活跃优先</SelectItem>
                <SelectItem key="recent">最近优先</SelectItem>
                <SelectItem key="failed">未通过优先</SelectItem>
              </Select>
            </div>
            <div className="flex gap-1">
              <Button size="sm" variant="flat" onPress={() => setExpanded(new Set(visibleGroups.map((g) => g.domain)))}>展开全部</Button>
              <Button size="sm" variant="light" onPress={() => setExpanded(new Set())}>收起全部</Button>
              <Button size="sm" variant="flat" onPress={refresh}>刷新</Button>
            </div>
          </div>

          {selected.size > 0 && (
            <div className="flex flex-wrap items-end gap-2 rounded-medium border border-primary/20 bg-primary/5 p-3">
              <span className="pb-2 text-xs text-default-500">已选 {selected.size} 个任务</span>
              <Select aria-label="内容" size="sm" className="w-32" selectedKeys={[exportDetail]} onChange={(e) => setExportDetail(e.target.value as ExportDetail)}>
                <SelectItem key="brief">精简版</SelectItem>
                <SelectItem key="full">详细版</SelectItem>
              </Select>
              <Select aria-label="脱敏" size="sm" className="w-32" selectedKeys={[exportType]} onChange={(e) => setExportType(e.target.value as ExportType)}>
                <SelectItem key="masked">脱敏</SelectItem>
                <SelectItem key="full">不脱敏</SelectItem>
              </Select>
              <Select aria-label="格式" size="sm" className="w-28" selectedKeys={[exportFormat]} onChange={(e) => setExportFormat(e.target.value as ExportFormat)}>
                <SelectItem key="pdf">PDF</SelectItem>
                <SelectItem key="md">MD</SelectItem>
                <SelectItem key="json">JSON</SelectItem>
              </Select>
              <Select aria-label="水印" size="sm" className="w-28" selectedKeys={[watermark ? "on" : "off"]} onChange={(e) => setWatermark(e.target.value === "on")}>
                <SelectItem key="on">带水印</SelectItem>
                <SelectItem key="off">无水印</SelectItem>
              </Select>
              <Button size="sm" color="primary" variant="flat" isDisabled={!mergeable} onPress={() => exportMerged([...selected], exportType, exportFormat, exportDetail, undefined, undefined, watermark)}>合并导出</Button>
              <Button size="sm" variant="light" onPress={() => setSelected(new Set())}>清空</Button>
              <p className={`w-full text-xs ${mergeable ? "text-default-400" : "text-warning"}`}>
                {mergeable ? "仅允许同标准、同网关、同模型、同渠道的任务合并。" : "已选任务目标不一致或不足 2 个：请逐个任务导出，或只选同一目标的任务合并。"}
              </p>
            </div>
          )}

          {error && <p className="rounded-medium bg-danger/10 px-3 py-2 text-sm text-danger">{error}</p>}
          {visibleGroups.length === 0 && <p className="py-12 text-center text-sm text-default-400">没有匹配的域名报告</p>}

          <div className="flex flex-col gap-3">
            {visibleGroups.map((group) => {
              const open = expanded.has(group.domain);
              const gs = groupStatus(group);
              const latest = group.latest;
              return (
                <div key={group.domain} className="rounded-large border border-default-200">
                  <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3 p-4 max-sm:grid-cols-1">
                    <button type="button" className="min-w-0 text-left" onClick={() => toggle(group.domain)} aria-expanded={open}>
                      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                        <span className="text-xs font-medium text-default-400">{group.name || "未命名任务"}</span>
                        <span className="break-all font-mono text-lg font-semibold tracking-tight text-foreground sm:text-xl">{group.domain}</span>
                        <Chip size="sm" variant="flat" color={statusColor[gs]} className="h-5 text-[10px]">{statusLabel[gs] || gs}</Chip>
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-2 text-sm">
                        {group.models.map((m) => <span key={m} className="text-default-500">{m}</span>)}
                        <span className="text-default-300">·</span>
                        <span className={group.last_completed?.status === "failed" ? "font-medium text-danger" : "text-success"}>
                          {domainCardSummary(group)}
                        </span>
                        {group.last_completed?.grade && (
                          <Chip size="sm" variant="flat" color={gradeColor(group.last_completed.grade)} className="h-5 text-[10px]">
                            评级 {group.last_completed.grade}
                            {typeof group.last_completed.pass_rate === "number" ? ` · ${group.last_completed.pass_rate.toFixed(1)}%` : ""}
                          </Chip>
                        )}
                        {failedLevelText(group) && <span className="text-xs text-default-400">{failedLevelText(group)}</span>}
                      </div>
                      <p className="mt-1 text-xs text-default-400">
                        {group.run_count} 个任务 · {group.active_count ? `${group.active_count} 个活跃 · ` : ""}
                        最近 {displayTime(latest?.created_at || "")}
                        {latest?.id ? ` · ${latest.id}` : ""}
                      </p>
                    </button>
                    <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
                      {(group.last_completed || latest) && (
                        <Button size="sm" variant="flat" onPress={() => setDetail(group.last_completed || latest || null)}>最近报告</Button>
                      )}
                      <Button size="sm" variant="flat" onPress={() => toggle(group.domain)}>{open ? "收起" : `展开 ${group.run_count}`}</Button>
                    </div>
                  </div>

                  {open && (
                    <div className="border-t border-default-200 bg-default-50/40 p-3 sm:p-4">
                      <div className="hidden grid-cols-[28px_72px_minmax(120px,1.3fr)_minmax(90px,.8fr)_minmax(90px,.8fr)_minmax(130px,1fr)_130px_170px] gap-2 border-b border-default-200 px-2 pb-2 text-[11px] text-default-400 lg:grid">
                        <span />
                        <span>状态</span>
                        <span>任务 ID / 结论</span>
                        <span>模型</span>
                        <span>渠道</span>
                        <span>时间</span>
                        <span>范围</span>
                        <span className="text-right">操作</span>
                      </div>
                      <div className="flex flex-col gap-1">
                        {group.runs.map((run) => {
                          const ranges = suite ? rangeSummary(run, suite) : [];
                          const isActive = runActive(run);
                          return (
                            <div key={run.id} className="grid gap-2 rounded-medium px-2 py-2 hover:bg-default-100/70 lg:grid-cols-[28px_72px_minmax(120px,1.3fr)_minmax(90px,.8fr)_minmax(90px,.8fr)_minmax(130px,1fr)_130px_170px] lg:items-center">
                              <Checkbox aria-label={`选择任务 ${run.id}`} size="sm" isSelected={selected.has(run.id)} onValueChange={(on) => toggleSelected(run.id, on)} />
                              <div className="flex items-center gap-1">
                                <Chip size="sm" variant="flat" color={statusColor[run.status]} className="h-5 text-[10px]">{statusLabel[run.status]}</Chip>
                              </div>
                              <div className="min-w-0">
                                <p className="select-all truncate font-mono text-xs text-default-600">{run.id}</p>
                                <div className="flex flex-wrap items-center gap-1">
                                  <p className={`truncate text-xs ${run.verdict.startsWith("0 项") ? "text-success" : "text-danger"}`}>{run.verdict || "—"}</p>
                                  {run.grade && <Chip size="sm" variant="flat" color={gradeColor(run.grade)} className="h-5 text-[10px]">{run.grade}{typeof run.pass_rate === "number" ? ` ${run.pass_rate.toFixed(1)}%` : ""}</Chip>}
                                </div>
                              </div>
                              <span className="truncate text-sm">{run.model}</span>
                              <span className="truncate text-sm text-default-500">{run.channel || "—"}</span>
                              <div className="text-xs text-default-400">
                                <p>{displayTime(run.created_at)}</p>
                                <p>{run.done_cases}/{run.total_cases} 项</p>
                              </div>
                              <div className="flex flex-wrap gap-1">
                                {ranges.slice(0, 4).map((item) => <Chip key={item.key} size="sm" variant="flat" className="h-5 text-[10px]">{item.label} {item.count}</Chip>)}
                              </div>
                              <div className="flex flex-nowrap justify-end gap-1">
                                <Button size="sm" variant="flat" onPress={() => setDetail(run)}>详情</Button>
                                {run.status === "running" && <Button size="sm" variant="flat" color="warning" onPress={() => act(pauseRun, run.id, "已请求暂停")}>暂停</Button>}
                                {["paused", "aborted"].includes(run.status) && <Button size="sm" variant="flat" color="primary" onPress={() => act(resumeRun, run.id, "已继续")}>继续</Button>}
                                {isActive && <Button size="sm" variant="flat" color="danger" onPress={() => act(abortRun, run.id, "已终止")}>终止</Button>}
                              </div>
                              {!isActive && run.status === "failed" && (
                                <div className="lg:col-span-8 lg:hidden">
                                  <Progress size="sm" value={run.total_cases ? (run.done_cases / run.total_cases) * 100 : 0} />
                                </div>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </CardBody>
      </Card>

      <RunDetailDrawer
        run={detail}
        onClose={() => setDetail(null)}
        onChanged={refresh}
      />
    </div>
  );
}
