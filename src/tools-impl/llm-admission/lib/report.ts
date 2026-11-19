import type { Run, SuiteCase, SuiteInfo, DomainGroup } from "../api";

export function displayTime(raw: string): string {
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return d.toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", hour12: false });
}

export function hostOf(raw: string): string {
  try {
    return new URL(raw).host || raw;
  } catch {
    return raw || "—";
  }
}

export function primaryDomainOf(run: Run): string {
  return run.primary_domain || hostOf(run.base_url);
}

export const statusColor: Record<string, "primary" | "warning" | "success" | "danger" | "default"> = {
  queued: "warning",
  running: "primary",
  pausing: "warning",
  paused: "warning",
  succeeded: "success",
  failed: "danger",
  aborted: "default",
};

export const statusLabel: Record<string, string> = {
  queued: "排队中",
  running: "运行中",
  pausing: "暂停中",
  paused: "已暂停",
  succeeded: "已完成",
  failed: "未通过",
  aborted: "已终止",
};

export const caseColor: Record<string, "success" | "danger" | "primary" | "default" | "warning"> = {
  passed: "success",
  failed: "danger",
  running: "primary",
  pending: "default",
  exempt: "warning",
  skipped: "warning",
};

export const caseLabel: Record<string, string> = {
  passed: "通过",
  failed: "失败",
  running: "运行中",
  pending: "待执行",
  exempt: "跳过",
  skipped: "跳过",
};

export function gradeColor(grade?: string): "success" | "primary" | "warning" | "danger" | "default" {
  switch (grade) {
    case "A+":
    case "A":
      return "success";
    case "B":
      return "primary";
    case "C":
      return "warning";
    case "D":
      return "danger";
    default:
      return "default";
  }
}

export function gradeOf(run?: { grade?: string; pass_rate?: number; assessed?: number; passed?: number }): string {
  if (!run) return "—";
  if (run.grade) return run.grade;
  if (!run.assessed) return "—";
  const rate = run.pass_rate ?? ((run.passed ?? 0) / run.assessed) * 100;
  if (rate >= 95) return "A+";
  if (rate >= 90) return "A";
  if (rate >= 80) return "B";
  if (rate >= 70) return "C";
  return "D";
}

export const levelNote: Record<string, string> = {
  P0: "P0 否决项",
  P1: "P1 必过项",
  P2: "P2 记录项",
  P3: "P3 可选项",
  gate: "前置闸门",
  required: "P1 必过项",
  record: "P2 记录项",
  optional: "P3 可选项",
};

export type CaseCategory = "video" | "image" | "latency" | "cache" | "gate" | "format";
export const categoryOrder: CaseCategory[] = ["video", "image", "latency", "cache", "gate", "format"];
export const categoryLabel: Record<CaseCategory, string> = {
  video: "视频",
  image: "图片",
  latency: "延迟",
  cache: "缓存",
  gate: "闸门",
  format: "格式",
};
const videoCaseIDs = new Set(["M-30", "M-30B", "M-31", "M-B2"]);
const imageCaseIDs = new Set(["F-10", "M-28", "M-29"]);

export function caseCategory(cs: SuiteCase): CaseCategory {
  if (cs.capability === "video" || videoCaseIDs.has(cs.id)) return "video";
  if (cs.capability === "image_url" || imageCaseIDs.has(cs.id)) return "image";
  if (cs.group === "latency") return "latency";
  if (cs.group === "cache") return "cache";
  if (cs.group === "gate") return "gate";
  return "format";
}

export function rangeSummary(run: Run, suite: SuiteInfo) {
  let cases: SuiteCase[];
  if (run.case_ids) {
    const ids = new Set(run.case_ids.split(",").filter(Boolean));
    cases = suite.cases.filter((c) => ids.has(c.id));
  } else if (run.preset === "video") {
    cases = suite.cases.filter((c) => c.group === "gate" || c.capability === "video");
  } else if (run.preset) {
    cases = suite.cases.filter((c) => c.group === run.preset || c.group === "gate");
  } else {
    cases = suite.cases;
  }
  const counts = new Map<CaseCategory, number>();
  for (const cs of cases) {
    const cat = caseCategory(cs);
    counts.set(cat, (counts.get(cat) ?? 0) + 1);
  }
  return categoryOrder.filter((cat) => counts.has(cat)).map((cat) => ({ key: cat, label: categoryLabel[cat], count: counts.get(cat) ?? 0 }));
}

export function groupStatus(group: DomainGroup): string {
  const statuses = new Set(group.runs.map((r) => r.status));
  if (statuses.has("running") || statuses.has("pausing")) return "running";
  if (statuses.has("queued")) return "queued";
  if (statuses.has("paused")) return "paused";
  return group.last_completed?.status || "queued";
}

export function groupVerdict(group: DomainGroup): string {
  return group.last_completed?.verdict || "";
}

export function failedLevelText(group: DomainGroup): string {
  const levels = ["P0", "P1", "P2", "P3"].filter((l) => (group.failed_by_level?.[l] ?? 0) > 0);
  if (levels.length === 0) return "";
  return levels.map((l) => `${l}×${group.failed_by_level[l]}`).join("、");
}
