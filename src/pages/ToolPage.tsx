import { useEffect, useState } from "react";
import { BreadcrumbItem, Breadcrumbs, Chip, Spinner } from "@heroui/react";
import { useNavigate, useParams } from "react-router-dom";
import { getComponent, type ToolMeta } from "../tools/registry";
import { api, ApiError, type ToolAccess } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import NotFound from "./NotFound";

type Gate = { id: string; identity: string; status: "ok" | "hidden" | "denied" | "error" };
export default function ToolPage() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { session, tools, loading, error } = useAuth();
  const identity = session?.user ? String(session.user.id) : "guest";
  const [gate, setGate] = useState<Gate | null>(null);
  const permitted = tools?.find(item => item.id === id);
  const component = id ? getComponent(id) : undefined;
  const tool: ToolMeta | undefined =
    permitted && component
      ? {
          id: permitted.id,
          name: permitted.name,
          description: permitted.description,
          emoji: permitted.emoji,
          category: permitted.category,
          tags: permitted.tags,
          component,
        }
      : undefined;
  const canUse = !!permitted?.can_use;
  // tool 每次渲染都是新对象；副作用只依赖稳定的原始值，否则 /access 会被无限重放。
  const hasTool = !!tool;
  useEffect(() => {
    if (!hasTool || loading || error || !canUse) return;
    let live = true;
    setGate(null);
    api<ToolAccess>(`/tools/${encodeURIComponent(id)}/access`).then(() => {
      if (live) setGate({ id, identity, status: "ok" });
    }).catch(e => {
      if (live) setGate({ id, identity, status: e instanceof ApiError && e.status === 404 ? "hidden" : e instanceof ApiError && e.status === 403 ? "denied" : "error" });
    });
    return () => { live = false; };
  }, [hasTool, id, identity, loading, error, canUse]);

  if (!component) return <NotFound />;
  const status = gate?.id === id && gate.identity === identity ? gate.status : null;
  if (error || status === "error") return <p role="alert" className="py-20 text-center text-sm text-default-500">加载失败，请稍后重试</p>;
  // 加载与校验中只转圈，不展示任何权限相关界面。
  if (loading || !tools || (canUse && status === null)) return <div className="flex justify-center py-24"><Spinner /></div>;
  if (!tool || !canUse || status === "denied" || status === "hidden") {
    return <p role="alert" className="py-20 text-center text-sm text-default-500">暂无该工具的使用权限</p>;
  }

  const Component = tool.component;
  return <div className="flex min-w-0 flex-col gap-6">
    <Breadcrumbs size="sm"><BreadcrumbItem onPress={() => navigate("/")}>全部工具</BreadcrumbItem><BreadcrumbItem onPress={() => navigate(`/?category=${encodeURIComponent(tool.category)}`)}>{tool.category}</BreadcrumbItem><BreadcrumbItem>{tool.name}</BreadcrumbItem></Breadcrumbs>
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div className="flex min-w-0 items-start gap-4"><div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-large bg-primary-50 text-3xl">{tool.emoji}</div><div className="min-w-0"><h1 className="text-2xl font-bold">{tool.name}</h1><p className="mt-1 text-sm leading-6 text-default-500">{tool.description}</p><div className="mt-3 flex flex-wrap gap-1.5">{tool.tags.map(tag => <Chip key={tag} size="sm" variant="bordered">{tag}</Chip>)}</div></div></div><Chip color="primary" variant="flat">{tool.category}</Chip></div>
    <Component />
  </div>;
}
