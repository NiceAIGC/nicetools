// OpenAI / Claude 两种接口格式的连通性测试：请求构造、curl 生成、SSE 解析。
export type ApiFormat = "openai" | "anthropic";

export interface TargetConfig {
  format: ApiFormat;
  baseUrl: string;
  apiKey: string;
  model: string;
  prompt: string;
  /** OpenAI 格式留空则不发送；Claude 格式必填 */
  maxTokens: number | null;
  stream: boolean;
  timeoutMs: number;
  /** 追加请求头的 JSON 文本 */
  extraHeaders: string;
  /** Claude 格式改用 Authorization: Bearer（部分网关要求） */
  bearerForAnthropic: boolean;
}

export interface TokenUsage {
  inputTokens: number | null;
  outputTokens: number | null;
  totalTokens: number | null;
}

export interface TestResult {
  ok: boolean;
  status: number | null;
  url: string;
  latencyMs: number;
  firstTokenMs: number | null;
  text: string;
  raw: string;
  chunks: number;
  usage: TokenUsage | null;
  error: string | null;
  /** 请求根本没到服务端（CORS / DNS / 超时 / 取消） */
  networkError: boolean;
  headers: Record<string, string>;
  body: string;
}

export interface BuiltRequest {
  url: string;
  headers: Record<string, string>;
  body: string;
  error: string | null;
}

const ANTHROPIC_VERSION = "2023-06-01";

/**
 * Base URL 补全规则（最终地址会展示在界面上，避免猜错）：
 * - 已包含完整终点（/chat/completions、/messages）时原样使用；
 * - 只给域名（如 https://api.openai.com）时补全 /v1 + 终点；
 * - 已带路径（如 https://open.bigmodel.cn/api/paas/v4）时只补终点。
 */
export function resolveEndpoint(format: ApiFormat, baseUrl: string): string {
  const trimmed = baseUrl.trim().replace(/\/+$/, "");
  if (!trimmed) return "";

  const [full, tail] =
    format === "anthropic" ? ["/v1/messages", "/messages"] : ["/v1/chat/completions", "/chat/completions"];

  if (trimmed.endsWith(full) || trimmed.endsWith(tail)) return trimmed;

  const hasPath = /^[a-zA-Z][a-zA-Z\d+.-]*:\/\/[^/]+\/.+/.test(trimmed);
  return hasPath ? `${trimmed}${tail}` : `${trimmed}${full}`;
}

export function parseExtraHeaders(text: string): { headers: Record<string, string>; error: string | null } {
  const source = text.trim();
  if (!source) return { headers: {}, error: null };

  let parsed: unknown;
  try {
    parsed = JSON.parse(source);
  } catch {
    return { headers: {}, error: "追加请求头不是合法 JSON" };
  }

  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return { headers: {}, error: "追加请求头需要是 JSON 对象，例如 {\"X-Title\":\"NiceTools\"}" };
  }

  const headers: Record<string, string> = {};
  for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
    headers[key] = typeof value === "string" ? value : JSON.stringify(value);
  }
  return { headers, error: null };
}

export function buildRequest(config: TargetConfig): BuiltRequest {
  const empty: BuiltRequest = { url: "", headers: {}, body: "", error: null };
  if (!config.baseUrl.trim()) return { ...empty, error: "请填写 Base URL" };
  if (!config.model.trim()) return { ...empty, error: "请填写模型名称" };

  const url = resolveEndpoint(config.format, config.baseUrl);
  const extra = parseExtraHeaders(config.extraHeaders);
  if (extra.error) return { ...empty, url, error: extra.error };

  const key = config.apiKey.trim();
  const prompt = config.prompt.trim() || "你好";

  if (config.format === "anthropic") {
    const headers: Record<string, string> = {
      "content-type": "application/json",
      "anthropic-version": ANTHROPIC_VERSION,
      // 浏览器直连 Anthropic 官方接口必需
      "anthropic-dangerous-direct-browser-access": "true",
      ...extra.headers,
    };
    if (key) {
      if (config.bearerForAnthropic) headers.authorization = `Bearer ${key}`;
      else headers["x-api-key"] = key;
    }

    const body: Record<string, unknown> = {
      model: config.model.trim(),
      max_tokens: config.maxTokens ?? 64,
      messages: [{ role: "user", content: prompt }],
    };
    if (config.stream) body.stream = true;
    return { url, headers, body: JSON.stringify(body), error: null };
  }

  const headers: Record<string, string> = {
    "content-type": "application/json",
    ...extra.headers,
  };
  if (key) headers.authorization = `Bearer ${key}`;

  const body: Record<string, unknown> = {
    model: config.model.trim(),
    messages: [{ role: "user", content: prompt }],
  };
  if (config.stream) body.stream = true;
  if (config.maxTokens !== null) body.max_tokens = config.maxTokens;

  return { url, headers, body: JSON.stringify(body), error: null };
}

