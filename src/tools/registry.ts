import type { ComponentType, LazyExoticComponent } from "react";
import { lazy } from "react";
import catalog from "./catalog.json";

// 展示信息（名称/Logo/说明/分类/标签/顺序）由后台维护，来自 /api/tools；
// 这里只负责「工具 id → 懒加载组件」的代码映射与搜索辅助。
// catalog.json 同时是后端播种新工具的默认值与构建期资源保护清单。

export interface ToolMeta {
  /** 唯一 id，同时作为路由 slug：/tools/:id */
  id: string;
  /** 展示名称 */
  name: string;
  /** 一句话说明 */
  description: string;
  /** Logo/图标字符 */
  emoji: string;
  /** 分类 */
  category: string;
  /** 标签 */
  tags: string[];
  /** 是否带后端模块（catalog.json 中 backend: true）；启动时后端校验模块与目录对齐 */
  backend?: boolean;
  /** 懒加载的工具组件 */
  component: LazyExoticComponent<ComponentType>;
}

const loaders = import.meta.glob<{ default: ComponentType }>(
  "../tools-impl/*/index.tsx",
);

const components = new Map<string, LazyExoticComponent<ComponentType>>();
for (const meta of catalog) {
  const load = loaders[`../tools-impl/${meta.id}/index.tsx`];
  if (!load) throw new Error(`工具 ${meta.id} 缺少实现组件`);
  components.set(meta.id, lazy(load));
}

/** 取工具组件；后台存在但代码缺实现时返回 undefined。 */
export function getComponent(
  id: string,
): LazyExoticComponent<ComponentType> | undefined {
  return components.get(id);
}

// 简单的关键词过滤：命中名称、说明、分类或标签任一即可。
export function searchTools(query: string, list: ToolMeta[]): ToolMeta[] {
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
