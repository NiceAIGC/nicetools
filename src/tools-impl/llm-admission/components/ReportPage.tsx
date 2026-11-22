import { useCallback, useEffect, useMemo, useState } from "react";
import {
  addToast,
  Button,
  Chip,
  Input,
  Select,
  SelectItem,
  Spinner,
} from "@heroui/react";

import {
  abortRun,
  listDomainGroups,
  pauseRun,
  saveDomainNote,
  saveRunNote,
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
        item = { domain: host, note: group.note || "", name: run.channel || host, hosts: [host], models: [], channels: [], run_count: 0, active_count: 0, failed_by_level: {}, runs: [] };
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
  return [group.domain, group.note, ...group.hosts, ...group.models, ...group.channels, ...group.runs.map((r) => `${r.id} ${r.task_note || ""}`)].join(" ").toLowerCase().includes(q);
}

export default function ReportPage({ suites, onStats }: { suites: SuiteInfo[]; onStats?: (stats: { runs: number; active: number }) => void }) {
  const [groups, setGroups] = useState<DomainGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState(() => readParam("q", ""));
  const [status, setStatus] = useState(() => readParam("status", "all"));
  const [model, setModel] = useState(() => readParam("model", ""));
  const [groupMode, setGroupMode] = useState<GroupMode>(() => readParam("group", "domain") === "host" ? "host" : "domain");
  const [selectedDomain, setSelectedDomain] = useState(() => readParam("domain", ""));
  const [detail, setDetail] = useState<Run | null>(null);
  const [editingDomain, setEditingDomain] = useState("");
  const [noteDraft, setNoteDraft] = useState("");
  const [savingNote, setSavingNote] = useState(false);
  const [editingTaskID, setEditingTaskID] = useState("");
  const [taskNoteDraft, setTaskNoteDraft] = useState("");
  const [savingTaskNote, setSavingTaskNote] = useState(false);

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
    // 严格按照提交任务时间倒序；不再按活跃/失败状态打乱。
    return [...list].sort((a, b) => {
      const timeDiff = (b.latest?.created_at || "").localeCompare(a.latest?.created_at || "");
      return timeDiff || a.domain.localeCompare(b.domain);
    });
  }, [sourceGroups, query, status, model]);

  useEffect(() => {
    if (!selectedDomain && window.matchMedia?.("(min-width: 1280px)").matches && visibleGroups[0]) {
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
    set("group", groupMode, "domain");
    set("domain", selectedDomain, "");
    window.history.replaceState(null, "", `${window.location.pathname}${p.toString() ? `?${p}` : ""}`);
  }, [query, status, model, groupMode, selectedDomain]);

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

  const pickDomain = (domain: string) => {
    setSelectedDomain(domain);
    setEditingDomain("");
    setNoteDraft("");
    setEditingTaskID("");
  };

  const beginNote = (group: DomainGroup) => {
    setEditingDomain(group.domain);
    setNoteDraft(group.note || "");
  };

  const saveNote = async (domain: string) => {
    setSavingNote(true);
    try {
      await saveDomainNote(domain, noteDraft.trim());
      addToast({ title: "备注已保存" });
      setEditingDomain("");
      await refresh();
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    } finally {
      setSavingNote(false);
    }
  };

  const saveTaskNote = async (runId: string) => {
    setSavingTaskNote(true);
    try {
      await saveRunNote(runId, taskNoteDraft.trim());
      addToast({ title: "任务备注已保存" });
      setEditingTaskID("");
      await refresh();
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    } finally {
      setSavingTaskNote(false);
    }
  };

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

      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_120px_140px_160px] lg:items-end">
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
              const latest = group.latest;
              const title = group.note ? `${group.note}-${group.domain}` : group.domain;
              return (
                <div
                  key={group.domain}
                  role="button"
                  tabIndex={0}
                  onClick={() => pickDomain(group.domain)}
                  onKeyDown={(e) => {
                    if (e.target !== e.currentTarget) return;
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      pickDomain(group.domain);
                    }
                  }}
                  className={`w-full cursor-pointer rounded-medium border px-3 py-3 text-left transition-colors ${selectedDomain === group.domain ? "border-primary bg-primary/5" : "border-default-200 hover:bg-default-50"}`}
                >
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 flex-1 truncate font-mono text-sm font-semibold" title={title}>{title}</span>
                    {latest?.grade && <Chip size="sm" variant="flat" color={gradeColor(latest.grade)} className="h-5 text-[10px]">{latest.grade}</Chip>}
                  </div>
                  <p className="mt-1 truncate text-xs text-default-500">{group.models.join(" / ") || "—"}</p>
                  <div className="mt-1 flex items-center justify-between gap-2 text-[11px] text-default-400">
                    <span>{group.run_count} 个任务</span>
                    <span>{displayTime(latest?.created_at || "")}</span>
                  </div>
                </div>
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
                  <div className="flex flex-wrap items-baseline gap-2">
                    <h2 className="break-all font-mono text-xl font-semibold sm:text-2xl">{selectedGroup.note ? `${selectedGroup.note}-${selectedGroup.domain}` : selectedGroup.domain}</h2>
                    <Chip size="sm" variant="flat" color={statusColor[groupStatus(selectedGroup)]} className="h-5 text-[10px]">{statusLabel[groupStatus(selectedGroup)]}</Chip>
                    {selectedGroup.last_completed?.grade && (
                      <Chip size="sm" variant="flat" color={gradeColor(selectedGroup.last_completed.grade)} className="h-5 text-[10px]">
                        评级 {selectedGroup.last_completed.grade}
                      </Chip>
                    )}
                  </div>
                  <p className="mt-1 text-xs text-default-400">
                    {selectedGroup.models.join(" / ")} · {selectedGroup.run_count} 个任务 · 最近 {displayTime(selectedGroup.latest?.created_at || "")}
                    {selectedGroup.latest?.id ? ` · ${selectedGroup.latest.id}` : ""}
                  </p>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button size="sm" variant="flat" onPress={() => beginNote(selectedGroup)}>{selectedGroup.note ? "改备注" : "设置备注"}</Button>
                  {selectedLatest && <Button size="sm" variant="flat" onPress={() => setDetail(selectedLatest)}>打开最近报告</Button>}
                </div>
              </div>

              {editingDomain === selectedGroup.domain && (
                <div className="flex flex-wrap items-center gap-2 rounded-medium border border-default-200 bg-default-50/60 p-2">
                  <Input
                    size="sm"
                    autoFocus
                    maxLength={120}
                    className="min-w-[220px] flex-1"
                    placeholder="输入备注，留空保存即可清除；显示为 备注-域名"
                    value={noteDraft}
                    onValueChange={setNoteDraft}
                    onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); void saveNote(selectedGroup.domain); } }}
                  />
                  <Button size="sm" color="primary" isLoading={savingNote} onPress={() => void saveNote(selectedGroup.domain)}>保存</Button>
                  <Button size="sm" variant="light" onPress={() => setEditingDomain("")}>取消</Button>
                </div>
              )}

              <div className="flex items-end justify-between gap-2 border-b border-default-200 pb-2">
                <div>
                  <h3 className="text-sm font-semibold">全部任务</h3>
                  <p className="text-[11px] text-default-400">{selectedGroup.run_count} 次测试记录，按提交时间倒序</p>
                </div>
              </div>

              <div className="flex max-h-[46vh] flex-col gap-2 overflow-y-auto pr-1">
                <div className="hidden xl:grid xl:grid-cols-[72px_128px_minmax(130px,1.2fr)_minmax(80px,.7fr)_minmax(90px,.8fr)_100px_150px] gap-2 border-b border-default-200 px-2 pb-2 text-[11px] text-default-400">
                  <span>状态</span><span>任务 ID</span><span>等级</span><span>模型</span><span>范围</span><span>时间</span><span className="text-right">操作</span>
                </div>
                {selectedGroup.runs.map((run) => (
                  <div key={run.id} className="grid gap-2 rounded-medium border border-default-100 px-2 py-2 hover:bg-default-50 xl:grid-cols-[72px_128px_minmax(130px,1.2fr)_minmax(80px,.7fr)_minmax(90px,.8fr)_100px_150px] xl:items-center">
                    <Chip size="sm" variant="flat" color={statusColor[run.status]} className="h-5 w-fit text-[10px]">{statusLabel[run.status]}</Chip>
                    <span className="select-all whitespace-nowrap font-mono text-xs text-default-500">{run.id}</span>
                    <div className="min-w-0">
                      {run.grade ? <Chip size="sm" variant="flat" color={gradeColor(run.grade)} className="h-5 text-[10px]">{run.grade}</Chip> : <span className="text-xs text-default-400">—</span>}
                      {run.task_note && <p className="mt-0.5 truncate text-[11px] text-default-500">{run.task_note}</p>}
                    </div>
                    <span className="truncate text-sm">{run.model}</span>
                    <div className="flex flex-wrap gap-1">
                      {(suite ? rangeSummary(run, suite) : []).slice(0, 3).map((item) => <Chip key={item.key} size="sm" variant="flat" className="h-5 text-[10px]">{item.label} {item.count}</Chip>)}
                    </div>
                    <span className="text-xs text-default-400">{displayTime(run.created_at)}</span>
                    <div className="flex flex-wrap justify-end gap-1">
                      <Button size="sm" variant="light" onPress={() => { setEditingTaskID(run.id); setTaskNoteDraft(run.task_note || ""); }}>{run.task_note ? "改任务备注" : "任务备注"}</Button>
                      <Button size="sm" variant="flat" onPress={() => setDetail(run)}>详情</Button>
                      {run.status === "running" && <Button size="sm" variant="flat" color="warning" onPress={() => act(pauseRun, run.id, "已请求暂停")}>暂停</Button>}
                      {["paused", "aborted"].includes(run.status) && <Button size="sm" variant="flat" color="primary" onPress={() => act(resumeRun, run.id, "已继续")}>继续</Button>}
                      {["queued", "running", "pausing", "paused"].includes(run.status) && <Button size="sm" variant="flat" color="danger" onPress={() => act(abortRun, run.id, "已终止")}>终止</Button>}
                    </div>
                    {editingTaskID === run.id && (
                      <div className="col-span-full flex flex-wrap items-center gap-2 border-t border-default-100 pt-2">
                        <Input
                          size="sm"
                          autoFocus
                          maxLength={120}
                          className="min-w-[220px] flex-1"
                          placeholder="本次任务备注，如：二次复测；留空保存可清除"
                          value={taskNoteDraft}
                          onValueChange={setTaskNoteDraft}
                          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); void saveTaskNote(run.id); } }}
                        />
                        <Button size="sm" color="primary" isLoading={savingTaskNote} onPress={() => void saveTaskNote(run.id)}>保存</Button>
                        <Button size="sm" variant="light" onPress={() => setEditingTaskID("")}>取消</Button>
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