export function buildCurl(config: TargetConfig): string {
  const built = buildRequest(config);
  if (!built.error && built.url) {
    const lines = [`curl${config.stream ? " -N" : ""} -X POST '${built.url}'`];
    for (const [key, value] of Object.entries(built.headers)) {
      lines.push(`  -H '${key}: ${value}'`);
    }
    lines.push(`  -d '${built.body.replace(/'/g, "'\\''")}'`);
    return lines.join(" \\\n");
  }
  return `# ${built.error ?? "请先补全 Base URL 与模型名称"}`;
}

export function maskSecret(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length <= 8) return trimmed ? "****" : "";
  return `${trimmed.slice(0, 4)}****${trimmed.slice(-4)}`;
}

export function prettyJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function readNumber(source: Record<string, unknown>, key: string): number | null {
  const value = source[key];
  return typeof value === "number" ? value : null;
}

/** 流式用量分散在多个事件里（Claude 的输入/输出 tokens 分属 message_start 与 message_delta），逐字段补齐。 */
export function mergeUsage(previous: TokenUsage | null, next: TokenUsage): TokenUsage {
  return {
    inputTokens: next.inputTokens ?? previous?.inputTokens ?? null,
    outputTokens: next.outputTokens ?? previous?.outputTokens ?? null,
    totalTokens: next.totalTokens ?? previous?.totalTokens ?? null,
  };
}

export function extractUsage(format: ApiFormat, payload: unknown): TokenUsage | null {
  if (typeof payload !== "object" || payload === null) return null;
  const container = payload as Record<string, unknown>;
  const usage = (container.usage ?? (container.message as Record<string, unknown> | undefined)?.usage) as
    | Record<string, unknown>
    | undefined;
  if (!usage || typeof usage !== "object") return null;

  if (format === "anthropic") {
    return {
      inputTokens: readNumber(usage, "input_tokens"),
      outputTokens: readNumber(usage, "output_tokens"),
      totalTokens: null,
    };
  }

  const prompt = readNumber(usage, "prompt_tokens");
  const completion = readNumber(usage, "completion_tokens");
  const total = readNumber(usage, "total_tokens");
  return {
    inputTokens: prompt,
    outputTokens: completion,
    totalTokens: total ?? (prompt !== null && completion !== null ? prompt + completion : null),
  };
}

/** 从完整响应里取出正文 */
export function extractCompletion(
  format: ApiFormat,
  payload: unknown,
): { text: string; error: string | null } {
  if (typeof payload !== "object" || payload === null) {
    return { text: "", error: "响应不是 JSON 对象" };
  }
  const container = payload as Record<string, unknown>;

  const apiError = container.error;
  if (apiError) {
    const message =
      typeof apiError === "string"
        ? apiError
        : ((apiError as Record<string, unknown>).message as string | undefined);
    return { text: "", error: message ? `接口返回错误：${message}` : "接口返回错误" };
  }

  if (format === "anthropic") {
    const blocks = Array.isArray(container.content) ? container.content : [];
    const text = blocks
      .map((block) =>
        typeof block === "object" && block !== null && (block as Record<string, unknown>).type === "text"
          ? String((block as Record<string, unknown>).text ?? "")
          : "",
      )
      .join("");
    return { text, error: null };
  }

  const choices = Array.isArray(container.choices) ? container.choices : [];
  const first = choices[0] as Record<string, unknown> | undefined;
  if (!first) return { text: "", error: "响应缺少 choices 字段" };
  const message = first.message as Record<string, unknown> | undefined;
  const text =
    (typeof message?.content === "string" ? message.content : "") ||
    (typeof first.text === "string" ? first.text : "");
  return { text, error: null };
}

