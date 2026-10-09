import { useCallback, useEffect, useState } from "react";
import { Button, Card, CardBody, Chip, Input, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Spinner, Tab, Tabs, Table, TableBody, TableCell, TableColumn, TableHeader, TableRow } from "@heroui/react";
import { api, json, type Audit, type Group, type ToolPolicy, type User } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import UserEditor, { type UserDraft } from "../components/admin/UserEditor";
import GroupEditor from "../components/admin/GroupEditor";
import PolicyEditor from "../components/admin/PolicyEditor";

type Editor = { kind: "user"; user: User | null } | { kind: "group"; group: Group | null } | { kind: "tool"; tool: ToolPolicy } | null;
type Removal = { kind: "user" | "group"; id: number; name: string } | null;

const sections: Record<string, { title: string; description: string }> = {
  users: { title: "用户账号", description: "创建账号、分配用户组，管理账号状态与密码。" },
  groups: { title: "用户组", description: "按团队或使用场景组织账号，统一分配工具权限。" },
  tools: { title: "工具权限", description: "分别设置游客、会员和用户组的可见与可用权限。" },
  audit: { title: "审计记录", description: "查看最新 200 条账号、权限与登录操作。" },
};

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
  const [isDesktop, setIsDesktop] = useState(() => window.matchMedia("(min-width: 768px)").matches);
  useEffect(() => {
    const media = window.matchMedia("(min-width: 768px)");
    const change = () => setIsDesktop(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
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
  const panelHeader = <div className="flex flex-col gap-5">
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 className="text-xl font-semibold tracking-tight">{sections[tab].title}</h2>
        <p className="mt-1.5 text-sm text-default-500">{sections[tab].description}</p>
      </div>
      {tab === "users" && <Button color="primary" isDisabled={busy} onPress={() => open({ kind: "user", user: null })}>新增账号</Button>}
      {tab === "groups" && <Button color="primary" isDisabled={busy} onPress={() => open({ kind: "group", group: null })}>新增用户组</Button>}
    </div>
    {error && <p role="alert" className="rounded-large bg-danger-50 p-3 text-sm text-danger">{error}</p>}
    {message && <p role="status" className="rounded-large bg-success-50 p-3 text-sm text-success-700">{message}</p>}
    {(tab === "users" || tab === "tools") && <Input aria-label="搜索管理数据" placeholder={tab === "users" ? "搜索用户名、显示名称或组名" : "搜索工具名称、ID 或分类"} value={query} onValueChange={setQuery} isClearable onClear={() => setQuery("")} variant="bordered" className="max-w-lg" />}
  </div>;
  return <div className="flex min-w-0 flex-col gap-7">
    <header className="flex flex-wrap items-center justify-between gap-4 border-b border-default-200 pb-6">
      <div><h1 className="text-2xl font-semibold tracking-tight">管理后台</h1><p className="mt-2 text-sm text-default-500">集中管理账号、分组与工具访问权限。</p></div>
      <Button variant="bordered" size="sm" isLoading={busy} onPress={() => { setBusy(true); setError(""); void load().catch(e => setError(e instanceof Error ? e.message : "加载失败")).finally(() => setBusy(false)); }}>刷新数据</Button>
    </header>
    <Tabs
      aria-label="管理栏目"
      selectedKey={tab}
      onSelectionChange={key => { setTab(String(key)); setQuery(""); }}
      isVertical={isDesktop}
      color="primary"
      variant="light"
      classNames={{
        tabWrapper: "flex w-full min-w-0 flex-col gap-6 md:flex-row md:items-start",
        base: "w-full min-w-0 shrink-0 md:sticky md:top-24 md:w-48",
        tabList: "w-full gap-1 rounded-large border border-default-200 bg-background p-2 shadow-small",
        tab: "h-11 px-3 md:justify-start",
        tabContent: "w-full group-data-[selected=true]:text-primary",
        cursor: "bg-primary-50 shadow-none",
        panel: "w-full min-w-0 flex-1 p-0",
      }}
    >
      <Tab key="users" title={<div className="flex items-center justify-between gap-3"><span>用户账号</span><Chip size="sm" variant="flat">{users.length}</Chip></div>}>
        <div className="flex min-w-0 flex-col gap-5">
          {panelHeader}
          <Table aria-label="用户账号列表" classNames={{ wrapper: "border border-default-200 shadow-none", table: "min-w-[600px]" }}>
            <TableHeader><TableColumn>账号</TableColumn><TableColumn>角色 / 状态</TableColumn><TableColumn>所属组</TableColumn><TableColumn>操作</TableColumn></TableHeader>
            <TableBody emptyContent="暂无匹配账号">
              {filteredUsers.map(user => <TableRow key={user.id}>
                <TableCell><p className="font-semibold">{user.display_name || user.username}</p><p className="text-xs text-default-500">{user.username}</p></TableCell>
                <TableCell><div className="flex flex-wrap gap-1"><Chip size="sm" color={user.role === "admin" ? "primary" : "default"}>{user.role === "admin" ? "管理员" : "普通用户"}</Chip><Chip size="sm" variant="flat" color={user.disabled ? "danger" : "success"}>{user.disabled ? "禁用" : "启用"}</Chip></div></TableCell>
                <TableCell><div className="flex flex-wrap gap-1">{user.groups.length ? user.groups.map(group => <Chip key={group.id} size="sm" variant="bordered">{group.name}</Chip>) : <span className="text-default-400">无分组</span>}</div></TableCell>
                <TableCell><div className="flex gap-2"><Button size="sm" variant="flat" isDisabled={busy} onPress={() => open({ kind: "user", user })}>编辑 / 重置密码</Button><Button size="sm" color="danger" variant="light" isDisabled={busy || user.id === session?.user?.id} onPress={() => { setError(""); setRemoval({ kind: "user", id: user.id, name: user.username }); }}>删除</Button></div></TableCell>
              </TableRow>)}
            </TableBody>
          </Table>
          <p className="text-xs leading-6 text-default-500">无法删除当前账号；不能删除、禁用或降级最后一个启用的管理员。修改密码、禁用和改变角色会撤销该账号所有会话。</p>
        </div>
      </Tab>
      <Tab key="groups" title={<div className="flex items-center justify-between gap-3"><span>用户组</span><Chip size="sm" variant="flat">{groups.length}</Chip></div>}>
        <div className="flex min-w-0 flex-col gap-5">
          {panelHeader}
          <div className="grid gap-4 xl:grid-cols-2">{groups.map(group => <Card key={group.id} shadow="none" className="border border-default-200"><CardBody className="gap-3 p-5"><h3 className="font-semibold">{group.name}</h3><p className="min-h-5 text-sm text-default-500">{group.description || "无说明"}</p><p className="text-xs text-default-500">{users.filter(user => user.groups.some(item => item.id === group.id)).length} 个账号 · {tools.filter(tool => tool.group_rules.some(rule => rule.group_id === group.id)).length} 项工具覆盖</p><div className="flex gap-2"><Button size="sm" variant="flat" isDisabled={busy} onPress={() => open({ kind: "group", group })}>编辑用户组</Button><Button size="sm" color="danger" variant="light" isDisabled={busy} onPress={() => { setError(""); setRemoval({ kind: "group", id: group.id, name: group.name }); }}>删除用户组</Button></div></CardBody></Card>)}</div>
          {!groups.length && <Card shadow="none" className="border border-dashed border-default-300"><CardBody className="gap-2 py-16 text-center"><p className="font-medium">还没有用户组</p><p className="text-sm text-default-500">创建用户组后，即可为一组账号配置工具权限。</p></CardBody></Card>}
        </div>
      </Tab>
      <Tab key="tools" title={<div className="flex items-center justify-between gap-3"><span>工具权限</span><Chip size="sm" variant="flat">{tools.length}</Chip></div>}>
        <div className="flex min-w-0 flex-col gap-5">
          {panelHeader}
          <div className="flex flex-col gap-3">{filteredTools.map(tool => <Card key={tool.id} shadow="none" className="border border-default-200"><CardBody className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between"><div className="min-w-0"><h3 className="font-semibold">{tool.name}</h3><p className="mt-1 text-xs text-default-500">{tool.id} · {tool.category}</p><div className="mt-3 flex flex-wrap gap-2"><Chip size="sm" color={tool.enabled ? "success" : "danger"}>{tool.enabled ? "启用" : "停用"}</Chip><Chip size="sm" variant="flat">游客：{!tool.guest_visible ? "隐藏" : tool.guest_use ? "可用" : "仅可见"}</Chip><Chip size="sm" variant="flat">会员：{!tool.member_visible ? "隐藏" : tool.member_use ? "可用" : "仅可见"}</Chip><Chip size="sm" variant="bordered">{tool.group_rules.length} 组覆盖</Chip></div></div><Button color="primary" variant="flat" className="shrink-0" isDisabled={busy} onPress={() => open({ kind: "tool", tool })}>配置权限</Button></CardBody></Card>)}{!filteredTools.length && <p className="py-8 text-center text-default-500">暂无匹配工具。</p>}</div>
        </div>
      </Tab>
      <Tab key="audit" title={<div className="text-left">审计记录</div>}>
        <div className="flex min-w-0 flex-col gap-5">
          {panelHeader}
          <Table aria-label="审计记录" classNames={{ wrapper: "border border-default-200 shadow-none", table: "min-w-[600px]" }}><TableHeader><TableColumn>时间</TableColumn><TableColumn>操作者</TableColumn><TableColumn>操作</TableColumn><TableColumn>对象</TableColumn></TableHeader><TableBody emptyContent="暂无记录">{audits.map(item => <TableRow key={item.id}><TableCell>{new Date(item.created_at).toLocaleString()}</TableCell><TableCell>{item.actor}</TableCell><TableCell>{item.action}</TableCell><TableCell>{item.target}</TableCell></TableRow>)}</TableBody></Table>
          <p className="text-xs text-default-500">不记录密码或会话令牌。</p>
        </div>
      </Tab>
    </Tabs>
    {editor?.kind === "user" && <UserEditor user={editor.user} groups={groups} busy={busy} error={error} onClose={() => { if (!busy) setEditor(null); }} onSave={(body: UserDraft) => update(`/admin/users${editor.user ? `/${editor.user.id}` : ""}`, editor.user ? "PUT" : "POST", body, "账号已保存")} />}
    {editor?.kind === "group" && <GroupEditor group={editor.group} busy={busy} error={error} onClose={() => { if (!busy) setEditor(null); }} onSave={body => update(`/admin/groups${editor.group ? `/${editor.group.id}` : ""}`, editor.group ? "PUT" : "POST", body, "用户组已保存")} />}
    {editor?.kind === "tool" && <PolicyEditor policy={editor.tool} groups={groups} busy={busy} error={error} onClose={() => { if (!busy) setEditor(null); }} onSave={body => update(`/admin/tools/${editor.tool.id}`, "PUT", body, "工具权限已保存")} />}
    <Modal isOpen={!!removal} onClose={() => { if (!busy) setRemoval(null); }} isDismissable={!busy} hideCloseButton={busy}><ModalContent><ModalHeader>确认删除 {removal?.name}</ModalHeader><ModalBody><p>{removal?.kind === "group" ? "会移除所有账号的该组归属以及工具中的该组覆盖规则。用户权限将按其余组与会员默认重新计算，可能增加或减少访问权限。" : "账号与所有会话将被永久删除，无法撤销。"}</p>{error && <p role="alert" className="text-danger">{error}</p>}</ModalBody><ModalFooter><Button variant="light" isDisabled={busy} onPress={() => setRemoval(null)}>取消</Button><Button color="danger" isLoading={busy} onPress={() => { if (removal) void update(`/admin/${removal.kind === "group" ? "groups" : "users"}/${removal.id}`, "DELETE", undefined, "删除成功").then(ok => { if (ok) setRemoval(null); }); }}>确认删除</Button></ModalFooter></ModalContent></Modal>
  </div>;
}
