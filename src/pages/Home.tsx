import { useMemo, useState } from "react";
import { Input, Tabs, Tab } from "@heroui/react";
import { tools, categories, searchTools } from "../tools/registry";
import ToolCard from "../components/ToolCard";

export default function Home() {
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState<string>("全部");

  const list = useMemo(() => {
    const byQuery = searchTools(query);
    if (category === "全部") return byQuery;
    return byQuery.filter((t) => t.category === category);
  }, [query, category]);

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-bold text-foreground sm:text-3xl">
          NiceTools 在线工具集
        </h1>
        <p className="text-sm text-default-500">
          简洁好用的一站式小工具，无需登录，纯前端本地运行。
        </p>
      </header>

      <Input
        aria-label="搜索工具"
        placeholder="搜索工具名称、说明或分类…"
        value={query}
        onValueChange={setQuery}
        isClearable
        onClear={() => setQuery("")}
        variant="bordered"
        size="lg"
        startContent={<span className="text-default-400">🔍</span>}
        className="max-w-xl"
      />

      {categories.length > 1 && (
        <Tabs
          aria-label="工具分类"
          selectedKey={category}
          onSelectionChange={(key) => setCategory(String(key))}
          variant="light"
          color="primary"
        >
          <Tab key="全部" title="全部" />
          {categories.map((c) => (
            <Tab key={c} title={c} />
          ))}
        </Tabs>
      )}

      {list.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-20 text-center">
          <span className="text-4xl" aria-hidden>
            🗂️
          </span>
          <p className="text-default-500">没有找到匹配的工具</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {list.map((tool) => (
            <ToolCard key={tool.id} tool={tool} />
          ))}
        </div>
      )}

      <p className="text-xs text-default-400">
        共 {tools.length} 个工具，更多持续添加中。
      </p>
    </div>
  );
}
