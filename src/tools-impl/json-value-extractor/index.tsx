import { useState } from "react";
import {
  Accordion,
  AccordionItem,
  Alert,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  Chip,
  Input,
  Kbd,
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
      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2">
        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex items-center justify-between gap-3">
            <div>
              <p className="font-semibold text-foreground">提取设置</p>
              <p className="text-xs text-default-500">每行输入一个 JSON 对象</p>
            </div>
            <Chip
              color={isError ? "danger" : result ? "success" : "default"}
              variant="flat"
              size="sm"
              aria-live="polite"
            >
              {status}
            </Chip>
          </CardHeader>
          <CardBody className="gap-5">
            <Textarea
              label="JSON 文本"
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
              minRows={12}
              variant="bordered"
              classNames={{ input: "font-mono text-sm" }}
            />

            <Input
              label="要提取的键名"
              labelPlacement="outside"
              placeholder="例如：name"
              description="精确匹配 JSON 对象的顶层字段"
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
          </CardBody>
          <CardFooter className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <Button color="primary" onPress={extract}>
              提取值
            </Button>
            <Button variant="bordered" onPress={copyResult} isDisabled={!result}>
              复制结果
            </Button>
            <Button variant="light" onPress={clear}>
              清空
            </Button>
          </CardFooter>
        </Card>

        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="font-semibold text-foreground">提取结果</p>
              <p className="text-xs text-default-500">
                {result ? `提取键：“${keyName.trim()}”` : "处理后的值会按行输出"}
              </p>
            </div>
            {result && (
              <div className="flex flex-wrap gap-1.5">
                <Chip size="sm" variant="flat">共 {result.total} 行</Chip>
                <Chip size="sm" color="success" variant="flat">
                  成功 {result.success}
                </Chip>
                <Chip
                  size="sm"
                  color={result.errors.length ? "danger" : "default"}
                  variant="flat"
                >
                  错误 {result.errors.length}
                </Chip>
              </div>
            )}
          </CardHeader>
          <CardBody className="gap-4">
            <Textarea
              aria-label="提取结果"
              placeholder="提取的结果将显示在这里…"
              value={result?.output ?? ""}
              isReadOnly
              minRows={12}
              variant="bordered"
              classNames={{ input: "font-mono text-sm" }}
            />

            {result && result.errors.length > 0 && (
              <Accordion variant="bordered" isCompact>
                <AccordionItem
                  key="errors"
                  aria-label="错误详情"
                  title={`查看 ${result.errors.length} 条错误详情`}
                >
                  <ol className="max-h-48 list-inside list-decimal space-y-1 overflow-y-auto pb-2 text-xs text-danger">
                    {result.errors.map((error) => (
                      <li key={`${error.line}-${error.message}`}>
                        行 {error.line}：{error.message}
                      </li>
                    ))}
                  </ol>
                </AccordionItem>
              </Accordion>
            )}
          </CardBody>
        </Card>
      </div>

      <Accordion variant="bordered">
        <AccordionItem
          key="guide"
          aria-label="使用说明"
          title="使用说明与示例"
          subtitle="了解输入格式、输出规则和快捷键"
        >
          <div className="flex flex-col gap-4 pb-2 text-sm text-default-600">
            <Alert color="primary" variant="flat" title="快速操作">
              在 JSON 文本框内按 <Kbd keys={["command"]}>Enter</Kbd> 或
              <Kbd keys={["ctrl"]}>Enter</Kbd> 可立即提取。
            </Alert>
            <pre className="overflow-x-auto rounded-medium bg-default-100 p-3 font-mono text-xs text-foreground">
              {example}
            </pre>
            <p>键名填写“name”，输出为张三、李四、王五，每个值单独一行。</p>
            <p>如果值是对象或数组，工具会将它压缩为 JSON 字符串输出。</p>
          </div>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
