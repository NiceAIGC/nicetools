import { useNavigate, useParams } from "react-router-dom";
import { BreadcrumbItem, Breadcrumbs, Chip } from "@heroui/react";
import { getTool } from "../tools/registry";
import NotFound from "./NotFound";

export default function ToolPage() {
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const tool = id ? getTool(id) : undefined;

  if (!tool) return <NotFound />;

  const Component = tool.component;

  return (
    <div className="flex min-w-0 flex-col gap-6">
      <div className="flex flex-col gap-4">
        <Breadcrumbs size="sm" variant="light">
          <BreadcrumbItem onPress={() => navigate("/")}>
            全部工具
          </BreadcrumbItem>
          <BreadcrumbItem onPress={() => navigate(`/?category=${encodeURIComponent(tool.category)}`)}>
            {tool.category}
          </BreadcrumbItem>
          <BreadcrumbItem>{tool.name}</BreadcrumbItem>
        </Breadcrumbs>

        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 items-start gap-4">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-large bg-primary-50 text-3xl">
              <span aria-hidden>{tool.emoji}</span>
            </div>
            <div className="min-w-0">
              <h1 className="text-2xl font-bold text-foreground">{tool.name}</h1>
              <p className="mt-1 max-w-3xl text-sm leading-6 text-default-500">
                {tool.description}
              </p>
              <div className="mt-3 flex flex-wrap gap-1.5">
                {tool.tags.map((tag) => (
                  <Chip key={tag} size="sm" variant="bordered">
                    {tag}
                  </Chip>
                ))}
              </div>
            </div>
          </div>
          <Chip color="primary" variant="flat" className="w-fit shrink-0">
            {tool.category}
          </Chip>
        </div>
      </div>

      <Component />
    </div>
  );
}
