import { useMemo, useState } from "react";
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
  Code,
  Input,
  Kbd,
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

  const ruleSummary = useMemo(() => {
    const parsed = parseIndexOrder(indexOrder);
    const indexes = parsed.error ? indexOrder || "?" : parsed.indexes.join("、");
    return `按“${inputDelimiter || "单个字符"}”拆分 → 取第 ${indexes} 列 → 用“${
      outputDelimiter || "空字符"
    }”连接`;
  }, [indexOrder, inputDelimiter, outputDelimiter]);

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
      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2">
        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex items-center justify-between gap-3">
            <div>
              <p className="font-semibold text-foreground">文本与分隔设置</p>
              <p className="text-xs text-default-500">拆分、重排并重新连接每行文本</p>
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
              label="原始文本"
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
              minRows={10}
              variant="bordered"
              classNames={{ input: "font-mono text-sm" }}
            />

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Input
                label="原始分隔符"
                labelPlacement="outside"
                value={inputDelimiter}
                onValueChange={setInputDelimiter}
                onKeyDown={handleInputEnter}
                description="留空时按单个字符拆分"
                variant="bordered"
              />
              <Input
                label="输出分隔符"
                labelPlacement="outside"
                value={outputDelimiter}
                onValueChange={setOutputDelimiter}
                onKeyDown={handleInputEnter}
                description="留空时直接拼接结果"
                variant="bordered"
              />
            </div>

            <Input
              label="输出索引顺序"
              labelPlacement="outside"
              value={indexOrder}
              onValueChange={setIndexOrder}
              onKeyDown={handleInputEnter}
              description="例如 0,2,1；多位数索引请使用逗号"
              variant="bordered"
              startContent={<span aria-hidden>↕️</span>}
            />

            <Code color="primary" className="whitespace-normal text-xs leading-5">
              {ruleSummary}
            </Code>
          </CardBody>
          <CardFooter className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <Button color="primary" onPress={process}>
              处理文本
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
              <p className="font-semibold text-foreground">处理结果</p>
              <p className="text-xs text-default-500">
                {result ? `索引顺序：${result.indexes.join(",")}` : "处理后的文本会显示在这里"}
              </p>
            </div>
            {result && (
              <div className="flex flex-wrap gap-1.5">
                <Chip size="sm" variant="flat">{result.lineCount} 行</Chip>
                <Chip size="sm" color="primary" variant="flat">
                  {result.minColumns === result.maxColumns
                    ? `${result.minColumns} 列`
                    : `${result.minColumns}-${result.maxColumns} 列`}
                </Chip>
                <Chip
                  size="sm"
                  color={result.invalidLines.length ? "danger" : "success"}
                  variant="flat"
                >
                  无效 {result.invalidLines.length}
                </Chip>
              </div>
            )}
          </CardHeader>
          <CardBody className="gap-4">
            <Textarea
              aria-label="处理结果"
              placeholder="处理后的结果将显示在这里…"
              value={result?.output ?? ""}
              isReadOnly
              minRows={14}
              variant="bordered"
              classNames={{ input: "font-mono text-sm" }}
            />

            {result && result.invalidLines.length > 0 && (
              <Accordion variant="bordered" isCompact>
                <AccordionItem
                  key="invalid-lines"
                  aria-label="无效索引行"
                  title={`查看 ${result.invalidLines.length} 个无效索引行`}
                >
                  <p className="pb-2 text-sm text-danger">
                    以下行的列数不足，越界索引已输出为空值：
                    {result.invalidLines.join("、")}
                  </p>
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
          subtitle="了解索引规则、分隔符和快捷键"
        >
          <div className="flex flex-col gap-4 pb-2 text-sm text-default-600">
            <Alert color="primary" variant="flat" title="快速操作">
              在原始文本框内按 <Kbd keys={["command"]}>Enter</Kbd> 或
              <Kbd keys={["ctrl"]}>Enter</Kbd> 可立即处理。
            </Alert>
            <pre className="overflow-x-auto rounded-medium bg-default-100 p-3 font-mono text-xs text-foreground">
              {`a----b----c\n1----2----3----4`}
            </pre>
            <p>
              原始分隔符填“----”，输出分隔符填“,”，索引顺序填
              “0,2,1”，结果为“a,c,b”和“1,3,2”。
            </p>
          </div>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
