import { useNavigate } from "react-router-dom";
import { Card, CardBody, CardFooter, CardHeader, Chip } from "@heroui/react";
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
      <CardHeader className="items-start justify-between gap-3 pb-2">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-default-100 text-2xl">
            <span aria-hidden>{tool.emoji}</span>
          </div>
          <h3 className="min-w-0 text-left text-base font-semibold text-foreground">
            {tool.name}
          </h3>
        </div>
        <Chip size="sm" variant="flat" color="primary" className="shrink-0">
          {tool.category}
        </Chip>
      </CardHeader>
      <CardBody className="gap-3 py-2">
        <p className="text-left text-sm leading-6 text-default-500">
          {tool.description}
        </p>
      </CardBody>
      <CardFooter className="flex items-end justify-between gap-3 pt-2">
        <div className="flex min-w-0 flex-wrap gap-1.5">
          {tool.tags.map((tag) => (
            <Chip key={tag} size="sm" variant="light">
              {tag}
            </Chip>
          ))}
        </div>
        <span className="shrink-0 text-sm font-medium text-primary" aria-hidden>
          打开 →
        </span>
      </CardFooter>
    </Card>
  );
}
