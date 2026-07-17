import { useState } from "react";
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  Input,
  Textarea,
} from "@heroui/react";
import { copyText } from "../../utils/copyText";
import { extractJsonValues, type ExtractionResult } from "./extract";

const example = `{"id":1,"name":"张三","age":25}
{"id":2,"name":"李四","age":30}
{"id":3,"name":"王五","age":35}`;

export default function JsonValueExtractor() {
  const [source, setSource] = useState("");
  const [keyName, setKeyName] = useState("");
  const [result, setResult] = useState<ExtractionResult | null>(null);
  const [status, setStatus] = useState("等待输入");
  const [isError, setIsError] = useState(false);

  function extract() {
    const key = keyName.trim();
    if (!source.trim()) {
      setStatus("请输入 JSON 文本");
      setIsError(true);
      return;
    }
    if (!key) {
      setStatus("请输入要提取的键名");
      setIsError(true);
      return;
    }

    const next = extractJsonValues(source, key);
    setResult(next);
    if (next.errors.length > 0) {
      setStatus(`提取完成，${next.errors.length} 行未成功`);
      setIsError(true);
    } else {
      setStatus("提取成功");
      setIsError(false);
    }
  }

  async function copyResult() {
    if (!result) return;
    const copied = await copyText(result.output);
    setStatus(copied ? "结果已复制" : "复制失败");
    setIsError(!copied);
  }

  function clear() {
    setSource("");
    setKeyName("");
    setResult(null);
    setStatus("已清空");
    setIsError(false);
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex justify-end" aria-live="polite">
        <Chip color={isError ? "danger" : "default"} variant="flat" size="sm">
          {status}
        </Chip>
      </div>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="text-base font-semibold">提取设置</CardHeader>
        <CardBody className="gap-5">
          <Textarea
            label="JSON 文本（每行一个 JSON 对象）"
            labelPlacement="outside"
            placeholder={example}
            value={source}
            onValueChange={setSource}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
                event.preventDefault();
                extract();
              }
            }}
            minRows={8}
            variant="bordered"
            classNames={{ input: "font-mono text-sm" }}
          />

          <Input
            label="要提取的键名"
            labelPlacement="outside"
            placeholder="例如：name"
            description="按键名精确匹配每个 JSON 对象的顶层字段"
            value={keyName}
            onValueChange={setKeyName}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                extract();
              }
            }}
            variant="bordered"
            startContent={<span aria-hidden>🔑</span>}
          />

          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <Button color="primary" onPress={extract}>
              提取值
            </Button>
            <Button variant="bordered" onPress={copyResult} isDisabled={!result}>
              复制结果
            </Button>
            <Button variant="light" onPress={clear}>
              清空
            </Button>
          </div>
          <p className="text-xs text-default-400">
            在 JSON 文本框内按 Ctrl/Command + Enter 可快速提取。
          </p>
        </CardBody>
      </Card>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-base font-semibold">提取结果</span>
          {result && (
            <span className="text-xs text-default-400">
              提取键：“{keyName.trim()}”
            </span>
          )}
        </CardHeader>
        <CardBody className="gap-4">
          <Textarea
            aria-label="提取结果"
            placeholder="提取的结果将显示在这里…"
            value={result?.output ?? ""}
            isReadOnly
            minRows={8}
            variant="bordered"
            classNames={{ input: "font-mono text-sm" }}
          />

          {result && (
            <div className="flex flex-wrap gap-2 text-sm">
              <Chip variant="flat">共处理 {result.total} 行</Chip>
              <Chip color="success" variant="flat">
                成功 {result.success} 行
              </Chip>
              <Chip color={result.errors.length ? "danger" : "default"} variant="flat">
                错误 {result.errors.length} 行
              </Chip>
            </div>
          )}

          {result && result.errors.length > 0 && (
            <div className="rounded-medium border border-danger-200 bg-danger-50 p-3">
              <p className="mb-2 text-sm font-semibold text-danger-700">错误详情</p>
              <ol className="max-h-40 list-inside list-decimal space-y-1 overflow-y-auto text-xs text-danger-600">
                {result.errors.map((error) => (
                  <li key={`${error.line}-${error.message}`}>
                    行 {error.line}：{error.message}
                  </li>
                ))}
              </ol>
            </div>
          )}
        </CardBody>
      </Card>

      <Card shadow="none" className="border border-primary-100 bg-primary-50">
        <CardHeader className="text-sm font-semibold text-primary-700">
          💡 使用示例
        </CardHeader>
        <CardBody className="gap-2 pt-0 text-sm text-primary-700">
          <pre className="overflow-x-auto rounded-medium bg-background p-3 font-mono text-xs text-foreground">
            {example}
          </pre>
          <p>键名填写 “name”，输出为张三、李四、王五，每个值单独一行。</p>
          <p>如果值是对象或数组，工具会将它压缩为 JSON 字符串输出。</p>
        </CardBody>
      </Card>
    </div>
  );
}
