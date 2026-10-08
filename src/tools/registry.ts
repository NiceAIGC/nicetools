import type { ComponentType, LazyExoticComponent } from "react";
import { lazy } from "react";
import catalog from "./catalog.json";

// 工具元数据与后端共享；组件按目录约定懒加载。

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

const loaders = import.meta.glob<{ default: ComponentType }>(
  "../tools-impl/*/index.tsx",
);

export const tools: ToolMeta[] = catalog.map((meta) => {
  const load = loaders[`../tools-impl/${meta.id}/index.tsx`];
  if (!load) throw new Error(`工具 ${meta.id} 缺少实现组件`);
  return { ...meta, component: lazy(load) };
});

export function getTool(id: string): ToolMeta | undefined {
  return tools.find((t) => t.id === id);
}

// 简单的关键词过滤：命中名称、说明、分类或标签任一即可。
export function searchTools(query: string, list: ToolMeta[] = tools): ToolMeta[] {
  const q = query.trim().toLowerCase();
  if (!q) return list;
  return list.filter((t) => {
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
