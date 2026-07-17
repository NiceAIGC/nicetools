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
import {
  parseIndexOrder,
  processDelimitedText,
  type DelimitedTextResult,
} from "./process";

const example = `姓名----年龄----性别----职业
张三----25----男----工程师
李四----30----女----设计师
王五----35----男----产品经理`;

export default function TextDelimiter() {
  const [source, setSource] = useState("");
  const [inputDelimiter, setInputDelimiter] = useState("----");
  const [outputDelimiter, setOutputDelimiter] = useState(",");
  const [indexOrder, setIndexOrder] = useState("0");
  const [result, setResult] = useState<DelimitedTextResult | null>(null);
  const [status, setStatus] = useState("等待输入");
  const [isError, setIsError] = useState(false);

  function process() {
    if (!source.trim()) {
      setStatus("请输入原始文本");
      setIsError(true);
      return;
    }

    const parsed = parseIndexOrder(indexOrder);
    if (parsed.error) {
      setStatus(parsed.error);
      setIsError(true);
      return;
    }

    const next = processDelimitedText(
      source,
      inputDelimiter,
      outputDelimiter,
      parsed.indexes,
    );
    setResult(next);
    if (next.invalidLines.length > 0) {
      setStatus(`处理完成，${next.invalidLines.length} 行存在无效索引`);
      setIsError(true);
    } else {
      setStatus("文本处理成功");
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
    setInputDelimiter("----");
    setOutputDelimiter(",");
    setIndexOrder("0");
    setResult(null);
    setStatus("已清空并恢复默认设置");
    setIsError(false);
  }

  function handleInputEnter(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      process();
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex justify-end" aria-live="polite">
        <Chip color={isError ? "danger" : "default"} variant="flat" size="sm">
          {status}
        </Chip>
      </div>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="text-base font-semibold">文本与分隔设置</CardHeader>
        <CardBody className="gap-5">
          <Textarea
            label="原始文本（多行）"
            labelPlacement="outside"
            placeholder={example}
            value={source}
            onValueChange={setSource}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
                event.preventDefault();
                process();
              }
            }}
            minRows={8}
            variant="bordered"
            classNames={{ input: "font-mono text-sm" }}
          />

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Input
              label="原始文本分隔符"
              labelPlacement="outside"
              value={inputDelimiter}
              onValueChange={setInputDelimiter}
              onKeyDown={handleInputEnter}
              description="默认为 ----；留空时按单个字符拆分"
              variant="bordered"
            />
            <Input
              label="输出结果分隔符"
              labelPlacement="outside"
              value={outputDelimiter}
              onValueChange={setOutputDelimiter}
              onKeyDown={handleInputEnter}
              description="默认为逗号；可留空以直接拼接结果"
              variant="bordered"
            />
          </div>

          <Input
            label="输出索引顺序"
            labelPlacement="outside"
            value={indexOrder}
            onValueChange={setIndexOrder}
            onKeyDown={handleInputEnter}
            description='例如 0,2,1；也可写成 021，多位数索引请用逗号分隔'
            variant="bordered"
            startContent={<span aria-hidden>↕️</span>}
          />

          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <Button color="primary" onPress={process}>
              处理文本
            </Button>
            <Button variant="bordered" onPress={copyResult} isDisabled={!result}>
              复制结果
            </Button>
            <Button variant="light" onPress={clear}>
              清空
            </Button>
          </div>
          <p className="text-xs text-default-400">
            在原始文本框内按 Ctrl/Command + Enter 可快速处理。
          </p>
        </CardBody>
      </Card>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardHeader className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-base font-semibold">处理结果</span>
          {result && (
            <span className="text-xs text-default-400">
              索引顺序：{result.indexes.join(",")}
            </span>
          )}
        </CardHeader>
        <CardBody className="gap-4">
          <Textarea
            aria-label="处理结果"
            placeholder="处理后的结果将显示在这里…"
            value={result?.output ?? ""}
            isReadOnly
            minRows={8}
            variant="bordered"
            classNames={{ input: "font-mono text-sm" }}
          />

          {result && (
            <div className="flex flex-wrap gap-2">
              <Chip variant="flat">共处理 {result.lineCount} 行</Chip>
              <Chip color="primary" variant="flat">
                每行列数：
                {result.minColumns === result.maxColumns
                  ? result.minColumns
                  : `${result.minColumns} - ${result.maxColumns}`}
              </Chip>
              <Chip
                color={result.invalidLines.length ? "danger" : "success"}
                variant="flat"
              >
                无效索引 {result.invalidLines.length} 行
              </Chip>
            </div>
          )}

          {result && result.invalidLines.length > 0 && (
            <div className="rounded-medium border border-danger-200 bg-danger-50 px-3 py-2 text-sm text-danger-700">
              以下行的列数不足，越界索引已输出为空值：
              {result.invalidLines.join("、")}
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
            {`a----b----c\n1----2----3----4`}
          </pre>
          <p>
            原始分隔符填 “----”，输出分隔符填 “,”，索引顺序填
            “0,2,1”，结果为 “a,c,b” 和 “1,3,2”。
          </p>
        </CardBody>
      </Card>
    </div>
  );
}
