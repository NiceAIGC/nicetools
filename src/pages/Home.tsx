import { useMemo, useState } from "react";
import { Button, Chip } from "@heroui/react";
import { useSearch } from "../search";
import { tools, groups, searchTools } from "../tools/registry";
import ToolCard from "../components/ToolCard";

const allGroupsKey = "全部工具";

export default function Home() {
  const { query } = useSearch();
  const [group, setGroup] = useState<string>(groups[0] ?? allGroupsKey);

  const list = useMemo(() => {
    const byQuery = searchTools(query);
    if (group === allGroupsKey) return byQuery;
    return byQuery.filter((t) => t.group === group);
  }, [query, group]);

  const groupItems = groups.length > 1 ? [allGroupsKey, ...groups] : groups;

  return (
    <div className="flex min-w-0 flex-col gap-6">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-bold text-foreground sm:text-3xl">
          NiceTools 在线工具集
        </h1>
        <p className="text-sm text-default-500">
          简洁好用的一站式小工具，安全快捷，注重隐私保护。
        </p>
      </header>

      <div className="grid min-w-0 grid-cols-1 gap-6 lg:grid-cols-[220px_1fr]">
        <aside className="min-w-0">
          <div className="flex gap-2 overflow-x-auto pb-1 lg:sticky lg:top-20 lg:flex-col lg:overflow-visible lg:pb-0">
            {groupItems.map((item) => {
              const count =
                item === allGroupsKey
                  ? tools.length
                  : tools.filter((tool) => tool.group === item).length;
              const selected = group === item;
              return (
                <Button
                  key={item}
                  fullWidth
                  variant={selected ? "solid" : "light"}
                  color={selected ? "primary" : "default"}
                  className="h-10 min-w-fit justify-between px-3 lg:min-w-0"
                  onPress={() => setGroup(item)}
                  endContent={
                    <Chip
                      size="sm"
                      variant={selected ? "flat" : "light"}
                      className="shrink-0"
                    >
                      {count}
                    </Chip>
                  }
                >
                  <span className="truncate">{item}</span>
                </Button>
              );
            })}
          </div>
        </aside>

        <div className="flex min-w-0 flex-col gap-5">
          <div className="min-w-0">
            <div className="min-w-0">
              <h2 className="text-lg font-semibold text-foreground">{group}</h2>
              <p className="text-sm text-default-500">
                当前分组 {list.length} 个工具
              </p>
            </div>
          </div>

          {list.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-20 text-center">
              <span className="text-4xl" aria-hidden>
                🗂️
              </span>
              <p className="text-default-500">没有找到匹配的工具</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {list.map((tool) => (
                <ToolCard key={tool.id} tool={tool} />
              ))}
            </div>
          )}

          <p className="text-xs text-default-400">
            共 {tools.length} 个工具，更多持续添加中。
          </p>
        </div>
      </div>
    </div>
  );
}
