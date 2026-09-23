// 服务商参考配置、提示词参考与本地记录（localStorage）。
import type { ApiFormat, TargetConfig } from "./request";

export interface ProviderPreset {
  key: string;
  label: string;
  format: ApiFormat;
  baseUrl: string;
  model: string;
  extraHeaders: string;
  note: string;
}

export const providerPresets: ProviderPreset[] = [
  {
    key: "custom",
    label: "自定义 / 中转网关",
    format: "openai",
    baseUrl: "",
    model: "",
    extraHeaders: "",
    note: "手动填写 Base URL；多数中转服务使用 OpenAI 兼容格式，Base URL 一般以 /v1 结尾。",
  },
  {
    key: "openai",
    label: "OpenAI",
    format: "openai",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    extraHeaders: "",
    note: "官方接口，Base URL 已含 /v1，工具会补全 /chat/completions。",
  },
  {
    key: "anthropic",
    label: "Anthropic Claude（原生格式）",
    format: "anthropic",
    baseUrl: "https://api.anthropic.com/v1",
    model: "claude-3-5-haiku-latest",
    extraHeaders: "",
    note: "浏览器直连官方接口需要 anthropic-dangerous-direct-browser-access 头，工具已自动附带。",
  },
  {
    key: "deepseek",
    label: "DeepSeek",
    format: "openai",
    baseUrl: "https://api.deepseek.com/v1",
    model: "deepseek-chat",
    extraHeaders: "",
    note: "OpenAI 兼容格式；深度思考模型返回 reasoning_content，可在原始响应中查看。",
  },
  {
    key: "moonshot",
    label: "月之暗面 Kimi",
    format: "openai",
    baseUrl: "https://api.moonshot.cn/v1",
    model: "moonshot-v1-8k",
    extraHeaders: "",
    note: "OpenAI 兼容格式。",
  },
  {
    key: "zhipu",
    label: "智谱 GLM",
    format: "openai",
    baseUrl: "https://open.bigmodel.cn/api/paas/v4",
    model: "glm-4-flash",
    extraHeaders: "",
    note: "Base URL 自带 /api/paas/v4 路径，工具只补全 /chat/completions。",
  },
  {
    key: "dashscope",
    label: "阿里云百炼（通义千问）",
    format: "openai",
    baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    model: "qwen-plus",
    extraHeaders: "",
    note: "使用百炼的 OpenAI 兼容模式路径。",
  },
  {
    key: "siliconflow",
    label: "硅基流动 SiliconFlow",
    format: "openai",
    baseUrl: "https://api.siliconflow.cn/v1",
    model: "Qwen/Qwen2.5-7B-Instruct",
    extraHeaders: "",
    note: "OpenAI 兼容格式，模型名带命名空间（如 Qwen/…）。",
  },
  {
    key: "openrouter",
    label: "OpenRouter",
    format: "openai",
    baseUrl: "https://openrouter.ai/api/v1",
    model: "openai/gpt-4o-mini",
    extraHeaders: '{\n  "HTTP-Referer": "https://tools.nicecode.ai",\n  "X-Title": "NiceTools"\n}',
    note: "建议附带 HTTP-Referer / X-Title 头（已预填），否则可能被限流。",
  },
  {
    key: "gemini",
    label: "Google Gemini（OpenAI 兼容）",
    format: "openai",
    baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai",
    model: "gemini-2.0-flash",
    extraHeaders: "",
    note: "使用 Gemini 的 OpenAI 兼容端点，Key 直接作为 Bearer。",
  },
  {
    key: "ollama",
    label: "Ollama（本地）",
    format: "openai",
    baseUrl: "http://localhost:11434/v1",
    model: "llama3.2",
    extraHeaders: "",
    note: "本地服务无需 API Key；注意 https 页面调用 http 会被浏览器拦截，建议用 curl 验证。",
  },
  {
    key: "vllm",
    label: "vLLM / LM Studio（本地）",
    format: "openai",
    baseUrl: "http://localhost:8000/v1",
    model: "",
    extraHeaders: "",
    note: "本地 OpenAI 兼容服务，模型名以服务端加载的为准。",
  },
];

export const promptPresets: { label: string; text: string }[] = [
  { label: "仅回复 OK", text: "只回复 OK" },
  { label: "1+1=?", text: "1+1=? 只回答数字" },
  { label: "一句话自述", text: "用一句话说明你是哪个模型" },
  { label: "较长输出（测流式）", text: "用 5 句话介绍你自己，每句一行" },
];

export interface SavedProfile {
  id: string;
  name: string;
  format: ApiFormat;
  baseUrl: string;
  apiKey: string;
  model: string;
  savedAt: number;
}

const profilesKey = "nicetools.llm-connectivity.profiles";
const draftKey = "nicetools.llm-connectivity.draft";

export const defaultConfig: TargetConfig = {
  format: "openai",
  baseUrl: "https://api.openai.com/v1",
  apiKey: "",
  model: "gpt-4o-mini",
  prompt: "只回复 OK",
  maxTokens: null,
  stream: false,
  timeoutMs: 30000,
  extraHeaders: "",
  bearerForAnthropic: false,
};

export function loadProfiles(): SavedProfile[] {
  try {
    const raw = localStorage.getItem(profilesKey);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (item): item is SavedProfile =>
        typeof item === "object" && item !== null && typeof item.id === "string" && typeof item.baseUrl === "string",
    );
  } catch {
    return [];
  }
}

export function storeProfiles(profiles: SavedProfile[]): void {
  try {
    localStorage.setItem(profilesKey, JSON.stringify(profiles));
  } catch {
    // 存储不可用（隐私模式 / 配额）时忽略
  }
}

export function loadDraft(): TargetConfig {
  try {
    const raw = localStorage.getItem(draftKey);
    if (!raw) return defaultConfig;
    const parsed = JSON.parse(raw) as Partial<TargetConfig>;
    return {
      ...defaultConfig,
      ...parsed,
      maxTokens: typeof parsed.maxTokens === "number" ? parsed.maxTokens : null,
    };
  } catch {
    return defaultConfig;
  }
}

export function storeDraft(config: TargetConfig): void {
  try {
    localStorage.setItem(draftKey, JSON.stringify(config));
  } catch {
    // 忽略
  }
}

export function clearLocalRecords(): void {
  try {
    localStorage.removeItem(profilesKey);
    localStorage.removeItem(draftKey);
  } catch {
    // 忽略
  }
}

export function profileName(config: TargetConfig): string {
  const base = config.baseUrl.replace(/^[a-zA-Z]+:\/\//, "").replace(/\/+$/, "");
  const parts = [config.format === "anthropic" ? "Claude" : "OpenAI", config.model.trim() || base]
    .filter(Boolean)
    .join(" · ");
  return parts || "未命名配置";
}
