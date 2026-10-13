import { useMemo, useState } from "react";
import {
  Accordion,
  AccordionItem,
  addToast,
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
  indexPresets,
  parseIndexOrder,
  processDelimitedText,
} from "./process";

const example = `姓名----年龄----性别----职业
张三----25----男----工程师
李四----30----女----设计师
王五----35----男----产品经理`;

const DELIMITER_PRESETS: { label: string; value: string }[] = [
  { label: "----", value: "----" },
  { label: "逗号", value: "," },
  { label: "Tab", value: "\t" },
  { label: "分号", value: ";" },
  { label: "竖线", value: "|" },
  { label: "空格", value: " " },
  { label: "斜杠", value: "/" },
  { label: "减号", value: "-" },
  { label: "等号", value: "=" },
];

function delimiterName(value: string): string {
  return value === ""
    ? "单个字符"
    : value === "\t"
      ? "Tab"
      : value === " "
        ? "空格"
        : value;
}

function DelimiterField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      <Input
        label={`${label}（${delimiterName(value)}）`}
        labelPlacement="outside"
        value={value}
        onValueChange={onChange}
        description="可直接输入任意字符，或点选常用值"
        variant="bordered"
      />
      <div className="flex flex-wrap gap-1.5">
        {DELIMITER_PRESETS.map((preset) => (
          <Button
            key={preset.label}
            size="sm"
            radius="full"
            variant={value === preset.value ? "solid" : "bordered"}
            color={value === preset.value ? "primary" : "default"}
            onPress={() => onChange(preset.value)}
          >
            {preset.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

export default function TextDelimiter() {
  const [source, setSource] = useState("");
  const [inputDelimiter, setInputDelimiter] = useState("----");
  const [outputDelimiter, setOutputDelimiter] = useState(",");
  const [indexOrder, setIndexOrder] = useState("0");

  const parsedIndexes = useMemo(() => parseIndexOrder(indexOrder), [indexOrder]);
  const columnStats = useMemo(
    () => (source.trim() ? processDelimitedText(source, inputDelimiter, "", []) : null),
    [source, inputDelimiter],
  );
  const result = useMemo(() => {
    if (!source.trim() || parsedIndexes.error) return null;
    return processDelimitedText(
      source,
      inputDelimiter,
      outputDelimiter,
      parsedIndexes.indexes,
    );
  }, [source, inputDelimiter, outputDelimiter, parsedIndexes]);

  const status = useMemo(() => {
    if (parsedIndexes.error) return { text: parsedIndexes.error, error: true };
    if (!source.trim()) return { text: "等待输入", error: false };
    if (result && result.invalidLines.length > 0) {
      return { text: `${result.invalidLines.length} 行索引越界`, error: true };
    }
    return { text: "实时处理完成", error: false };
  }, [parsedIndexes, source, result]);

  const ruleSummary = useMemo(() => {
    const indexes = parsedIndexes.error
      ? indexOrder || "?"
      : parsedIndexes.indexes.join("、");
    return `按“${delimiterName(inputDelimiter)}”拆分 → 取第 ${indexes} 列 → 用“${delimiterName(
      outputDelimiter,
    )}”连接`;
  }, [parsedIndexes, indexOrder, inputDelimiter, outputDelimiter]);

  async function copyResult() {
    if (!result) return;
    const copied = await copyText(result.output);
    addToast({
      title: copied ? "结果已复制到剪贴板" : "复制失败，请手动选择文本",
      color: copied ? "success" : "danger",
    });
  }

  function clear() {
    setSource("");
    setInputDelimiter("----");
    setOutputDelimiter(",");
    setIndexOrder("0");
    addToast({ title: "已清空并恢复默认设置" });
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex items-center justify-between gap-3">
            <div>
              <p className="font-semibold text-foreground">文本与分隔设置</p>
              <p className="text-xs text-default-500">拆分、重排并重新连接每行文本</p>
            </div>
            <Chip
              color={status.error ? "danger" : result ? "success" : "default"}
              variant="flat"
              size="sm"
              aria-live="polite"
            >
              {status.text}
            </Chip>
          </CardHeader>
          <CardBody className="gap-5">
            <Textarea
              label="原始文本"
              labelPlacement="outside"
              placeholder={example}
              value={source}
              onValueChange={setSource}
              minRows={12}
              maxRows={28}
              variant="bordered"
              classNames={{ input: "font-mono text-sm" }}
            />

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <DelimiterField label="原始分隔符" value={inputDelimiter} onChange={setInputDelimiter} />
              <DelimiterField label="输出分隔符" value={outputDelimiter} onChange={setOutputDelimiter} />
            </div>

            <div className="flex flex-col gap-2">
              <Input
                label="输出索引顺序"
                labelPlacement="outside"
                value={indexOrder}
                onValueChange={setIndexOrder}
                description="例如 0,2,1；多位数索引请使用逗号，点下面的按钮可快速填入"
                variant="bordered"
                startContent={<span aria-hidden>↕️</span>}
              />
              <div className="flex flex-wrap gap-1.5">
                {indexPresets(columnStats?.maxColumns ?? 0).map((preset) => (
                  <Button
                    key={`${preset.label}:${preset.value}`}
                    size="sm"
                    radius="full"
                    variant={indexOrder === preset.value ? "solid" : "bordered"}
                    color={indexOrder === preset.value ? "primary" : "default"}
                    onPress={() => setIndexOrder(preset.value)}
                  >
                    {preset.label}
                  </Button>
                ))}
              </div>
            </div>

            <Code color="primary" className="whitespace-normal text-xs leading-5">
              {ruleSummary}
            </Code>
          </CardBody>
          <CardFooter className="grid grid-cols-2 gap-2">
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
                {result
                  ? `索引顺序：${result.indexes.join(",")}`
                  : "输入文本后会立即显示在这里"}
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
              minRows={22}
              maxRows={40}
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
                    第 {result.invalidLines.join("、")} 行的列数少于索引要求，对应列输出为空。
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
        >
          <div className="flex flex-col gap-4 pb-2 text-sm text-default-600">
            <Alert color="primary" variant="flat" title="实时处理">
              修改文本、分隔符或索引顺序后结果会立即更新，无需点击按钮。快捷按钮可直接填入 <Kbd>Tab</Kbd>、空格、逗号等常用分隔符。
            </Alert>
            <pre className="overflow-x-auto rounded-medium bg-default-100 p-3 font-mono text-xs text-foreground">
              {`输入：a----b----c
       1----2----3
索引顺序：1,0
输出：b,a
       2,1`}
            </pre>
          </div>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
