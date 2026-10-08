import { useState, type FormEvent } from "react";
import { Button, Card, CardBody, Chip, Input } from "@heroui/react";
import { useNavigate } from "react-router-dom";
import { api, json } from "../api/client";
import { useAuth } from "../auth/AuthContext";

export default function Account() {
  const { session, refresh } = useAuth();
  const navigate = useNavigate();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const user = session?.user;
  async function submit(event: FormEvent) {
    event.preventDefault(); setError("");
    if (password !== confirm) { setError("两次新密码不一致"); return; }
    if (new TextEncoder().encode(password).length > 72) { setError("新密码最多 72 字节"); return; }
    setBusy(true);
    try { await api("/account/password", json("PUT", { current_password: current, password })); await refresh(); navigate("/login", { replace: true }); }
    catch (e) { setError(e instanceof Error ? e.message : "修改失败"); }
    finally { setBusy(false); }
  }
  return <div className="flex max-w-xl flex-col gap-5">
    <h1 className="text-2xl font-bold">我的账号</h1>
    <Card><CardBody className="gap-2"><p className="font-semibold">{user?.display_name || user?.username}</p><p className="text-sm text-default-500">用户名：{user?.username} · {user?.role === "admin" ? "管理员" : "普通用户"}</p><div className="flex flex-wrap gap-2">{user?.groups.map(group => <Chip key={group.id} size="sm">{group.name}</Chip>)}</div></CardBody></Card>
    <Card><CardBody className="gap-4 p-6"><h2 className="text-lg font-semibold">修改密码</h2><p className="text-sm text-default-500">修改后所有设备退出登录。新密码须为 12–72 字节。</p>
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Input label="当前密码" type="password" autoComplete="current-password" value={current} onValueChange={setCurrent} isRequired />
        <Input label="新密码" type="password" autoComplete="new-password" value={password} onValueChange={setPassword} isRequired minLength={12} />
        <Input label="确认新密码" type="password" autoComplete="new-password" value={confirm} onValueChange={setConfirm} isRequired />
        {error && <p role="alert" className="text-sm text-danger">{error}</p>}
        <Button type="submit" color="primary" isLoading={busy}>修改并退出登录</Button>
      </form>
    </CardBody></Card>
  </div>;
}
