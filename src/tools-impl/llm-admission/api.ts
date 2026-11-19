import { json, toolApi } from "../../api/client";

const ID = "llm-admission";

export interface SuiteCase {
  id: string;
  name: string;
  level: string;
  group: string;
  capability?: string;
  default_skip?: boolean;
  doc: { purpose: string; method: string; expect: string; ref: string };
}

export interface SuiteInfo {
  id: string;
  name: string;
  default_model: string;
  capabilities: { key: string; label: string }[];
  cases: SuiteCase[];
  presets: { key: string; label: string; hint: string }[];
}

export interface Run {
  id: string;
  created_by: string;
  suite: string;
  suite_name: string;
  base_url: string;
  primary_domain?: string;
  model: string;
  channel: string;
  preset: string;
  declarations: string;
  case_ids?: string;
  status: "queued" | "running" | "pausing" | "paused" | "succeeded" | "failed" | "aborted";
  verdict: string;
  stop_reason?: string;
  total_cases: number;
  done_cases: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  passed?: number;
  failed?: number;
  assessed?: number;
  pass_rate?: number;
  grade?: string;
}

export interface DomainGroup {
  domain: string;
  name: string;
  hosts: string[];
  models: string[];
  channels: string[];
  run_count: number;
  active_count: number;
  latest?: Run;
  last_completed?: Run;
  failed_by_level: Record<string, number>;
  runs: Run[];
}

export interface CaseRow {
  id: number;
  seq: number;
  case_id: string;
  group: string;
  name: string;
  level: string;
  status: "pending" | "running" | "passed" | "failed" | "exempt" | "skipped";
  http_status: number;
  duration_ms: number;
  retries: number;
  summary: string;
  metrics?: string;
  defects: string;
}

export interface CaseEvidenceItem {
  label: string;
  request: string;
  request_headers?: Record<string, string>;
  response: string;
  response_headers?: Record<string, string>;
  http_status: number;
  finish_reason?: string;
  duration_ms?: number;
  retries?: number;
}

export interface CaseEvidence {
  case_id: string;
  status: string;
  summary: string;
  doc: { purpose: string; method: string; expect: string; ref: string };
  defects: { level: string; expect: string; actual: string; attribution: string }[];
  explanation: string;
  evidence: CaseEvidenceItem[];
}

export interface CreateInput {
  base_url: string;
  api_key: string;
  model: string;
  channel: string;
  suite: string;
  preset: string;
  declarations: Record<string, boolean>;
  case_overrides: Record<string, "skip" | "exempt">;
  case_ids?: string[];
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

export function exportUrl(base: string, type: ExportType, format: ExportFormat, detail: ExportDetail, channel?: string, name?: string, watermark = true) {
  const params = new URLSearchParams({ type, format });
  if (detail === "brief") params.set("detail", "brief");
  if (!watermark) params.set("watermark", "0");
  if (channel) params.set("channel", channel);
  if (name) params.set("name", name);
  return `${base}${base.includes("?") ? "&" : "?"}${params}`;
}

export const listSuites = () => toolApi<SuiteInfo[]>(ID, "/suites");
export const listRuns = () => toolApi<Run[]>(ID, "/runs");
export const listDomainGroups = () => toolApi<{ groups: DomainGroup[] }>(ID, "/runs/domains");
export const runDetail = (id: string) => toolApi<{ run: Run; cases: CaseRow[] }>(ID, `/runs/${id}`);
export const caseEvidence = (runId: string, caseId: string) => toolApi<CaseEvidence>(ID, `/runs/${runId}/cases/${encodeURIComponent(caseId)}/evidence`);
export const createRun = (input: CreateInput) => toolApi<Run>(ID, "/runs", json("POST", input));
export const pauseRun = (id: string) => toolApi(ID, `/runs/${id}/pause`, json("POST", {}));
export const resumeRun = (id: string) => toolApi(ID, `/runs/${id}/resume`, json("POST", {}));
export const abortRun = (id: string) => toolApi(ID, `/runs/${id}/abort`, json("POST", {}));

export function exportFailedEvidence(id: string, format: "md" | "txt" = "md") {
  window.open(`/api/tools/${ID}/runs/${id}/export/failed?format=${format}`, "_blank");
}

export function exportRun(id: string, type: ExportType, format: ExportFormat, detail: ExportDetail, channel?: string, name?: string, watermark = true) {
  window.open(exportUrl(`/api/tools/${ID}/runs/${id}/export`, type, format, detail, channel, name, watermark), "_blank");
}

export function exportMerged(ids: string[], type: ExportType, format: ExportFormat, detail: ExportDetail, channel?: string, name?: string, watermark = true) {
  window.open(exportUrl(`/api/tools/${ID}/export/merged?ids=${ids.join(",")}`, type, format, detail, channel, name, watermark), "_blank");
}