export interface StreamDelta {
  text: string;
  usage: TokenUsage | null;
  done: boolean;
  error: string | null;
}

/** 单条 SSE data: 负载解析，兼容 OpenAI 与 Anthropic 两种事件格式 */
export function parseStreamPayload(format: ApiFormat, payload: unknown): StreamDelta {
  const empty: StreamDelta = { text: "", usage: null, done: false, error: null };
  if (typeof payload !== "object" || payload === null) return empty;
  const container = payload as Record<string, unknown>;

  if (container.error) {
    const message =
      typeof container.error === "string"
        ? container.error
        : ((container.error as Record<string, unknown>).message as string | undefined);
    return { ...empty, error: message ?? "流式响应返回错误" };
  }

  if (format === "anthropic") {
    const type = container.type;
    if (type === "content_block_delta") {
      const delta = container.delta as Record<string, unknown> | undefined;
      return { ...empty, text: typeof delta?.text === "string" ? delta.text : "" };
    }
    if (type === "message_delta") return { ...empty, usage: extractUsage(format, container) };
    if (type === "message_start") return { ...empty, usage: extractUsage(format, container) };
    if (type === "message_stop") return { ...empty, done: true };
    return empty;
  }

  const choices = Array.isArray(container.choices) ? container.choices : [];
  const first = choices[0] as Record<string, unknown> | undefined;
  const delta = first?.delta as Record<string, unknown> | undefined;
  const text = typeof delta?.content === "string" ? delta.content : "";
  return {
    text,
    usage: extractUsage(format, container),
    done: false,
    error: null,
  };
}

export interface RunHandlers {
  signal?: AbortSignal;
  onText?: (text: string, chunks: number) => void;
}

interface StreamState {
  text: string;
  raw: string;
  chunks: number;
  usage: TokenUsage | null;
  firstTokenMs: number | null;
  error: string | null;
}

/** 逐块解析 SSE：按行处理 data: 负载，兼容 \n\n 与 \r\n 分隔。 */
function consumeStreamChunk(
  format: ApiFormat,
  buffer: string,
  state: StreamState,
  startedAt: number,
): string {
  const lines = buffer.split("\n");
  const rest = lines.pop() ?? "";

  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line) continue;
    state.raw += `${line}\n`;
    if (!line.startsWith("data:")) continue;

    const data = line.slice(5).trim();
    if (data === "[DONE]") continue;

    let payload: unknown;
    try {
      payload = JSON.parse(data);
    } catch {
      continue;
    }

    const delta = parseStreamPayload(format, payload);
    if (delta.usage) state.usage = mergeUsage(state.usage, delta.usage);
    if (delta.error) state.error = delta.error;
    if (!delta.text) continue;

    if (state.firstTokenMs === null) state.firstTokenMs = Math.round(performance.now() - startedAt);
    state.chunks += 1;
    state.text += delta.text;
  }

  return rest;
}

