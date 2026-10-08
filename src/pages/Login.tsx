import { useState, type FormEvent } from "react";
import { Button, Card, CardBody, Input } from "@heroui/react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { api, json } from "../api/client";
import { useAuth } from "../auth/AuthContext";

export default function Login() {
  const { session, loading, error: connectionError, refresh } = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const requested = params.get("next") ?? "/";
  const next = requested.startsWith("/") && !requested.startsWith("//") && requested !== "/login" ? requested : "/";
  if (session?.user) return <Navigate replace to={next} />;
  async function submit(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try { await api("/auth/login", json("POST", { username, password })); await refresh(); navigate(next, { replace: true }); }
    catch (e) { setError(e instanceof Error ? e.message : "登录失败"); }
    finally { setBusy(false); }
  }
  return <Card className="mx-auto max-w-md" shadow="sm"><CardBody className="gap-5 p-6">
    <div><h1 className="text-2xl font-bold">账号登录</h1><p className="mt-2 text-sm text-default-500">不开放注册。需要账号或工具权限，请联系管理员。</p></div>
    <form onSubmit={submit} className="flex flex-col gap-4">
      <Input label="用户名" autoComplete="username" value={username} onValueChange={setUsername} isRequired maxLength={32} />
      <Input label="密码" type="password" autoComplete="current-password" value={password} onValueChange={setPassword} isRequired />
      {(error || connectionError) && <p role="alert" className="text-sm text-danger">{error || connectionError}</p>}
      <Button type="submit" color="primary" isLoading={busy || loading} isDisabled={!!connectionError}>登录</Button>
      {connectionError && <Button variant="bordered" onPress={() => void refresh()}>重新连接</Button>}
    </form>
    <Link to="/" className="text-sm text-primary">作为游客浏览工具</Link>
  </CardBody></Card>;
}
