import { useNavigate } from "react-router-dom";
import { Card, CardBody, Chip } from "@heroui/react";
import type { ToolMeta } from "../tools/registry";

export default function ToolCard({ tool }: { tool: ToolMeta }) {
  const navigate = useNavigate();

  return (
    <Card
      isPressable
      isHoverable
      shadow="sm"
      onPress={() => navigate(`/tools/${tool.id}`)}
      className="h-full border border-default-200"
    >
      <CardBody className="gap-3 p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-default-100 text-2xl">
            <span aria-hidden>{tool.emoji}</span>
          </div>
          <Chip size="sm" variant="flat" className="shrink-0">
            {tool.category}
          </Chip>
        </div>
        <div className="min-w-0">
          <h3 className="truncate text-base font-semibold text-foreground">
            {tool.name}
          </h3>
          <p className="mt-1 line-clamp-2 text-sm text-default-500">
            {tool.description}
          </p>
        </div>
      </CardBody>
    </Card>
  );
}
