export interface ProbeConfig {
  apiKey: string;
  baseUrl: string;
  model: string;
  rounds: number;
  waitSeconds: number;
  concurrency: number;
  timeoutSeconds: number;
}

export interface RequestResult {
  ok: boolean;
  read: number;
  write: number;
  error?: string;
}

export interface RoundResult {
  index: number;
  warmup: RequestResult;
  results: RequestResult[];
}

export function buildUrl(baseUrl: string): string {
  const base = baseUrl.trim().replace(/\/+$/, "");
  if (base.endsWith("/v1/messages")) return base;
  if (base.endsWith("/v1")) return `${base}/messages`;
  return `${base}/v1/messages`;
}

export function buildSystem(tag: string) {
  const paragraph =
    "固定政策段落 {n}：审批、风控、法务、客服、运营、研发和财务都使用同一套企业知识库口径；处理任何请求时，先确认固定规则，再基于本轮短问题给出简洁回答。\n";
  const body =
    `[prompt-cache-bench tag=${tag}] 企业知识库稳定前缀，所有请求保持完全一致；cache_control 放在该 system block 末尾。\n` +
    Array.from({ length: 200 }, (_, index) =>
      paragraph.replace("{n}", String(index + 1).padStart(3, "0")),
    ).join("");

  return [{ type: "text", text: body, cache_control: { type: "ephemeral" } }];
}

export async function sendRequest(
  url: string,
  apiKey: string,
  model: string,
  system: ReturnType<typeof buildSystem>,
  question: string,
  timeoutSeconds: number,
): Promise<RequestResult> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), timeoutSeconds * 1000);

  try {
    const response = await fetch(url, {
      method: "POST",
      signal: controller.signal,
      headers: {
        "Content-Type": "application/json",
        "x-api-key": apiKey,
        Authorization: `Bearer ${apiKey}`,
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify({
        model,
        system,
        max_tokens: 16,
        stream: false,
        messages: [{ role: "user", content: question }],
      }),
    });

    const body = await response.text();
    if (!response.ok) {
      return { ok: false, read: 0, write: 0, error: `HTTP ${response.status}: ${body.slice(0, 120)}` };
    }

    const usage = (JSON.parse(body) as { usage?: Record<string, unknown> }).usage ?? {};
    return {
      ok: true,
      read: Number(usage.cache_read_input_tokens) || 0,
      write: Number(usage.cache_creation_input_tokens) || 0,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return {
      ok: false,
      read: 0,
      write: 0,
      error: error instanceof DOMException && error.name === "AbortError" ? "请求超时" : message,
    };
  } finally {
    window.clearTimeout(timeout);
  }
}

export function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}
