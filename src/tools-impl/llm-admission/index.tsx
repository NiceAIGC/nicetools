import { useEffect, useState } from "react";
import type { MouseEvent } from "react";
import {
  addToast,
  Button,
  Card,
  CardBody,
  Checkbox,
  Chip,
  Input,
  Select,
  SelectItem,
  Spinner,
} from "@heroui/react";

import {
  createRun,
  listDomainGroups,
  listSuites,
  type SuiteCase,
  type SuiteInfo,
} from "./api";
import ReportPage from "./components/ReportPage";
import {
  caseCategory,
  categoryLabel,
  levelNote,
} from "./lib/report";

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

function normalizeGateway(raw: string): string {
  const value = raw.trim();
  if (!value) return value;
  try {
    const u = new URL(value);
    const path = u.pathname.replace(/\/$/, "");
    if (!/\/v1$/i.test(path) && !/\/openai$/i.test(path)) u.pathname = `${path}/v1`;
    return u.toString().replace(/\/$/, "");
  } catch {
    return value.replace(/\/$/, "");
  }
}

function caseInPreset(cs: SuiteCase, preset: string): boolean {
  if (cs.group === "gate" || !preset) return true;
  if (preset === "video") return cs.capability === "video";
  return cs.group === preset;
}

export default function LlmAdmission() {
  const [suites, setSuites] = useState<SuiteInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<"new" | "reports">(() => {
    try { return new URLSearchParams(window.location.search).get("view") === "reports" ? "reports" : "new"; } catch { return "new"; }
  });
  const [reportStats, setReportStats] = useState({ runs: 0, active: 0 });
  const [knownModels, setKnownModels] = useState<{ name: string; count: number }[]>([]);
  const [showKey, setShowKey] = useState(false);

  const savedTarget = useState(loadTarget)[0];
  const [suiteId, setSuiteId] = useState("");
  const [baseURL, setBaseURL] = useState(savedTarget.base_url);
  const [apiKey, setAPIKey] = useState(savedTarget.api_key);
  const [channel, setChannel] = useState(savedTarget.channel);
  const [model, setModel] = useState(savedTarget.model);
  const [preset, setPreset] = useState("");
  const [selectedCases, setSelectedCases] = useState<Set<string>>(new Set());
  const [caseQuery, setCaseQuery] = useState("");
  const [caseCategoryFilter, setCaseCategoryFilter] = useState("all");
  const [casesOpen, setCasesOpen] = useState(false);
  const [lastClicked, setLastClicked] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    listSuites()
      .then((list) => {
        setSuites(list);
        if (list[0]) {
          setSuiteId(list[0].id);
          setModel((m) => m || list[0].default_model);
        }
      })
      .catch((e) => addToast({ title: e.message, color: "danger" }))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    try {
      const p = new URLSearchParams(window.location.search);
      if (view === "reports") p.set("view", "reports");
      else p.delete("view");
      const next = `${window.location.pathname}${p.toString() ? `?${p}` : ""}`;
      window.history.replaceState(null, "", next);
    } catch {
      // URL 不可写时忽略
    }
  }, [view]);

  useEffect(() => {
    listDomainGroups()
      .then((res) => {
        const counts = new Map<string, number>();
        for (const group of res.groups) for (const run of group.runs) counts.set(run.model, (counts.get(run.model) ?? 0) + 1);
        setKnownModels([...counts.entries()].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name)));
      })
      .catch(() => {
        // 历史模型只做建议，不影响新建测试。
      });
  }, []);

  const suite = suites.find((s) => s.id === suiteId) ?? suites[0];
  const groupRank = ["gate", "format", "cache", "latency"];
  const allCases = suite?.cases ?? [];
  const presetCases = allCases.filter((c) => caseInPreset(c, preset));
  const visibleCases = allCases
    .filter((c) => caseCategoryFilter === "all" || caseCategory(c) === caseCategoryFilter)
    .filter((c) => {
      const q = caseQuery.trim().toLowerCase();
      return !q || `${c.id} ${c.name} ${c.doc.purpose}`.toLowerCase().includes(q);
    })
    .sort((a, b) => groupRank.indexOf(a.group) - groupRank.indexOf(b.group));
  const defaultCaseIDs = new Set(
    presetCases
      .filter((c) => c.group === "gate" || !c.default_skip)
      .map((c) => c.id),
  );
  const capabilityLabel = Object.fromEntries((suite?.capabilities ?? []).map((c) => [c.key, c.label]));

  useEffect(() => {
    if (!suite) return;
    setSelectedCases(new Set(
      suite.cases
        .filter((c) => caseInPreset(c, preset))
        .filter((c) => c.group === "gate" || !c.default_skip)
        .map((c) => c.id),
    ));
    setLastClicked(null);
  }, [suite?.id, preset]);

  const toggleCase = (cs: SuiteCase, on: boolean) => {
    setSelectedCases((prev) => {
      const next = new Set(prev);
      if (on) next.add(cs.id);
      else next.delete(cs.id);
      return next;
    });
    setLastClicked(cs.id);
  };

  const selectAll = () => {
    setSelectedCases((prev) => {
      const next = new Set(prev);
      for (const c of visibleCases) next.add(c.id);
      return next;
    });
    setLastClicked(null);
  };
  const selectNone = () => {
    setSelectedCases((prev) => {
      const next = new Set(prev);
      for (const c of visibleCases) next.delete(c.id);
      return next;
    });
    setLastClicked(null);
  };
  const invertSelection = () => {
    setSelectedCases((prev) => {
      const next = new Set(prev);
      for (const c of visibleCases) {
        if (next.has(c.id)) next.delete(c.id);
        else next.add(c.id);
      }
      return next;
    });
    setLastClicked(null);
  };
  const resetDefault = () => {
    setSelectedCases(new Set(defaultCaseIDs));
    setLastClicked(null);
  };

  const applyRange = (cs: SuiteCase, on: boolean) => {
    if (!lastClicked) {
      toggleCase(cs, on);
      return;
    }
    const ids = visibleCases.map((c) => c.id);
    const from = ids.indexOf(lastClicked);
    const to = ids.indexOf(cs.id);
    if (from < 0 || to < 0) {
      toggleCase(cs, on);
      return;
    }
    const [start, end] = from < to ? [from, to] : [to, from];
    const next = new Set(selectedCases);
    for (let i = start; i <= end; i++) {
      if (on) next.add(ids[i]);
      else next.delete(ids[i]);
    }
    setSelectedCases(next);
    setLastClicked(cs.id);
  };

  const handleRowClick = (e: MouseEvent, cs: SuiteCase) => {
    if ((e.target as HTMLElement).closest("[data-case-checkbox]")) return;
    const on = !selectedCases.has(cs.id);
    if (e.shiftKey) applyRange(cs, on);
    else toggleCase(cs, on);
  };

  const submit = async () => {
    if (!suite) return;
    setSubmitting(true);
    try {
      const selectedIDs = [...selectedCases].filter((id) => allCases.some((c) => c.id === id));
      if (selectedIDs.length === 0) {
        addToast({ title: "至少选择一个用例", color: "warning" });
        return;
      }
      await createRun({
        base_url: baseURL,
        api_key: apiKey,
        model,
        channel,
        suite: suite.id,
        preset,
        declarations: {},
        case_overrides: {},
        case_ids: selectedIDs,
      });
      localStorage.setItem(TARGET_KEY, JSON.stringify({ base_url: baseURL, api_key: apiKey, model, channel }));
      addToast({ title: "测试已启动，正在加入队列" });
      setView("reports");
    } catch (e) {
      addToast({ title: (e as Error).message, color: "danger" });
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) return <div className="flex justify-center py-16"><Spinner /></div>;
  if (!suite) return <p className="py-16 text-center text-sm text-default-500">没有可用的测试标准</p>;

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <h2 className="text-xl font-semibold">大模型准入测试</h2>
          <p className="mt-1 text-xs text-default-400">{suite.name} · 按分组执行，失败不终止，最终汇总裁决</p>
        </div>
        <div className="ml-auto inline-flex rounded-full border border-default-200 bg-default-50 p-1">
          <Button size="sm" radius="full" variant={view === "new" ? "solid" : "light"} color={view === "new" ? "primary" : "default"} onPress={() => setView("new")}>提交测试</Button>
          <Button size="sm" radius="full" variant={view === "reports" ? "solid" : "light"} color={view === "reports" ? "primary" : "default"} onPress={() => setView("reports")}>
            测试报告{reportStats.runs > 0 ? ` (${reportStats.runs})` : ""}
          </Button>
        </div>
      </div>

      {view === "new" ? (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
          <Card shadow="none" className="border border-default-200">
            <CardBody className="gap-4">
              <div>
                <h3 className="text-lg font-semibold">测试目标</h3>
                <p className="mt-1 text-xs text-default-400">网关地址与模型进入报告；密钥只存本机浏览器。</p>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <Input label="名称 / 渠道" placeholder="如：yropc K3" value={channel} onValueChange={setChannel} description="显示在报告域名组上" />
                <Input label="网关地址" placeholder="https://api.example.com/v1" value={baseURL} onValueChange={setBaseURL} onBlur={() => setBaseURL((v) => normalizeGateway(v))} isRequired />
                <Input
                  label="密钥"
                  type={showKey ? "text" : "password"}
                  placeholder="sk-…（仅存本机浏览器）"
                  value={apiKey}
                  onValueChange={setAPIKey}
                  isRequired
                  endContent={
                    <button
                      type="button"
                      aria-label={showKey ? "隐藏密钥" : "显示密钥"}
                      className="text-xs text-default-400 hover:text-default-600"
                      onClick={() => setShowKey((v) => !v)}
                    >
                      {showKey ? "隐藏" : "显示"}
                    </button>
                  }
                />
                <div>
                  <Input
                    label="模型名称"
                    list="known-models"
                    value={model}
                    onValueChange={setModel}
                    description={knownModels.length ? "下拉选择历史测试模型，按测试次数降序；也可直接输入" : "可直接输入模型名称"}
                    isRequired
                  />
                  <datalist id="known-models">
                    {knownModels.map((item) => <option key={item.name} value={item.name}>{`${item.name}（${item.count} 次）`}</option>)}
                  </datalist>
                </div>
                <Select label="测试标准" selectedKeys={[suite.id]} onChange={(e) => {
                  const next = suites.find((s) => s.id === e.target.value);
                  if (!next) return;
                  setSuiteId(next.id);
                  setModel(next.default_model);
                  setPreset("");
                }}>
                  {suites.map((s) => <SelectItem key={s.id}>{s.name}</SelectItem>)}
                </Select>
                <Select label="测试范围" selectedKeys={[preset || "all"]} onChange={(e) => setPreset(e.target.value === "all" ? "" : e.target.value)}>
                  {suite.presets.map((p) => <SelectItem key={p.key || "all"} description={p.hint}>{p.label}</SelectItem>)}
                </Select>
              </div>

              <div className="rounded-large border border-default-200">
                <button type="button" className="flex w-full items-center gap-3 px-4 py-3 text-left" onClick={() => setCasesOpen((v) => !v)} aria-expanded={casesOpen}>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold">用例范围</p>
                    <p className="text-xs text-default-400">已选 {selectedCases.size} 项；默认跳过可选/长耗时用例</p>
                  </div>
                  <Chip size="sm" variant="flat">{casesOpen ? "收起" : "展开"}</Chip>
                </button>
                {casesOpen && (
                  <div className="border-t border-default-200">
                    <div className="flex flex-col gap-2 px-3 py-3 sm:flex-row sm:flex-wrap sm:items-center">
                      <Input size="sm" className="w-full sm:w-64" placeholder="搜索用例 ID / 名称 / 说明" value={caseQuery} onValueChange={setCaseQuery} isClearable />
                      <Select aria-label="用例类别" size="sm" className="w-full sm:w-36" selectedKeys={[caseCategoryFilter]} onChange={(e) => setCaseCategoryFilter(e.target.value)}>
                        <SelectItem key="all">全部类别</SelectItem>
                        <SelectItem key="video">视频</SelectItem>
                        <SelectItem key="image">图片</SelectItem>
                        <SelectItem key="latency">延迟</SelectItem>
                        <SelectItem key="cache">缓存</SelectItem>
                        <SelectItem key="format">格式</SelectItem>
                        <SelectItem key="gate">闸门</SelectItem>
                      </Select>
                      <div className="flex flex-wrap gap-2">
                        <Button size="sm" variant="flat" onPress={selectAll}>全选</Button>
                        <Button size="sm" variant="flat" onPress={selectNone}>全不选</Button>
                        <Button size="sm" variant="flat" onPress={invertSelection}>反选</Button>
                        <Button size="sm" variant="light" onPress={resetDefault}>恢复默认</Button>
                      </div>
                    </div>
                    <p className="px-3 pb-2 text-xs text-default-400">全选/全不选/反选只作用于当前筛选结果；Shift + 点击可范围选择。勾选能力用例会自动声明对应能力。</p>
                    <div className="max-h-[60vh] overflow-y-auto">
                      {visibleCases.length === 0 && <p className="px-4 py-8 text-center text-sm text-default-400">没有匹配的用例</p>}
                      {visibleCases.map((cs) => {
                        const checked = selectedCases.has(cs.id);
                        return (
                          <div key={cs.id} data-case-id={cs.id} className={`flex cursor-pointer items-start gap-3 border-b border-default-100 px-3 py-3 last:border-0 hover:bg-default-50 sm:px-4 ${checked ? "bg-default-50/60" : ""}`} onClick={(e) => handleRowClick(e, cs)}>
                            <span data-case-checkbox className="pt-0.5" onClick={(e) => e.stopPropagation()}>
                              <Checkbox aria-label={`${cs.name} 测试`} isSelected={checked} onValueChange={(on) => toggleCase(cs, on)} />
                            </span>
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <span className="font-mono text-xs text-default-400">{cs.id}</span>
                                <span className="min-w-0 max-w-full truncate text-sm font-medium sm:max-w-[26rem]">{cs.name}</span>
                                <Chip size="sm" variant="flat">{categoryLabel[caseCategory(cs)]}</Chip>
                                <Chip size="sm" variant="bordered" color={cs.level === "P0" ? "danger" : cs.level === "P1" || cs.level === "required" ? "default" : "warning"}>
                                  {cs.level === "gate" ? "闸门" : levelNote[cs.level] ?? cs.level}
                                </Chip>
                                {cs.default_skip && <Chip size="sm" color="warning" variant="flat">可选/长耗时</Chip>}
                                {cs.capability && <Chip size="sm" variant="flat">能力：{capabilityLabel[cs.capability] ?? cs.capability}</Chip>}
                              </div>
                              <p className="mt-1 line-clamp-2 text-xs text-default-400">{cs.doc.purpose}</p>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            </CardBody>
          </Card>

          <div className="flex h-fit flex-col gap-3 rounded-large border border-default-200 bg-default-50/60 p-4 xl:sticky xl:top-20">
            <h3 className="text-sm font-semibold">提交摘要</h3>
            <div className="flex flex-col gap-1 text-sm">
              <div className="flex justify-between gap-3"><span className="text-default-400">名称</span><span className="truncate text-right">{channel || "未命名"}</span></div>
              <div className="flex justify-between gap-3"><span className="text-default-400">域名</span><span className="truncate text-right font-mono">{(() => { try { return new URL(baseURL).host || "—"; } catch { return "—"; } })()}</span></div>
              <div className="flex justify-between gap-3"><span className="text-default-400">模型</span><span className="truncate text-right">{model || "—"}</span></div>
              <div className="flex justify-between gap-3"><span className="text-default-400">范围</span><span className="text-right">{preset ? suite.presets.find((p) => p.key === preset)?.label : "全部"}</span></div>
              <div className="flex justify-between gap-3"><span className="text-default-400">用例</span><span className="tabular-nums">{selectedCases.size} 项</span></div>
            </div>
            <p className="text-xs text-default-400">提交后自动排队；暂停任务不会阻塞队列。</p>
            <Button color="primary" isLoading={submitting} isDisabled={!baseURL || !apiKey || !model || selectedCases.size === 0} onPress={submit}>开始测试</Button>
            {reportStats.active > 0 && <p className="text-xs text-warning">当前有 {reportStats.active} 个活跃任务，新任务会排队。</p>}
          </div>
        </div>
      ) : (
        <ReportPage suites={suites} onStats={setReportStats} />
      )}
    </div>
  );
}
