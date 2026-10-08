import { useState, type FormEvent } from "react";
import { Button, Input, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Textarea } from "@heroui/react";
import type { Group } from "../../api/client";

interface Props {
  group: Group | null;
  busy: boolean;
  error: string;
  onClose: () => void;
  onSave: (body: { name: string; description: string }) => Promise<boolean>;
}
export default function GroupEditor({ group, busy, error, onClose, onSave }: Props) {
  const [name, setName] = useState(group?.name ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (await onSave({ name: name.trim(), description })) onClose();
  }
  return <Modal isOpen onClose={onClose} isDismissable={!busy} hideCloseButton={busy}><ModalContent><form onSubmit={submit}>
    <ModalHeader>{group ? "编辑用户组" : "新增用户组"}</ModalHeader>
    <ModalBody><Input label="组名" isRequired maxLength={64} value={name} onValueChange={setName} /><Textarea label="说明" maxLength={500} value={description} onValueChange={setDescription} />{error && <p role="alert" className="text-sm text-danger">{error}</p>}</ModalBody>
    <ModalFooter><Button variant="light" isDisabled={busy} onPress={onClose}>取消</Button><Button color="primary" type="submit" isLoading={busy}>保存用户组</Button></ModalFooter>
  </form></ModalContent></Modal>;
}