/** 执行一次连通性测试；取消或超时通过 AbortController 传递。 */
export async function runConnectivityTest(
  config: TargetConfig,
  handlers: RunHandlers = {},
): Promise<TestResult> {
  const built = buildRequest(config);
  const startedAt = performance.now();
  const result: TestResult = {
    ok: false,
    status: null,
    url: built.url,
    latencyMs: 0,
    firstTokenMs: null,
    text: "",
    raw: "",
    chunks: 0,
    usage: null,
    error: built.error,
    networkError: false,
    headers: {},
    body: built.body,
  };
  if (built.error) return result;

  const controller = new AbortController();
  let timedOut = false;
  let cancelled = false;
  const timer = window.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, Math.max(1000, config.timeoutMs));
  const forwardAbort = () => {
    cancelled = true;
    controller.abort();
  };
  handlers.signal?.addEventListener("abort", forwardAbort);

  try {
    let response: Response;
    try {
      response = await fetch(built.url, {
        method: "POST",
        headers: built.headers,
        body: built.body,
        signal: controller.signal,
      });
    } catch {
      result.latencyMs = Math.round(performance.now() - startedAt);
      result.networkError = true;
      result.error = cancelled
        ? "测试已取消"
        : timedOut
          ? `请求超时（${config.timeoutMs}ms）`
          : "请求未送达服务端：可能是 CORS 限制、Base URL 不可达，或 https 页面请求了 http 接口。请用下方 curl 在终端复测。";
      return result;
    }

    result.status = response.status;
    response.headers.forEach((value, key) => {
      result.headers[key] = value;
    });

    if (!response.ok) {
      const text = await response.text();
      result.raw = text;
      const parsed = extractCompletion(config.format, safeParse(text));
      result.error = parsed.error ?? `HTTP ${response.status} ${response.statusText}`.trim();
      result.latencyMs = Math.round(performance.now() - startedAt);
      return result;
    }

    if (!config.stream || !response.body) {
      const text = await response.text();
      result.raw = text;
      const payload = safeParse(text);
      if (payload === null) {
        result.error = "响应不是合法 JSON，请查看原始响应";
      } else {
        const parsed = extractCompletion(config.format, payload);
        result.text = parsed.text;
        result.error = parsed.error;
        result.usage = extractUsage(config.format, payload);
      }
      result.latencyMs = Math.round(performance.now() - startedAt);
      result.ok = result.error === null;
      return result;
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    const state: StreamState = {
      text: "",
      raw: "",
      chunks: 0,
      usage: null,
      firstTokenMs: null,
      error: null,
    };
    let buffer = "";
    let lastPush = 0;

    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      buffer = consumeStreamChunk(config.format, buffer, state, startedAt);

      const now = performance.now();
      if (now - lastPush > 90) {
        lastPush = now;
        handlers.onText?.(state.text, state.chunks);
      }
    }

    result.text = state.text;
    result.raw = state.raw;
    result.chunks = state.chunks;
    result.usage = state.usage;
    result.firstTokenMs = state.firstTokenMs;
    result.error = state.error ?? (state.text ? null : "流式响应未返回任何文本内容");
    result.latencyMs = Math.round(performance.now() - startedAt);
    result.ok = result.error === null;
    handlers.onText?.(state.text, state.chunks);
    return result;
  } catch (error) {
    result.latencyMs = Math.round(performance.now() - startedAt);
    result.error = error instanceof Error ? `读取响应失败：${error.message}` : "读取响应失败";
    return result;
  } finally {
    window.clearTimeout(timer);
    handlers.signal?.removeEventListener("abort", forwardAbort);
  }
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

export interface ModelListResult {
  models: string[];
  error: string | null;
}

/** 拉取模型列表：OpenAI 走 GET /models，Anthropic 走 GET /v1/models。 */
export async function fetchModelList(config: TargetConfig): Promise<ModelListResult> {
  const built = buildRequest(config);
  if (built.error) return { models: [], error: built.error };

  const url = built.url.replace(/\/(chat\/completions|messages)$/, "/models");
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), Math.max(1000, config.timeoutMs));

  try {
    const response = await fetch(url, { method: "GET", headers: built.headers, signal: controller.signal });
    const text = await response.text();
    if (!response.ok) {
      return { models: [], error: `HTTP ${response.status}：${text.slice(0, 200)}` };
    }

    const payload = safeParse(text);
    if (typeof payload !== "object" || payload === null) {
      return { models: [], error: "模型列表不是合法 JSON" };
    }

    const container = payload as Record<string, unknown>;
    const raw = Array.isArray(container.data) ? container.data : container.models;
    if (!Array.isArray(raw)) return { models: [], error: "未找到模型列表字段" };

    const models = raw
      .map((item) => {
        if (typeof item === "string") return item;
        if (typeof item === "object" && item !== null) {
          const record = item as Record<string, unknown>;
          return String(record.id ?? record.name ?? "");
        }
        return "";
      })
      .filter(Boolean);

    return { models, error: models.length ? null : "模型列表为空" };
  } catch {
    return { models: [], error: "拉取模型列表失败（网络或 CORS）" };
  } finally {
    window.clearTimeout(timer);
  }
}
