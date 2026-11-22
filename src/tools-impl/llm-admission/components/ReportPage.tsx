import { useCallback, useEffect, useMemo, useState } from "react";
import {
  addToast,
  Button,
  Chip,
  Input,
  Progress,
  Select,
  SelectItem,
  Spinner,
} from "@heroui/react";

import {
  abortRun,
  listDomainGroups,
  pauseRun,
  resumeRun,
  type DomainGroup,
  type Run,
  type SuiteInfo,
} from "../api";
import {
  displayTime,
  gradeColor,
  groupStatus,
  hostOf,
  rangeSummary,
  statusColor,
  statusLabel,
} from "../lib/report";
import RunDetailDrawer from "./RunDetailDrawer";

type SortKey = "recent" | "active" | "failed";
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
        item = { domain: host, name: run.channel || host, hosts: [host], models: [], channels: [], run_count: 0, active_count: 0, failed_by_level: {}, runs: [] };
        map.set(host, item);
      }
      item.runs.push(run);
      item.run_count++;
      if (runActive(run)) item.active_count++;
      if (!item.models.includes(run.model)) item.models.push(run.model);
      if (run.channel && !item.channels.includes(run.channel)) item.channels.push(run.channel);
      if (!item.latest || run.created_at > item.latest.created_at) item.latest = run;
      const terminal = ["succeeded", "failed", "aborted"].includes(run.status);
      if (terminal && (!item.last_completed || (run.finished_at || run.created_at) > (item.last_completed.finished_at || item.last_completed.created_at))) item.last_completed = run;
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
  return [group.domain, group.name, ...group.hosts, ...group.models, ...group.channels, ...group.runs.map((r) => r.id)].join(" ").toLowerCase().includes(q);
}

function currentProgress(group: DomainGroup) {
  const run = group.runs.find((r) => ["running", "pausing", "paused"].includes(r.status) && r.progress);
  return run ? { run, progress: run.progress! } : null;
}

function progressLabel(run: Run): string {
  const p = run.progress;
  if (!p) return "";
  return `${p.step || p.case_name || p.case_id}${p.total > 0 ? ` · ${p.done}/${p.total}` : ""} · 请求 ${p.requests}`;
}

function statusDotClass(status: string): string {
  switch (status) {
    case "running":
    case "pausing": return "bg-primary";
    case "queued": return "bg-warning";
    case "paused": return "bg-warning";
    case "failed": return "bg-danger";
    case "succeeded": return "bg-success";
    default: return "bg-default-300";
  }
}

