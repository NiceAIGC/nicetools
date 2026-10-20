import { json, toolApi } from "../../api/client";

const ID = "llm-admission";

export interface SuiteCase {
  id: string;
  name: string;
  level: string;
  capability?: string;
  doc: { purpose: string; method: string; expect: string; ref: string };
}

export interface SuiteInfo {
  id: string;
  name: string;
  default_model: string;
  capabilities: { key: string; label: string }[];
  tiers: { id: string; name: string; hint: string; cases: SuiteCase[] }[];
}

export interface Run {
  id: string;
  created_by: string;
  suite: string;
  suite_name: string;
  base_url: string;
  model: string;
  channel: string;
  tier: string;
  declarations: string;
  status: "running" | "pausing" | "paused" | "succeeded" | "failed" | "aborted";
  verdict: string;
  stop_reason?: string;
  total_cases: number;
  done_cases: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
}

export interface CaseRow {
  id: number;
  seq: number;
  case_id: string;
  tier: string;
  name: string;
  level: string;
  status: "pending" | "running" | "passed" | "failed" | "exempt" | "skipped";
  http_status: number;
  duration_ms: number;
  retries: number;
  summary: string;
  defects: string;
}

export interface CreateInput {
  base_url: string;
  api_key: string;
  model: string;
  channel: string;
  suite: string;
  tier: string;
  declarations: Record<string, boolean>;
  case_overrides: Record<string, "skip" | "exempt">;
}

export type ExportFormat = "json" | "md" | "pdf";
export type ExportDetail = "full" | "brief";
export type ExportType = "full" | "masked";

export interface CaseCfg {
  case_id: string;
  mode: "" | "skip" | "exempt";
  level: "" | "P0" | "P1" | "P2" | "P3";
}

export const listCaseCfg = () => toolApi<CaseCfg[]>(ID, "/config");
export const saveCaseCfg = (suite: string, cfgs: CaseCfg[]) =>
  toolApi(ID, "/config", json("PUT", { suite, cfgs }));

export function exportUrl(base: string, type: ExportType, format: ExportFormat, detail: ExportDetail, channel?: string) {
  const params = new URLSearchParams({ type, format });
  if (detail === "brief") params.set("detail", "brief");
  if (channel) params.set("channel", channel);
  return `${base}?${params}`;
}

export const listSuites = () => toolApi<SuiteInfo[]>(ID, "/suites");
export const listRuns = () => toolApi<Run[]>(ID, "/runs");
export const runDetail = (id: string) => toolApi<{ run: Run; cases: CaseRow[] }>(ID, `/runs/${id}`);
export const createRun = (input: CreateInput) => toolApi<Run>(ID, "/runs", json("POST", input));
export const pauseRun = (id: string) => toolApi(ID, `/runs/${id}/pause`, json("POST", {}));
export const resumeRun = (id: string) => toolApi(ID, `/runs/${id}/resume`, json("POST", {}));
export const abortRun = (id: string) => toolApi(ID, `/runs/${id}/abort`, json("POST", {}));

export function exportRun(id: string, type: ExportType, format: ExportFormat, detail: ExportDetail, channel?: string) {
  window.open(exportUrl(`/api/tools/${ID}/runs/${id}/export`, type, format, detail, channel), "_blank");
}

export function exportMerged(ids: string[], type: ExportType, format: ExportFormat, detail: ExportDetail, channel?: string) {
  window.open(exportUrl(`/api/tools/${ID}/export/merged?ids=${ids.join(",")}`, type, format, detail, channel), "_blank");
}
