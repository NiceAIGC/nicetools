import type { ComponentType, LazyExoticComponent } from "react";
import { lazy } from "react";

// 工具注册表 —— 新增工具只需在下方 `tools` 数组追加一条记录。
// 首页搜索/卡片与路由都会自动读取本表，无需改动其它文件。

export interface ToolMeta {
  /** 唯一 id，同时作为路由 slug：/tools/:id */
  id: string;
  /** 展示名称 */
  name: string;
  /** 一句话说明 */
  description: string;
  /** emoji 图标 */
  emoji: string;
  /** 分类 */
  category: string;
  /** 标签，用于补充工具特征并参与搜索 */
  tags: string[];
  /** 懒加载的工具组件 */
  component: LazyExoticComponent<ComponentType>;
}

export const tools: ToolMeta[] = [
  {
    id: "alarm-clock",
    name: "在线闹钟",
    description: "在浏览器中创建多个闹钟，支持重复提醒、贪睡和声音通知。",
    emoji: "⏰",
    category: "生活工具",
    tags: ["闹钟", "提醒", "贪睡", "本地存储"],
    component: lazy(() => import("../tools-impl/alarm-clock")),
  },
  {
    id: "llm-cost",
    name: "大模型费用计算器",
    description:
      "按 TPM、时长、输入输出比例、缓存命中率和支付折扣估算 token 成本。",
    emoji: "💰",
    category: "AI 工具",
    tags: ["LLM", "Token", "TPM", "费用估算"],
    component: lazy(() => import("../tools-impl/llm-cost")),
  },
  {
    id: "prompt-cache-probe",
    name: "Prompt 缓存探测器",
    description: "预热固定前缀后发起并发请求，检测上游 API 的缓存命中情况。",
    emoji: "🧪",
    category: "AI 工具",
    tags: ["Prompt Cache", "并发测试"],
    component: lazy(() => import("../tools-impl/prompt-cache-probe")),
  },
  {
    id: "json-value-extractor",
    name: "JSON 值提取工具",
    description: "逐行解析 JSON 对象，提取指定顶层键的值并汇总输出。",
    emoji: "🔎",
    category: "文本工具",
    tags: ["JSON", "字段提取", "批量处理"],
    component: lazy(() => import("../tools-impl/json-value-extractor")),
  },
  {
    id: "text-delimiter",
    name: "多行文本分隔工具",
    description: "按自定义分隔符拆分多行文本，并按索引顺序重排、拼接。",
    emoji: "✂️",
    category: "文本工具",
    tags: ["文本分隔", "列重排", "批量处理"],
    component: lazy(() => import("../tools-impl/text-delimiter")),
  },
];

export const categories: string[] = Array.from(
  new Set(tools.map((t) => t.category)),
);

export function getTool(id: string): ToolMeta | undefined {
  return tools.find((t) => t.id === id);
}

// 简单的关键词过滤：命中名称、说明、分类或标签任一即可。
export function searchTools(query: string): ToolMeta[] {
  const q = query.trim().toLowerCase();
  if (!q) return tools;
  return tools.filter((t) => {
    const haystack = [
      t.name,
      t.description,
      t.category,
      ...t.tags,
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(q);
  });
}
