import { useMemo } from "react";
import { Button, Card, CardBody, Chip, Spinner, Tab, Tabs } from "@heroui/react";
import { useSearchParams } from "react-router-dom";
import { useSearch } from "../search";
import { searchTools, tools } from "../tools/registry";
import ToolCard from "../components/ToolCard";
import { useAuth } from "../auth/AuthContext";
import type { ToolAccess } from "../api/client";

const all = "全部工具";
export default function Home() {
  const { query } = useSearch();
  const { tools: access, loading, error, refresh } = useAuth();
  const [params, setParams] = useSearchParams();
  const permitted = useMemo(() => {
    const byID: Record<string, ToolAccess> = {};
    for (const item of access ?? []) byID[item.id] = item;
    return tools.filter(tool => byID[tool.id]).map(tool => ({ tool, access: byID[tool.id] }));
  }, [access]);
  const categories = [...new Set(permitted.map(item => item.tool.category))];
  const requested = params.get("category") ?? all;
  const category = requested === all || categories.includes(requested) ? requested : all;
  const list = useMemo(() => {
    const found = searchTools(query, permitted.map(item => item.tool));
    return permitted.filter(item => found.includes(item.tool) && (category === all || item.tool.category === category));
  }, [query, permitted, category]);
  if (loading) return <Spinner label="加载工具权限…" />;
  if (error || !access) return <div className="space-y-3"><p role="alert" className="text-danger">{error || "无法获取工具权限"}</p><Button onPress={() => void refresh()}>重试</Button></div>;
  return <div className="flex min-w-0 flex-col gap-6">
    <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div><h1 className="text-2xl font-bold sm:text-3xl">NiceTools 在线工具集</h1><p className="mt-2 text-sm text-default-500">工具在浏览器内处理数据；带权限标记的工具需管理员授权。</p></div>
      <Chip color="primary" variant="flat">{permitted.length} 个可见工具</Chip>
    </header>
    <Tabs aria-label="工具分类" selectedKey={category} variant="underlined" color="primary" onSelectionChange={key => setParams(String(key) === all ? {} : { category: String(key) })}>
      {[all, ...categories].map(item => <Tab key={item} title={<div className="flex gap-2"><span>{item}</span><Chip size="sm" variant="flat">{item === all ? permitted.length : permitted.filter(t => t.tool.category === item).length}</Chip></div>} />)}
    </Tabs>
    <div><h2 className="text-lg font-semibold">{category}</h2><p className="text-sm text-default-500">{list.length} 个匹配工具</p></div>
    {list.length ? <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">{list.map(({ tool, access: item }) => <ToolCard key={tool.id} tool={tool} canUse={item.can_use} />)}</div> : <Card><CardBody className="py-16 text-center"><h3 className="font-semibold">没有可显示的工具</h3><p className="mt-2 text-sm text-default-500">尝试其他搜索词，或联系管理员获取权限。</p></CardBody></Card>}
  </div>;
}
