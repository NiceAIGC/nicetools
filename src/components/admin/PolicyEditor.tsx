import { useState } from "react";
import { Button, Divider, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Select, SelectItem, Switch } from "@heroui/react";
import type { Group, GroupRule, ToolPolicy } from "../../api/client";

interface Props {
  policy: ToolPolicy;
  groups: Group[];
  busy: boolean;
  error: string;
  onClose: () => void;
  onSave: (policy: ToolPolicy) => Promise<boolean>;
}
function TriState({ label, value, onChange }: { label: string; value: boolean | null; onChange: (value: boolean | null) => void }) {
  return <Select label={label} selectedKeys={[value === null ? "inherit" : value ? "allow" : "deny"]} disallowEmptySelection size="sm" onSelectionChange={keys => { const key = Array.from(keys)[0]; onChange(key === "inherit" ? null : key === "allow"); }}>
    <SelectItem key="inherit">继承会员默认</SelectItem><SelectItem key="allow">允许</SelectItem><SelectItem key="deny">拒绝（优先）</SelectItem>
  </Select>;
}
export default function PolicyEditor({ policy, groups, busy, error, onClose, onSave }: Props) {
  const [draft, setDraft] = useState<ToolPolicy>(() => ({ ...policy, group_rules: policy.group_rules.map(rule => ({ ...rule })) }));
  function flag(field: "enabled" | "guest_visible" | "guest_use" | "member_visible" | "member_use", value: boolean) {
    setDraft(previous => {
      const next = { ...previous, [field]: value };
      if (field === "guest_use" && value) next.guest_visible = true;
      if (field === "guest_visible" && !value) next.guest_use = false;
      if (field === "member_use" && value) next.member_visible = true;
      if (field === "member_visible" && !value) next.member_use = false;
      return next;
    });
  }
  function ruleChange(groupID: number, field: "visible" | "use", value: boolean | null) {
    setDraft(previous => {
      const old = previous.group_rules.find(rule => rule.group_id === groupID);
      const next: GroupRule = { group_id: groupID, visible: old?.visible ?? null, use: old?.use ?? null, [field]: value };
      if (field === "use" && value === true) next.visible = true;
      if (field === "visible" && value !== true && next.use === true) next.use = value;
      return { ...previous, group_rules: [...previous.group_rules.filter(rule => rule.group_id !== groupID), next].filter(rule => rule.visible !== null || rule.use !== null) };
    });
  }
  async function save() { if (await onSave(draft)) onClose(); }
  return <Modal isOpen onClose={onClose} isDismissable={!busy} hideCloseButton={busy} size="3xl" scrollBehavior="inside"><ModalContent>
    <ModalHeader>工具策略 · {policy.name}</ModalHeader>
    <ModalBody className="gap-5">
      <Switch isSelected={draft.enabled} onValueChange={value => flag("enabled", value)}>启用工具</Switch>
      <p className="text-xs text-default-500">停用后普通用户和游客不可见，管理员只能查看策略，不能运行。</p>
      <div className="grid gap-5 sm:grid-cols-2">
        <section className="flex flex-col gap-3 rounded-large border border-default-200 p-4"><h3 className="font-semibold">游客（未登录）</h3><Switch isSelected={draft.guest_visible} onValueChange={value => flag("guest_visible", value)}>游客可见</Switch><Switch isSelected={draft.guest_use} onValueChange={value => flag("guest_use", value)}>游客可用</Switch></section>
        <section className="flex flex-col gap-3 rounded-large border border-default-200 p-4"><h3 className="font-semibold">已登录用户默认</h3><Switch isSelected={draft.member_visible} onValueChange={value => flag("member_visible", value)}>会员默认可见</Switch><Switch isSelected={draft.member_use} onValueChange={value => flag("member_use", value)}>会员默认可用</Switch></section>
      </div>
      <Divider />
      <div><h3 className="font-semibold">用户组覆盖规则</h3><p className="mt-2 text-sm text-default-500">每项分别计算：任一所属组显式拒绝优先，其次显式允许，否则使用会员默认。可用必须同时可见。管理员绕过分组权限。</p></div>
      {groups.length ? groups.map(group => {
        const rule = draft.group_rules.find(item => item.group_id === group.id);
        return <section key={group.id} className="grid items-center gap-3 rounded-large bg-default-50 p-3 sm:grid-cols-[1fr_1fr_1fr]"><div><h4 className="font-medium">{group.name}</h4><p className="text-xs text-default-500">{group.description}</p></div><TriState label={`${group.name} · 可见`} value={rule?.visible ?? null} onChange={value => ruleChange(group.id, "visible", value)} /><TriState label={`${group.name} · 可用`} value={rule?.use ?? null} onChange={value => ruleChange(group.id, "use", value)} /></section>;
      }) : <p className="text-sm text-default-500">暂无用户组。在“用户组”中创建后即可配置。</p>}
      {error && <p role="alert" className="text-sm text-danger">{error}</p>}
    </ModalBody>
    <ModalFooter><Button variant="light" isDisabled={busy} onPress={onClose}>取消</Button><Button color="primary" isLoading={busy} onPress={() => void save()}>保存工具策略</Button></ModalFooter>
  </ModalContent></Modal>;
}