export default function ReportPage({ suites, onStats }: { suites: SuiteInfo[]; onStats?: (stats: { runs: number; active: number }) => void }) {
  const [groups, setGroups] = useState<DomainGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState(() => readParam("q", ""));
  const [status, setStatus] = useState(() => readParam("status", "all"));
  const [model, setModel] = useState(() => readParam("model", ""));
  const [sort, setSort] = useState<SortKey>(() => {
    const value = readParam("sort", "recent");
    return value === "active" || value === "failed" ? value : "recent";
  });
  const [groupMode, setGroupMode] = useState<GroupMode>(() => readParam("group", "domain") === "host" ? "host" : "domain");
  const [selectedDomain, setSelectedDomain] = useState(() => readParam("domain", ""));
  const [detail, setDetail] = useState<Run | null>(null);

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

  useEffect(() => {
    if (!selectedDomain && window.matchMedia?.("(min-width: 1024px)").matches && visibleGroups[0]) {
      setSelectedDomain(visibleGroups[0].domain);
    }
  }, [selectedDomain, visibleGroups]);

  useEffect(() => {
    const p = new URLSearchParams(window.location.search);
    const set = (key: string, value: string, fallback: string) => {
      if (!value || value === fallback) p.delete(key);
      else p.set(key, value);
    };
    set("q", query, "");
    set("status", status, "all");
    set("model", model, "");
    set("sort", sort, "recent");
    set("group", groupMode, "domain");
    set("domain", selectedDomain, "");
    window.history.replaceState(null, "", `${window.location.pathname}${p.toString() ? `?${p}` : ""}`);
  }, [query, status, model, sort, groupMode, selectedDomain]);

  const totalRuns = useMemo(() => sourceGroups.reduce((n, g) => n + g.run_count, 0), [sourceGroups]);
  const activeRuns = useMemo(() => sourceGroups.reduce((n, g) => n + g.active_count, 0), [sourceGroups]);
  const failedDomains = useMemo(() => sourceGroups.filter((g) => g.last_completed?.status === "failed").length, [sourceGroups]);
  const selectedGroup = sourceGroups.find((g) => g.domain === selectedDomain) || null;
  const selectedLatest = selectedGroup ? (selectedGroup.last_completed || selectedGroup.latest) : null;

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

  const pickDomain = (domain: string) => setSelectedDomain(domain);

  if (loading) return <div className="flex justify-center py-16"><Spinner /></div>;

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-2 sm:gap-3 xl:grid-cols-4">
        {[
          { label: groupMode === "host" ? "测试 host" : "测试域名", value: sourceGroups.length, note: groupMode === "host" ? "按完整 host" : "按主域名" },
          { label: "任务总数", value: totalRuns, note: "全部历史任务" },
          { label: "活跃/暂停", value: activeRuns, note: "暂停不占队列" },
          { label: "未通过域名", value: failedDomains, note: "最近完成结论" },
        ].map((item) => (
          <div key={item.label} className="rounded-large border border-default-200 bg-default-50/60 px-4 py-3">
            <p className="text-xs text-default-400">{item.label}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">{item.value}</p>
            <p className="mt-0.5 text-xs text-default-300">{item.note}</p>
          </div>
        ))}
      </div>

      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_120px_140px_160px_140px] lg:items-end">
        <Input className="col-span-2 lg:col-span-1" size="sm" label="搜索" placeholder="域名 / 名称 / 模型 / 任务 ID" value={query} onValueChange={setQuery} isClearable />
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
        <Select aria-label="模型" size="sm" label="模型" selectedKeys={[model || "all"]} onChange={(e) => setModel(e.target.value === "all" ? "" : e.target.value)}>
          {["all", ...allModels].map((m) => <SelectItem key={m} textValue={m === "all" ? "全部模型" : m}>{m === "all" ? "全部模型" : m}</SelectItem>)}
        </Select>
        <Select aria-label="排序" size="sm" label="排序" selectedKeys={[sort]} onChange={(e) => setSort(e.target.value as SortKey)}>
          <SelectItem key="recent">最近测试优先</SelectItem>
          <SelectItem key="active">活跃优先</SelectItem>
          <SelectItem key="failed">未通过优先</SelectItem>
        </Select>
      </div>

      {error && <p className="rounded-medium bg-danger/10 px-3 py-2 text-sm text-danger">{error}</p>}

      <div className="grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
        <aside className={`${selectedDomain ? "hidden xl:block" : "block"} min-w-0`}>
          <div className="flex items-center justify-between px-1 pb-2 text-xs text-default-400">
            <span>{visibleGroups.length} 个域名</span>
            <Button size="sm" variant="light" onPress={refresh}>刷新</Button>
          </div>
          <div className="flex max-h-[calc(100vh-18rem)] flex-col gap-1 overflow-y-auto pr-1">
            {visibleGroups.length === 0 && <p className="py-12 text-center text-sm text-default-400">没有匹配的域名</p>}
            {visibleGroups.map((group) => {
              const gs = groupStatus(group);
              const latest = group.latest;
              const active = currentProgress(group);
              return (
                <button
                  key={group.domain}
                  type="button"
                  onClick={() => pickDomain(group.domain)}
                  className={`w-full rounded-medium border px-3 py-3 text-left transition-colors ${selectedDomain === group.domain ? "border-primary bg-primary/5" : "border-default-200 hover:bg-default-50"}`}
                >
                  <div className="flex items-center gap-2">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${statusDotClass(gs)}`} />
                    <span className="min-w-0 flex-1 truncate font-mono text-sm font-semibold">{group.domain}</span>
                    {latest?.grade && <Chip size="sm" variant="flat" color={gradeColor(latest.grade)} className="h-5 text-[10px]">{latest.grade}</Chip>}
                  </div>
                  <p className="mt-1 truncate text-xs text-default-500">{group.name || "未命名"} · {group.models.join(" / ")}</p>
                  <div className="mt-1 flex items-center justify-between gap-2 text-[11px] text-default-400">
                    <span>{group.run_count} 个任务</span>
                    <span>{displayTime(latest?.created_at || "")}</span>
                  </div>
                  <div className="mt-1 flex items-center gap-2 text-[11px]">
                    <span className="text-default-400">{statusLabel[gs] || gs}</span>
                    {latest?.verdict && <span className={latest.verdict.startsWith("0 项") ? "truncate text-success" : "truncate text-danger"}>{latest.verdict}</span>}
                  </div>
                  {active && (
                    <div className="mt-2">
                      <Progress size="sm" value={active.progress.total ? (active.progress.done / active.progress.total) * 100 : 0} />
                      <p className="mt-1 truncate text-[11px] text-default-400">{progressLabel(active.run)}</p>
                    </div>
                  )}
                </button>
              );
            })}
          </div>
        </aside>

        <section className={`${selectedDomain ? "block" : "hidden xl:block"} min-w-0`}>
          {!selectedGroup ? (
            <div className="flex min-h-[320px] items-center justify-center rounded-large border border-dashed border-default-200 text-sm text-default-400">
              选择左侧域名查看报告
            </div>
          ) : (
            <div className="flex flex-col gap-4 rounded-large border border-default-200 p-4 sm:p-5">
              <div className="flex flex-wrap items-start gap-2">
                <Button className="xl:hidden" size="sm" variant="light" onPress={() => setSelectedDomain("")}>返回</Button>
                <div className="min-w-0 flex-1">
                  <p className="text-xs text-default-400">{selectedGroup.name || "未命名"}</p>
                  <div className="flex flex-wrap items-baseline gap-2">
                    <h2 className="break-all font-mono text-xl font-semibold sm:text-2xl">{selectedGroup.domain}</h2>
                    <Chip size="sm" variant="flat" color={statusColor[groupStatus(selectedGroup)]} className="h-5 text-[10px]">{statusLabel[groupStatus(selectedGroup)]}</Chip>
                    {selectedGroup.last_completed?.grade && (
                      <Chip size="sm" variant="flat" color={gradeColor(selectedGroup.last_completed.grade)} className="h-5 text-[10px]">
                        评级 {selectedGroup.last_completed.grade} · {selectedGroup.last_completed.pass_rate?.toFixed(1)}%
                      </Chip>
                    )}
                  </div>
                  <p className="mt-1 text-xs text-default-400">
                    {selectedGroup.models.join(" / ")} · {selectedGroup.run_count} 个任务 · 最近 {displayTime(selectedGroup.latest?.created_at || "")}
                    {selectedGroup.latest?.id ? ` · ${selectedGroup.latest.id}` : ""}
                  </p>
                </div>
                <div className="flex shrink-0 gap-1">
                  {selectedLatest && <Button size="sm" variant="flat" onPress={() => setDetail(selectedLatest)}>打开最近报告</Button>}
                </div>
              </div>

              {currentProgress(selectedGroup) && (
                <div className="rounded-medium border border-primary/20 bg-primary/5 px-3 py-2">
                  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                    <span className="font-medium">当前：{currentProgress(selectedGroup)!.progress.case_id} {currentProgress(selectedGroup)!.progress.case_name}</span>
                    <span className="text-default-500">{progressLabel(currentProgress(selectedGroup)!.run)}</span>
                  </div>
                  <Progress className="mt-2" size="sm" value={currentProgress(selectedGroup)!.progress.total ? (currentProgress(selectedGroup)!.progress.done / currentProgress(selectedGroup)!.progress.total) * 100 : 0} />
                </div>
              )}

              {selectedLatest && (
                <div className="flex flex-col gap-4">
                  <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                    <div className="rounded-medium border border-default-200 px-3 py-2">
                      <p className="text-xs text-default-400">通过率</p>
                      <p className="mt-1 text-2xl font-semibold tabular-nums">{typeof selectedLatest.pass_rate === "number" ? `${selectedLatest.pass_rate.toFixed(1)}%` : "—"}</p>
                      <p className="text-[11px] text-default-400">评级 {selectedLatest.grade || "—"}</p>
                    </div>
                    <div className="rounded-medium border border-default-200 px-3 py-2">
                      <p className="text-xs text-default-400">通过</p>
                      <p className="mt-1 text-2xl font-semibold tabular-nums">{selectedLatest.passed ?? "—"}</p>
                      <p className="text-[11px] text-default-400">考核 {selectedLatest.assessed ?? "—"} 项</p>
                    </div>
                    <div className="rounded-medium border border-default-200 px-3 py-2">
                      <p className="text-xs text-default-400">失败</p>
                      <p className="mt-1 text-2xl font-semibold tabular-nums text-danger">{selectedLatest.failed ?? "—"}</p>
                      <p className="text-[11px] text-default-400">需处理项</p>
                    </div>
                    <div className="rounded-medium border border-default-200 px-3 py-2">
                      <p className="text-xs text-default-400">最近结论</p>
                      <p className={`mt-1 truncate text-sm font-medium ${selectedLatest.verdict?.startsWith("0 项") ? "text-success" : "text-danger"}`}>{selectedLatest.verdict || "暂无结论"}</p>
                      <p className="text-[11px] text-default-400">{displayTime(selectedLatest.created_at)}</p>
                    </div>
                  </div>
                </div>
              )}

              <div className="flex items-end justify-between gap-2 border-b border-default-200 pb-2">
                <div>
                  <h3 className="text-sm font-semibold">全部任务</h3>
                  <p className="text-[11px] text-default-400">{selectedGroup.run_count} 次测试记录，按时间倒序</p>
                </div>
              </div>

              <div className="flex max-h-[46vh] flex-col gap-2 overflow-y-auto pr-1">
                <div className="hidden xl:grid xl:grid-cols-[72px_128px_minmax(130px,1.2fr)_minmax(80px,.7fr)_minmax(90px,.8fr)_100px_150px] gap-2 border-b border-default-200 px-2 pb-2 text-[11px] text-default-400">
                  <span>状态</span><span>任务 ID</span><span>结论</span><span>模型</span><span>范围</span><span>时间</span><span className="text-right">操作</span>
                </div>
                {selectedGroup.runs.map((run) => (
                  <div key={run.id} className="grid gap-2 rounded-medium border border-default-100 px-2 py-2 hover:bg-default-50 xl:grid-cols-[72px_128px_minmax(130px,1.2fr)_minmax(80px,.7fr)_minmax(90px,.8fr)_100px_150px] xl:items-center">
                    <Chip size="sm" variant="flat" color={statusColor[run.status]} className="h-5 w-fit text-[10px]">{statusLabel[run.status]}</Chip>
                    <span className="select-all whitespace-nowrap font-mono text-xs text-default-500">{run.id}</span>
                    <div className="min-w-0">
                      <p className={`truncate text-xs ${run.verdict?.startsWith("0 项") ? "text-success" : "text-danger"}`}>{run.verdict || "—"}</p>
                      {run.grade && <Chip size="sm" variant="flat" color={gradeColor(run.grade)} className="mt-0.5 h-5 text-[10px]">{run.grade} {typeof run.pass_rate === "number" ? `${run.pass_rate.toFixed(1)}%` : ""}</Chip>}
                    </div>
                    <span className="truncate text-sm">{run.model}</span>
                    <div className="flex flex-wrap gap-1">
                      {(suite ? rangeSummary(run, suite) : []).slice(0, 3).map((item) => <Chip key={item.key} size="sm" variant="flat" className="h-5 text-[10px]">{item.label} {item.count}</Chip>)}
                    </div>
                    <span className="text-xs text-default-400">{displayTime(run.created_at)}</span>
                    <div className="flex flex-wrap justify-end gap-1">
                      <Button size="sm" variant="flat" onPress={() => setDetail(run)}>详情</Button>
                      {run.status === "running" && <Button size="sm" variant="flat" color="warning" onPress={() => act(pauseRun, run.id, "已请求暂停")}>暂停</Button>}
                      {["paused", "aborted"].includes(run.status) && <Button size="sm" variant="flat" color="primary" onPress={() => act(resumeRun, run.id, "已继续")}>继续</Button>}
                      {["queued", "running", "pausing", "paused"].includes(run.status) && <Button size="sm" variant="flat" color="danger" onPress={() => act(abortRun, run.id, "已终止")}>终止</Button>}
                    </div>
                    {run.progress && runActive(run) && (
                      <div className="col-span-full flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
                        <Progress size="sm" className="min-w-0 flex-1" value={run.progress.total ? (run.progress.done / run.progress.total) * 100 : 0} />
                        <span className="text-xs text-default-400">{progressLabel(run)}</span>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </section>
      </div>

      <RunDetailDrawer run={detail} onClose={() => setDetail(null)} onChanged={refresh} />
    </div>
  );
}
