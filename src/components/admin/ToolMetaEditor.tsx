import { useState, type FormEvent } from "react";
import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Textarea,
} from "@heroui/react";
import type { Category, ToolPolicy } from "../../api/client";

export interface ToolMetaBody {
  name: string;
  emoji: string;
  description: string;
  tags: string[];
  category_id: number | null;
  sort: number;
}

interface Props {
  tool: ToolPolicy;
  categories: Category[];
  busy: boolean;
  error: string;
  onClose: () => void;
  onSave: (body: ToolMetaBody) => Promise<boolean>;
  onReset: () => Promise<boolean>;
}

export default function ToolMetaEditor({ tool, categories, busy, error, onClose, onSave, onReset }: Props) {
  const [name, setName] = useState(tool.name);
  const [emoji, setEmoji] = useState(tool.emoji);
  const [description, setDescription] = useState(tool.description);
  const [tags, setTags] = useState<string[]>(tool.tags);
  const [tagInput, setTagInput] = useState("");
  const [categoryId, setCategoryId] = useState(String(tool.category_id ?? 0));
  const [sort, setSort] = useState(String(tool.sort));
  const [validation, setValidation] = useState("");
  const categoryOptions = [
    { id: "0", name: "未分类" },
    ...categories.map((category) => ({ id: String(category.id), name: category.name })),
  ];

  function addTag() {
    const tag = tagInput.trim();
    if (!tag || tags.includes(tag) || tags.length >= 12) return;
    setTags((previous) => [...previous, tag]);
    setTagInput("");
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setValidation("");
    const numericSort = Number(sort);
    if (!name.trim()) {
      setValidation("工具名称不能为空");
      return;
    }
    if (!Number.isInteger(numericSort) || numericSort < 0 || numericSort > 9999) {
      setValidation("排序值须为 0–9999 的整数");
      return;
    }
    const selected = Number(categoryId);
    const body: ToolMetaBody = {
      name: name.trim(),
      emoji: emoji.trim(),
      description,
      tags,
      category_id: selected ? selected : null,
      sort: numericSort,
    };
    if (await onSave(body)) onClose();
  }

  return (
    <Modal isOpen onClose={onClose} isDismissable={!busy} hideCloseButton={busy} size="2xl" scrollBehavior="inside">
      <ModalContent>
        <form onSubmit={submit}>
          <ModalHeader>
            <div>
              <p>编辑工具信息 · {tool.id}</p>
              <p className="mt-1 text-xs font-normal text-default-500">
                名称、Logo、说明、分类、标签和排序都保存在后台，随时可改
              </p>
            </div>
          </ModalHeader>
          <ModalBody className="gap-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_120px]">
              <Input label="工具名称" isRequired value={name} onValueChange={setName} maxLength={60} />
              <Input label="Logo / 图标" isRequired value={emoji} onValueChange={setEmoji} maxLength={8} description="emoji 或短字符" />
            </div>
            <Textarea label="工具说明" value={description} onValueChange={setDescription} maxLength={300} minRows={3} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Select
                label="所属分类"
                selectedKeys={[categoryId]}
                disallowEmptySelection
                onSelectionChange={(keys) => setCategoryId(String(Array.from(keys)[0]))}
              >
                {categoryOptions.map((option) => (
                  <SelectItem key={option.id}>{option.name}</SelectItem>
                ))}
              </Select>
              <Input
                label="排序值"
                type="number"
                value={sort}
                onValueChange={setSort}
                description="同类内从小到大展示，建议留出间隔"
              />
            </div>
            <div className="flex flex-col gap-2">
              <Input
                label="标签"
                value={tagInput}
                onValueChange={setTagInput}
                description="输入后按回车或点添加，最多 12 个"
                maxLength={24}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    addTag();
                  }
                }}
                endContent={
                  <Button size="sm" variant="flat" onPress={addTag} isDisabled={!tagInput.trim()}>
                    添加
                  </Button>
                }
              />
              <div className="flex min-h-8 flex-wrap gap-1.5">
                {tags.map((tag) => (
                  <Chip
                    key={tag}
                    size="sm"
                    variant="flat"
                    isCloseable
                    onClose={() => setTags((previous) => previous.filter((item) => item !== tag))}
                  >
                    {tag}
                  </Chip>
                ))}
                {tags.length === 0 && <span className="text-xs text-default-400">暂无标签</span>}
              </div>
            </div>
            {(validation || error) && <p role="alert" className="text-sm text-danger">{validation || error}</p>}
          </ModalBody>
          <ModalFooter className="flex-wrap">
            <Button
              variant="light"
              color="warning"
              isDisabled={busy}
              onPress={() => void onReset()}
            >
              恢复默认信息
            </Button>
            <div className="flex-1" />
            <Button variant="light" isDisabled={busy} onPress={onClose}>取消</Button>
            <Button color="primary" type="submit" isLoading={busy}>保存工具信息</Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  );
}
