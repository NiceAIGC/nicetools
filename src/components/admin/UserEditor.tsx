import { useState, type FormEvent } from "react";
import { Button, Input, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Select, SelectItem, Switch } from "@heroui/react";
import type { Group, User } from "../../api/client";

export interface UserDraft {
  username?: string;
  display_name: string;
  password?: string;
  role: "admin" | "user";
  disabled: boolean;
  group_ids: number[];
}
interface Props {
  user: User | null;
  groups: Group[];
  busy: boolean;
  error: string;
  onClose: () => void;
  onSave: (body: UserDraft) => Promise<boolean>;
}
export default function UserEditor({ user, groups, busy, error, onClose, onSave }: Props) {
  const [username, setUsername] = useState(user?.username ?? "");
  const [displayName, setDisplayName] = useState(user?.display_name ?? "");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "user">(user?.role ?? "user");
  const [disabled, setDisabled] = useState(user?.disabled ?? false);
  const [groupIDs, setGroupIDs] = useState<string[]>(user?.groups.map(group => String(group.id)) ?? []);
  const [validation, setValidation] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault(); setValidation("");
    const bytes = new TextEncoder().encode(password).length;
    if ((!user || password) && (bytes < 12 || bytes > 72)) { setValidation("密码须为 12–72 字节"); return; }
    const body: UserDraft = { display_name: displayName, role, disabled, group_ids: groupIDs.map(Number) };
    if (!user) body.username = username;
    if (!user || password) body.password = password;
    if (await onSave(body)) onClose();
  }
  return <Modal isOpen onClose={onClose} isDismissable={!busy} hideCloseButton={busy} size="xl" scrollBehavior="inside"><ModalContent>
    <form onSubmit={submit}>
      <ModalHeader>{user ? `编辑账号 · ${user.username}` : "新增账号"}</ModalHeader>
      <ModalBody className="gap-4">
        <Input label="用户名" value={username} onValueChange={setUsername} isRequired isReadOnly={!!user} minLength={3} maxLength={32} pattern="[a-zA-Z0-9_.\-]{3,32}" description="3–32 位字母、数字或 _.-；创建后不可修改" />
        <Input label="显示名称" value={displayName} onValueChange={setDisplayName} maxLength={64} />
        <Input label={user ? "重置密码（留空不修改）" : "初始密码"} type="password" value={password} onValueChange={setPassword} isRequired={!user} autoComplete="new-password" description={user ? "重置后该账号所有设备退出登录" : "12–72 字节；请安全地交付给账号使用者"} />
        <Select label="账号角色" selectedKeys={[role]} disallowEmptySelection onSelectionChange={keys => setRole(Array.from(keys)[0] === "admin" ? "admin" : "user")}>
          <SelectItem key="user">普通用户</SelectItem><SelectItem key="admin">管理员（可管理所有账号和策略）</SelectItem>
        </Select>
        <Select label="所属用户组" placeholder={groups.length ? "选择一个或多个组" : "暂无用户组，请先创建"} selectionMode="multiple" selectedKeys={groupIDs} onSelectionChange={keys => setGroupIDs(keys === "all" ? groups.map(group => String(group.id)) : Array.from(keys, String))} isDisabled={!groups.length}>
          {groups.map(group => <SelectItem key={String(group.id)} textValue={group.name}>{group.name}</SelectItem>)}
        </Select>
        <Switch isSelected={disabled} onValueChange={setDisabled}>禁用账号（即时撤销会话）</Switch>
        {(validation || error) && <p role="alert" className="text-sm text-danger">{validation || error}</p>}
      </ModalBody>
      <ModalFooter><Button variant="light" isDisabled={busy} onPress={onClose}>取消</Button><Button color="primary" type="submit" isLoading={busy}>{user ? "保存账号" : "创建账号"}</Button></ModalFooter>
    </form>
  </ModalContent></Modal>;
}
