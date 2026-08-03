import { useMemo, useState } from "react";
import { Card, CardBody, Chip, Tab, Tabs } from "@heroui/react";
import { useSearch } from "../search";
import { tools, categories, searchTools } from "../tools/registry";
import ToolCard from "../components/ToolCard";

const allCategoriesKey = "全部工具";

export default function Home() {
  const { query } = useSearch();
  const [category, setCategory] = useState(allCategoriesKey);

  const list = useMemo(() => {
    const byQuery = searchTools(query);
    if (category === allCategoriesKey) return byQuery;
    return byQuery.filter((tool) => tool.category === category);
  }, [query, category]);

  const categoryItems = [allCategoriesKey, ...categories];

  return (
    <div className="flex min-w-0 flex-col gap-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-bold text-foreground sm:text-3xl">
            NiceTools 在线工具集
          </h1>
          <p className="max-w-2xl text-sm text-default-500">
            简洁好用的一站式小工具，所有数据都在浏览器内处理。
          </p>
        </div>
        <Chip variant="flat" color="primary">
          {tools.length} 个可用工具
        </Chip>
      </header>

      <Tabs
        aria-label="工具分类"
        selectedKey={category}
        onSelectionChange={(key) => setCategory(String(key))}
        variant="underlined"
        color="primary"
        classNames={{ tabList: "gap-4", cursor: "w-full" }}
      >
        {categoryItems.map((item) => {
          const count = item === allCategoriesKey
            ? tools.length
            : tools.filter((tool) => tool.category === item).length;
          return (
            <Tab
              key={item}
              title={
                <div className="flex items-center gap-2">
                  <span>{item}</span>
                  <Chip size="sm" variant="flat">
                    {count}
                  </Chip>
                </div>
              }
            />
          );
        })}
      </Tabs>

      <div className="flex min-w-0 flex-col gap-5">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h2 className="text-lg font-semibold text-foreground">{category}</h2>
            <p className="text-sm text-default-500">
              {list.length} 个匹配工具
            </p>
          </div>
        </div>

        {list.length === 0 ? (
          <Card shadow="sm" className="border border-default-200">
            <CardBody className="flex flex-col items-center gap-3 py-20 text-center">
              <span className="text-4xl" aria-hidden>
                🗂️
              </span>
              <div>
                <p className="font-semibold text-foreground">没有找到匹配的工具</p>
                <p className="mt-1 text-sm text-default-500">
                  可以尝试更换搜索词或切换到其他分类。
                </p>
              </div>
            </CardBody>
          </Card>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {list.map((tool) => (
              <ToolCard key={tool.id} tool={tool} />
            ))}
          </div>
        )}

        <p className="text-center text-xs text-default-400">
          更多实用工具持续添加中。
        </p>
      </div>
    </div>
  );
}
