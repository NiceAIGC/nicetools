import { useCallback, useEffect, useState } from "react";
import { Button, Card, CardBody, Chip, Input, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Spinner, Tab, Tabs, Table, TableBody, TableCell, TableColumn, TableHeader, TableRow } from "@heroui/react";
import { api, json, type Audit, type Group, type ToolPolicy, type User } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import UserEditor, { type UserDraft } from "../components/admin/UserEditor";
import GroupEditor from "../components/admin/GroupEditor";
import PolicyEditor from "../components/admin/PolicyEditor";

type Editor = { kind: "user"; user: User | null } | { kind: "group"; group: Group | null } | { kind: "tool"; tool: ToolPolicy } | null;
type Removal = { kind: "user" | "group"; id: number; name: string } | null;

export default function Admin() {
  const { session, refresh } = useAuth();
  const [tab, setTab] = useState("users");
  const [users, setUsers] = useState<User[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [tools, setTools] = useState<ToolPolicy[]>([]);
  const [audits, setAudits] = useState<Audit[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [query, setQuery] = useState("");
  const [editor, setEditor] = useState<Editor>(null);
  const [removal, setRemoval] = useState<Removal>(null);
  const load = useCallback(async () => {
    const [newUsers, newGroups, newTools, newAudits] = await Promise.all([
      api<User[]>("/admin/users"), api<Group[]>("/admin/groups"), api<ToolPolicy[]>("/admin/tools"), api<Audit[]>("/admin/audit"),
    ]);
    setUsers(newUsers); setGroups(newGroups); setTools(newTools); setAudits(newAudits);
  }, []);
  useEffect(() => {
    if (session?.user?.role !== "admin") return;
    let live = true;
    load().catch(e => { if (live) setError(e instanceof Error ? e.message : "加载失败"); }).finally(() => { if (live) setLoading(false); });
    return () => { live = false; };
  }, [load, session?.user?.id, session?.user?.role]);
  async function update(path: string, method: string, body: unknown, success: string): Promise<boolean> {
    setBusy(true); setError(""); setMessage("");
    try {
      await api(path, body === undefined ? { method } : json(method, body));
      await refresh();
      await load();
      setMessage(success);
      return true;
    } catch (e) { setError(e instanceof Error ? e.message : "操作失败"); return false; }
    finally { setBusy(false); }
  }
  function open(next: Editor) { setError(""); setMessage(""); setEditor(next); }
  const needle = query.trim().toLowerCase();
  const filteredUsers = users.filter(user => [user.username, user.display_name, ...user.groups.map(group => group.name)].join(" ").toLowerCase().includes(needle));
  const filteredTools = tools.filter(tool => `${tool.name} ${tool.category} ${tool.id}`.toLowerCase().includes(needle));
  if (loading) return <Spinner label="加载管理数据…" />;
  return <div className="flex min-w-0 flex-col gap-5">
    <header className="flex flex-wrap items-start justify-between gap-3"><div><h1 className="text-2xl font-bold">管理后台</h1><p className="mt-2 text-sm text-default-500">账号仅由管理员创建。工具的可见与可用独立配置，用户可加入多个组。</p></div><Button variant="bordered" isLoading={busy} onPress={() => { setBusy(true); setError(""); void load().catch(e => setError(e instanceof Error ? e.message : "加载失败")).finally(() => setBusy(false)); }}>刷新数据</Button></header>
    <div className="grid gap-3 sm:grid-cols-3"><Card><CardBody><p className="text-sm text-default-500">账号</p><p className="text-2xl font-semibold">{users.length}</p></CardBody></Card><Card><CardBody><p className="text-sm text-default-500">用户组</p><p className="text-2xl font-semibold">{groups.length}</p></CardBody></Card><Card><CardBody><p className="text-sm text-default-500">工具</p><p className="text-2xl font-semibold">{tools.length}</p></CardBody></Card></div>
    {error && <p role="alert" className="rounded-large bg-danger-50 p-3 text-sm text-danger">{error}</p>}
    {message && <p role="status" className="rounded-large bg-success-50 p-3 text-sm text-success-700">{message}</p>}
    <Tabs aria-label="管理栏目" selectedKey={tab} onSelectionChange={key => { setTab(String(key)); setQuery(""); }} color="primary" variant="underlined">
      <Tab key="users" title="用户账号" /><Tab key="groups" title="用户组" /><Tab key="tools" title="工具权限" /><Tab key="audit" title="审计记录" />
    </Tabs>
    {(tab === "users" || tab === "tools") && <Input aria-label="搜索管理数据" placeholder={tab === "users" ? "搜索用户名、显示名称或组名" : "搜索工具名称、ID 或分类"} value={query} onValueChange={setQuery} isClearable onClear={() => setQuery("")} className="max-w-lg" />}
    {tab === "users" && <>
      <Button color="primary" className="self-start" isDisabled={busy} onPress={() => open({ kind: "user", user: null })}>新增账号</Button>
      <div className="overflow-x-auto"><Table aria-label="用户账号列表" className="min-w-[640px]"><TableHeader><TableColumn>账号</TableColumn><TableColumn>角色 / 状态</TableColumn><TableColumn>所属组</TableColumn><TableColumn>操作</TableColumn></TableHeader><TableBody emptyContent="暂无匹配账号">
        {filteredUsers.map(user => <TableRow key={user.id}><TableCell><p className="font-semibold">{user.display_name || user.username}</p><p className="text-xs text-default-500">{user.username}</p></TableCell><TableCell><div className="flex flex-wrap gap-1"><Chip size="sm" color={user.role === "admin" ? "primary" : "default"}>{user.role === "admin" ? "管理员" : "普通用户"}</Chip><Chip size="sm" variant="flat" color={user.disabled ? "danger" : "success"}>{user.disabled ? "禁用" : "启用"}</Chip></div></TableCell><TableCell><div className="flex flex-wrap gap-1">{user.groups.length ? user.groups.map(group => <Chip key={group.id} size="sm" variant="bordered">{group.name}</Chip>) : <span className="text-default-400">无分组</span>}</div></TableCell><TableCell><div className="flex gap-2"><Button size="sm" variant="flat" isDisabled={busy} onPress={() => open({ kind: "user", user })}>编辑 / 重置密码</Button><Button size="sm" color="danger" variant="light" isDisabled={busy || user.id === session?.user?.id} onPress={() => { setError(""); setRemoval({ kind: "user", id: user.id, name: user.username }); }}>删除</Button></div></TableCell></TableRow>)}
      </TableBody></Table></div>
      <p className="text-xs text-default-500">无法删除当前账号；不能删除、禁用或降级最后一个启用的管理员。修改密码、禁用和改变角色会撤销该账号所有会话。</p>
    </>}
    {tab === "groups" && <>
      <Button color="primary" className="self-start" isDisabled={busy} onPress={() => open({ kind: "group", group: null })}>新增用户组</Button>
      <div className="grid gap-3 sm:grid-cols-2">{groups.map(group => <Card key={group.id}><CardBody className="gap-3"><h2 className="font-semibold">{group.name}</h2><p className="min-h-5 text-sm text-default-500">{group.description || "无说明"}</p><p className="text-xs text-default-500">{users.filter(user => user.groups.some(item => item.id === group.id)).length} 个账号 · {tools.filter(tool => tool.group_rules.some(rule => rule.group_id === group.id)).length} 项工具覆盖</p><div className="flex gap-2"><Button size="sm" variant="flat" isDisabled={busy} onPress={() => open({ kind: "group", group })}>编辑用户组</Button><Button size="sm" color="danger" variant="light" isDisabled={busy} onPress={() => { setError(""); setRemoval({ kind: "group", id: group.id, name: group.name }); }}>删除用户组</Button></div></CardBody></Card>)}</div>
      {!groups.length && <p className="py-8 text-center text-default-500">暂无用户组。</p>}
    </>}
    {tab === "tools" && <div className="flex flex-col gap-3">{filteredTools.map(tool => <Card key={tool.id}><CardBody className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div><h2 className="font-semibold">{tool.name}</h2><p className="mt-1 text-xs text-default-500">{tool.id} · {tool.category}</p><div className="mt-3 flex flex-wrap gap-2"><Chip size="sm" color={tool.enabled ? "success" : "danger"}>{tool.enabled ? "启用" : "停用"}</Chip><Chip size="sm" variant="flat">游客：{!tool.guest_visible ? "隐藏" : tool.guest_use ? "可用" : "仅可见"}</Chip><Chip size="sm" variant="flat">会员：{!tool.member_visible ? "隐藏" : tool.member_use ? "可用" : "仅可见"}</Chip><Chip size="sm" variant="bordered">{tool.group_rules.length} 组覆盖</Chip></div></div><Button color="primary" variant="flat" isDisabled={busy} onPress={() => open({ kind: "tool", tool })}>配置权限</Button></CardBody></Card>)}{!filteredTools.length && <p className="text-default-500">暂无匹配工具。</p>}</div>}
    {tab === "audit" && <div className="overflow-x-auto"><p className="mb-3 text-xs text-default-500">最新 200 条账号、权限与登录操作；不记录密码或会话令牌。</p><Table aria-label="审计记录" className="min-w-[600px]"><TableHeader><TableColumn>时间</TableColumn><TableColumn>操作者</TableColumn><TableColumn>操作</TableColumn><TableColumn>对象</TableColumn></TableHeader><TableBody emptyContent="暂无记录">{audits.map(item => <TableRow key={item.id}><TableCell>{new Date(item.created_at).toLocaleString()}</TableCell><TableCell>{item.actor}</TableCell><TableCell>{item.action}</TableCell><TableCell>{item.target}</TableCell></TableRow>)}</TableBody></Table></div>}
    {editor?.kind === "user" && <UserEditor user={editor.user} groups={groups} busy={busy} error={error} onClose={() => { if (!busy) setEditor(null); }} onSave={(body: UserDraft) => update(`/admin/users${editor.user ? `/${editor.user.id}` : ""}`, editor.user ? "PUT" : "POST", body, "账号已保存")} />}
    {editor?.kind === "group" && <GroupEditor group={editor.group} busy={busy} error={error} onClose={() => { if (!busy) setEditor(null); }} onSave={body => update(`/admin/groups${editor.group ? `/${editor.group.id}` : ""}`, editor.group ? "PUT" : "POST", body, "用户组已保存")} />}
    {editor?.kind === "tool" && <PolicyEditor policy={editor.tool} groups={groups} busy={busy} error={error} onClose={() => { if (!busy) setEditor(null); }} onSave={body => update(`/admin/tools/${editor.tool.id}`, "PUT", body, "工具权限已保存")} />}
    <Modal isOpen={!!removal} onClose={() => { if (!busy) setRemoval(null); }} isDismissable={!busy} hideCloseButton={busy}><ModalContent><ModalHeader>确认删除 {removal?.name}</ModalHeader><ModalBody><p>{removal?.kind === "group" ? "会移除所有账号的该组归属以及工具中的该组覆盖规则。用户权限将按其余组与会员默认重新计算，可能增加或减少访问权限。" : "账号与所有会话将被永久删除，无法撤销。"}</p>{error && <p role="alert" className="text-danger">{error}</p>}</ModalBody><ModalFooter><Button variant="light" isDisabled={busy} onPress={() => setRemoval(null)}>取消</Button><Button color="danger" isLoading={busy} onPress={() => { if (removal) void update(`/admin/${removal.kind === "group" ? "groups" : "users"}/${removal.id}`, "DELETE", undefined, "删除成功").then(ok => { if (ok) setRemoval(null); }); }}>确认删除</Button></ModalFooter></ModalContent></Modal>
  </div>;
}
