import { useState, type FormEvent } from "react";
import { Button, Input, Modal, ModalBody, ModalContent, ModalFooter, ModalHeader, Textarea } from "@heroui/react";
import type { Category } from "../../api/client";

export interface CategoryBody {
  name: string;
  description: string;
}

interface Props {
  category: Category | null;
  busy: boolean;
  error: string;
  onClose: () => void;
  onSave: (body: CategoryBody) => Promise<boolean>;
}

export default function CategoryEditor({ category, busy, error, onClose, onSave }: Props) {
  const [name, setName] = useState(category?.name ?? "");
  const [description, setDescription] = useState(category?.description ?? "");
  const [validation, setValidation] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setValidation("");
    if (!name.trim()) {
      setValidation("分类名称不能为空");
      return;
    }
    if (await onSave({ name: name.trim(), description })) onClose();
  }

  return (
    <Modal isOpen onClose={onClose} isDismissable={!busy} hideCloseButton={busy}>
      <ModalContent>
        <form onSubmit={submit}>
          <ModalHeader>{category ? `编辑分类 · ${category.name}` : "新增分类"}</ModalHeader>
          <ModalBody className="gap-4">
            <Input label="分类名称" isRequired value={name} onValueChange={setName} maxLength={32} />
            <Textarea label="分类说明" value={description} onValueChange={setDescription} maxLength={200} minRows={2} />
            <p className="text-xs text-default-500">新建分类会排在最后，顺序可用列表中的「上移 / 下移」调整。</p>
            {(validation || error) && <p role="alert" className="text-sm text-danger">{validation || error}</p>}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" isDisabled={busy} onPress={onClose}>取消</Button>
            <Button color="primary" type="submit" isLoading={busy}>保存分类</Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  );
}
