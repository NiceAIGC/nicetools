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
  /** 首页左侧导航分组 */
  group: string;
  /** 搜索关键词（可选，额外命中词） */
  keywords?: string[];
  /** 懒加载的工具组件 */
  component: LazyExoticComponent<ComponentType>;
}

export const tools: ToolMeta[] = [
  {
    id: "llm-cost",
    name: "大模型费用计算器",
    description:
      "按 TPM、时长、输入输出比例、缓存命中率和支付折扣估算 token 成本。",
    emoji: "💰",
    category: "AI 工具",
    group: "默认分组",
    keywords: ["llm", "token", "费用", "成本", "计算器", "tpm", "cost"],
    component: lazy(() => import("../tools-impl/llm-cost")),
  },
];

export const groups: string[] = Array.from(new Set(tools.map((t) => t.group)));

export const categories: string[] = Array.from(
  new Set(tools.map((t) => t.category)),
);

export function getTool(id: string): ToolMeta | undefined {
  return tools.find((t) => t.id === id);
}

// 简单的关键词过滤：命中名称/说明/分类/关键词任一即可。
export function searchTools(query: string): ToolMeta[] {
  const q = query.trim().toLowerCase();
  if (!q) return tools;
  return tools.filter((t) => {
    const haystack = [
      t.name,
      t.description,
      t.category,
      t.group,
      ...(t.keywords || []),
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(q);
  });
}
