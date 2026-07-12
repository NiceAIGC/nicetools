import { useParams, Link } from "react-router-dom";
import { Button } from "@heroui/react";
import { getTool } from "../tools/registry";
import NotFound from "./NotFound";

export default function ToolPage() {
  const { id } = useParams<{ id: string }>();
  const tool = id ? getTool(id) : undefined;

  if (!tool) return <NotFound />;

  const Component = tool.component;

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <div className="flex flex-col gap-3">
        <Button
          as={Link}
          to="/"
          variant="light"
          size="sm"
          className="w-fit px-2 text-default-500"
          startContent={<span aria-hidden>←</span>}
        >
          返回工具集
        </Button>
        <div className="flex min-w-0 items-start gap-3">
          <span className="shrink-0 text-3xl" aria-hidden>
            {tool.emoji}
          </span>
          <div className="min-w-0">
            <h1 className="text-xl font-bold text-foreground sm:text-2xl">
              {tool.name}
            </h1>
            <p className="mt-0.5 max-w-full break-all text-sm text-default-500 sm:break-words">
              {tool.description}
            </p>
          </div>
        </div>
      </div>

      <Component />
    </div>
  );
}
