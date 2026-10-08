import { useEffect, useState } from "react";
import { BreadcrumbItem, Breadcrumbs, Button, Chip, Spinner } from "@heroui/react";
import { useNavigate, useParams } from "react-router-dom";
import { getTool } from "../tools/registry";
import { api, ApiError, type ToolAccess } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import NotFound from "./NotFound";

type Gate = { id: string; identity: string; status: "ok" | "hidden" | "denied" | "error" };
export default function ToolPage() {
  const { id = "" } = useParams<{ id: string }>();
  const tool = getTool(id);
  const navigate = useNavigate();
  const { session, tools, loading, error, refresh } = useAuth();
  const identity = session?.user ? String(session.user.id) : "guest";
  const [gate, setGate] = useState<Gate | null>(null);
  const [retry, setRetry] = useState(0);
  const permitted = tools?.find(item => item.id === id);
  const canUse = !!permitted?.can_use;
  useEffect(() => {
    if (!tool || loading || error || !canUse) return;
    let live = true;
    setGate(null);
    api<ToolAccess>(`/tools/${encodeURIComponent(id)}/access`).then(() => {
      if (live) setGate({ id, identity, status: "ok" });
    }).catch(e => {
      if (live) setGate({ id, identity, status: e instanceof ApiError && e.status === 404 ? "hidden" : e instanceof ApiError && e.status === 403 ? "denied" : "error" });
    });
    return () => { live = false; };
  }, [tool, id, identity, loading, error, canUse, retry]);
  if (!tool) return <NotFound />;
  if (loading) return <Spinner label="验证工具权限…" />;
  if (error || !tools) return <div className="space-y-3"><p role="alert" className="text-danger">{error || "无法获取权限"}</p><Button onPress={() => void refresh()}>重新连接</Button></div>;
  const status = gate?.id === id && gate.identity === identity ? gate.status : null;
  if (!permitted || status === "hidden") return <NotFound />;
  if (!canUse || status === "denied") return <div className="space-y-4 rounded-large border border-default-200 bg-background p-6">
    <h1 className="text-xl font-semibold">{tool.name}</h1><p role="alert" className="text-default-500">此工具可见，但当前没有使用权限或已停用。请联系管理员授权。</p>
    <div className="flex gap-3">{!session?.user && <Button color="primary" onPress={() => navigate(`/login?next=${encodeURIComponent(`/tools/${id}`)}`)}>登录账号</Button>}<Button variant="bordered" onPress={() => navigate("/")}>返回工具</Button></div>
  </div>;
  if (status === "error") return <div className="space-y-3"><p role="alert">无法验证工具权限。</p><Button onPress={() => { void refresh(); setRetry(value => value + 1); }}>重试</Button></div>;
  if (status !== "ok") return <Spinner label="验证工具权限…" />;
  const Component = tool.component;
  return <div className="flex min-w-0 flex-col gap-6">
    <Breadcrumbs size="sm"><BreadcrumbItem onPress={() => navigate("/")}>全部工具</BreadcrumbItem><BreadcrumbItem onPress={() => navigate(`/?category=${encodeURIComponent(tool.category)}`)}>{tool.category}</BreadcrumbItem><BreadcrumbItem>{tool.name}</BreadcrumbItem></Breadcrumbs>
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div className="flex min-w-0 items-start gap-4"><div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-large bg-primary-50 text-3xl">{tool.emoji}</div><div className="min-w-0"><h1 className="text-2xl font-bold">{tool.name}</h1><p className="mt-1 text-sm leading-6 text-default-500">{tool.description}</p><div className="mt-3 flex flex-wrap gap-1.5">{tool.tags.map(tag => <Chip key={tag} size="sm" variant="bordered">{tag}</Chip>)}</div></div></div><Chip color="primary" variant="flat">{tool.category}</Chip></div>
    <Component />
  </div>;
}
