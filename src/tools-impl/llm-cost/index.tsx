import { useEffect, useMemo, useRef, useState } from "react";
import {
  Accordion,
  AccordionItem,
  Alert,
  Card,
  CardHeader,
  CardBody,
  CardFooter,
  Input,
  Select,
  SelectItem,
  Button,
  Chip,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@heroui/react";
import { copyText } from "../../utils/copyText";
import {
  type CalcState,
  type Currency,
  type DurationUnit,
  defaults,
  numericFields,
  calculate,
  loadState,
  hasSavedState,
  getCurrencyMeta,
  formatMoney,
  formatNumber,
  formatPercent,
  formatDiscountFromRate,
  formatUnitPrice,
  resultText,
  nonNegative,
  percent,
  paymentRateValue,
  STORAGE_KEY,
  LEGACY_STORAGE_KEY,
} from "./calc";

const durationUnitItems: { key: DurationUnit; label: string }[] = [
  { key: "minute", label: "分钟" },
  { key: "hour", label: "小时" },
  { key: "day", label: "天" },
  { key: "month", label: "月" },
];

const paymentPresets = [
  { value: 100, label: "10折" },
  { value: 80, label: "8折" },
  { value: 60, label: "6折" },
  { value: 45, label: "4.5折" },
  { value: 30, label: "3折" },
  { value: 10, label: "1折" },
];

export default function LlmCost() {
  const [state, setState] = useState<CalcState>(() => loadState());
  const [status, setStatus] = useState(() =>
    hasSavedState() ? "已恢复上次配置" : "已加载默认配置",
  );
  const saveTimer = useRef<number | null>(null);
  const firstRun = useRef(true);

  const result = useMemo(() => calculate(state), [state]);
  const meta = getCurrencyMeta(state.currency);
  const money = (v: number) => formatMoney(v, state.currency);
  const unitPrice = (v: unknown) => formatUnitPrice(v, state.currency);

  // 防抖持久化到 localStorage（首次挂载不写，避免覆盖“已恢复配置”状态提示）。
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    if (saveTimer.current) window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(() => {
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
        setStatus("已自动保存");
      } catch {
        setStatus("保存失败");
      }
    }, 200);
    return () => {
      if (saveTimer.current) window.clearTimeout(saveTimer.current);
    };
  }, [state]);

  // 文本/枚举字段：直接写入。
  function setText<K extends keyof CalcState>(key: K, value: CalcState[K]) {
    setState((s) => ({ ...s, [key]: value }));
  }

  // 保留输入中的原始文本，避免 0.5 在输入到 "0." 时被立即转回 0。
  function setNumber(key: keyof CalcState, raw: string) {
    setState((s) => ({ ...s, [key]: raw }));
  }

  // 失焦时归一化数值（对齐原实现的 change 行为）。
  function normalizeOnBlur(key: keyof CalcState) {
    if (!numericFields.includes(key)) return;
    setState((s) => {
      let value = nonNegative(s[key]);
      if (key === "cacheReadRate" || key === "cacheCreateRate")
        value = percent(s[key]);
      if (key === "paymentRate") value = paymentRateValue(s[key]);
      if (key === "monthDays") value = Math.max(1, value || (defaults.monthDays as number));
      return { ...s, [key]: value };
    });
  }

  async function copyResult() {
    const copied = await copyText(resultText(state));
    setStatus(copied ? "结果已复制" : "复制失败");
  }

  function resetDefault() {
    setState({ ...defaults });
    setStatus("已重置默认");
  }

  function clearStorage() {
    try {
      window.localStorage.removeItem(STORAGE_KEY);
      window.localStorage.removeItem(LEGACY_STORAGE_KEY);
      firstRun.current = true; // 阻止本次清空触发的 effect 立即回写
      setState({ ...defaults });
      setStatus("缓存已清空");
    } catch {
      setStatus("清空失败");
    }
  }

  const val = (v: number | string) => String(v);

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2">
        {/* 左列：输入 */}
        <div className="flex min-w-0 flex-col gap-4">
          {/* 模型价格 */}
          <Card shadow="sm" className="min-w-0 border border-default-200">
            <CardHeader className="flex items-center justify-between gap-3">
              <div>
                <p className="font-semibold text-foreground">模型价格</p>
                <p className="text-xs text-default-500">设置每百万 token 的计费单价</p>
              </div>
              <Chip variant="flat" color="default" size="sm" aria-live="polite">
                {status}
              </Chip>
            </CardHeader>
            <CardBody className="grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-2">
              <Input
                label="模型名称"
                labelPlacement="outside"
                placeholder="模型名称"
                value={state.modelName}
                onValueChange={(v) => setText("modelName", v)}
                variant="bordered"
                autoComplete="off"
              />
              <Select
                label="币种"
                labelPlacement="outside"
                selectedKeys={[state.currency]}
                onSelectionChange={(keys) =>
                  setText(
                    "currency",
                    (Array.from(keys)[0] as Currency) ?? "CNY",
                  )
                }
                variant="bordered"
                disallowEmptySelection
              >
                <SelectItem key="CNY">人民币</SelectItem>
                <SelectItem key="USD">美金</SelectItem>
              </Select>
              <PriceInput
                label="输入价格"
                value={val(state.inputPrice)}
                onChange={(v) => setNumber("inputPrice", v)}
                onBlur={() => normalizeOnBlur("inputPrice")}
                unit={meta.unit}
              />
              <PriceInput
                label="输出价格"
                value={val(state.outputPrice)}
                onChange={(v) => setNumber("outputPrice", v)}
                onBlur={() => normalizeOnBlur("outputPrice")}
                unit={meta.unit}
              />
              <PriceInput
                label="缓存创建价格"
                value={val(state.cacheCreatePrice)}
                onChange={(v) => setNumber("cacheCreatePrice", v)}
                onBlur={() => normalizeOnBlur("cacheCreatePrice")}
                unit={meta.unit}
              />
              <PriceInput
                label="缓存读取价格"
                value={val(state.cacheReadPrice)}
                onChange={(v) => setNumber("cacheReadPrice", v)}
                onBlur={() => normalizeOnBlur("cacheReadPrice")}
                unit={meta.unit}
              />
            </CardBody>
          </Card>

          {/* 用量设置 */}
          <Card shadow="sm" className="min-w-0 border border-default-200">
            <CardHeader>
              <div>
                <p className="font-semibold text-foreground">用量设置</p>
                <p className="text-xs text-default-500">设置吞吐量、时长、缓存命中率和折扣</p>
              </div>
            </CardHeader>
            <CardBody className="grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-2">
              <NumInput
                label="总 TPM"
                value={val(state.tpm)}
                onChange={(v) => setNumber("tpm", v)}
                onBlur={() => normalizeOnBlur("tpm")}
                endText="万 token / 分钟"
                step="0.01"
              />
              <NumInput
                label="运行时长"
                value={val(state.durationValue)}
                onChange={(v) => setNumber("durationValue", v)}
                onBlur={() => normalizeOnBlur("durationValue")}
                step="0.01"
              />
              <Select
                label="时长单位"
                labelPlacement="outside"
                selectedKeys={[state.durationUnit]}
                onSelectionChange={(keys) =>
                  setText(
                    "durationUnit",
                    (Array.from(keys)[0] as DurationUnit) ?? "minute",
                  )
                }
                variant="bordered"
                disallowEmptySelection
              >
                {durationUnitItems.map((it) => (
                  <SelectItem key={it.key}>{it.label}</SelectItem>
                ))}
              </Select>
              <NumInput
                label="每月天数"
                value={val(state.monthDays)}
                onChange={(v) => setNumber("monthDays", v)}
                onBlur={() => normalizeOnBlur("monthDays")}
                endText="天"
                step="1"
              />

              {/* 输入输出比例 */}
              <div className="flex flex-col gap-1.5 sm:col-span-2">
                <span className="text-sm text-foreground">输入输出比例</span>
                <div className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-[1fr_auto_1fr] sm:items-center">
                  <Input
                    aria-label="输入比例"
                    type="number"
                    min="0"
                    step="0.01"
                    value={val(state.inputRatio)}
                    onValueChange={(v) => setNumber("inputRatio", v)}
                    onBlur={() => normalizeOnBlur("inputRatio")}
                    variant="bordered"
                    className="min-w-0"
                  />
                  <span className="hidden text-center font-semibold text-default-400 sm:block">
                    :
                  </span>
                  <Input
                    aria-label="输出比例"
                    type="number"
                    min="0"
                    step="0.01"
                    value={val(state.outputRatio)}
                    onValueChange={(v) => setNumber("outputRatio", v)}
                    onBlur={() => normalizeOnBlur("outputRatio")}
                    variant="bordered"
                    className="min-w-0"
                  />
                </div>
              </div>

              <NumInput
                label="缓存读取率"
                value={val(state.cacheReadRate)}
                onChange={(v) => setNumber("cacheReadRate", v)}
                onBlur={() => normalizeOnBlur("cacheReadRate")}
                endText="%"
                step="0.01"
              />
              <NumInput
                label="缓存创建率"
                value={val(state.cacheCreateRate)}
                onChange={(v) => setNumber("cacheCreateRate", v)}
                onBlur={() => normalizeOnBlur("cacheCreateRate")}
                endText="%"
                step="0.01"
              />

              <Input
                label="普通输入率"
                labelPlacement="outside"
                value={formatPercent(result.normalInputRate)}
                isReadOnly
                variant="flat"
                description="由 100% - 缓存读取率 - 缓存创建率自动计算"
              />

              <div className="flex flex-col gap-1.5">
                <span className="text-sm text-foreground">实际支付比例</span>
                <Input
                  aria-label="实际支付比例"
                  type="number"
                  min="0"
                  max="100"
                  step="0.01"
                  value={val(state.paymentRate)}
                  onValueChange={(v) => setNumber("paymentRate", v)}
                  onBlur={() => normalizeOnBlur("paymentRate")}
                  variant="bordered"
                  endContent={<span className="text-default-400">%</span>}
                  list="paymentRateOptions"
                />
                <datalist id="paymentRateOptions">
                  {paymentPresets.map((p) => (
                    <option key={p.value} value={p.value} label={p.label} />
                  ))}
                </datalist>
                <div className="flex flex-wrap gap-1.5">
                  {paymentPresets.map((p) => (
                    <Chip
                      key={p.value}
                      size="sm"
                      variant={
                        paymentRateValue(state.paymentRate) === p.value
                          ? "solid"
                          : "flat"
                      }
                      color={
                        paymentRateValue(state.paymentRate) === p.value
                          ? "primary"
                          : "default"
                      }
                      className="cursor-pointer"
                      onClick={() => setText("paymentRate", p.value)}
                    >
                      {p.label}
                    </Chip>
                  ))}
                </div>
              </div>

              {result.rateOverflow && (
                <Alert
                  color="warning"
                  variant="flat"
                  title="缓存比例超过 100%"
                  description="普通输入率已按 0% 计算，缓存读取与创建 token 会按比例缩放。"
                  className="sm:col-span-2"
                />
              )}
            </CardBody>
          </Card>
        </div>

        {/* 右列：结果 */}
        <Card shadow="sm" className="min-w-0 border border-default-200 lg:sticky lg:top-20 lg:self-start">
          <CardHeader>
            <div>
              <p className="font-semibold text-foreground">计算结果</p>
              <p className="text-xs text-default-500">根据当前设置实时更新</p>
            </div>
          </CardHeader>
          <CardBody className="min-w-0 gap-4">
            {/* 指标卡 */}
            <div className="grid min-w-0 grid-cols-2 gap-3">
              <Metric
                primary
                label="折后实际消耗"
                value={money(result.actualTotalCost)}
              />
              <Metric label="每月实际" value={money(result.actualMonthlyCost)} />
              <Metric label="每日实际" value={money(result.actualDailyCost)} />
              <Metric label="每小时实际" value={money(result.actualHourlyCost)} />
              <Metric label="每分钟实际" value={money(result.actualMinuteCost)} />
            </div>

            {/* 关键指标行 */}
            <div className="min-w-0 overflow-hidden rounded-medium border border-default-200">
              <InfoRow label="折前总费用" value={money(result.totalCost)} />
              <InfoRow
                label="实际 TPM"
                value={`${formatNumber(result.tpm, 0)} token / 分钟`}
              />
              <InfoRow
                label="实际支付比例"
                value={`${formatPercent(result.paymentRatio)}（${formatDiscountFromRate(
                  state.paymentRate,
                )}）`}
              />
              <InfoRow
                label="当前时长总 token"
                value={formatNumber(result.totalTokens, 0)}
              />
              <InfoRow
                label="输入 token"
                value={formatNumber(result.inputTokens, 0)}
              />
              <InfoRow
                label="输出 token"
                value={formatNumber(result.outputTokens, 0)}
                last
              />
            </div>

            {/* 明细表 */}
            <Table
              aria-label="费用明细"
              removeWrapper
              classNames={{
                base: "min-w-0 overflow-x-auto rounded-medium border border-default-200",
                table: "min-w-[560px] sm:min-w-full",
              }}
            >
              <TableHeader>
                <TableColumn>项目</TableColumn>
                <TableColumn className="text-right">token</TableColumn>
                <TableColumn className="text-right">单价</TableColumn>
                <TableColumn className="text-right">折前费用</TableColumn>
              </TableHeader>
              <TableBody>
                <TableRow key="normal">
                  <TableCell>普通输入</TableCell>
                  <TableCell className="text-right">
                    {formatNumber(result.normalInputTokens, 0)}
                  </TableCell>
                  <TableCell className="text-right">
                    {unitPrice(state.inputPrice)}
                  </TableCell>
                  <TableCell className="text-right font-semibold text-success-600">
                    {money(result.normalInputCost)}
                  </TableCell>
                </TableRow>
                <TableRow key="cacheCreate">
                  <TableCell>缓存创建</TableCell>
                  <TableCell className="text-right">
                    {formatNumber(result.cacheCreateTokens, 0)}
                  </TableCell>
                  <TableCell className="text-right">
                    {unitPrice(state.cacheCreatePrice)}
                  </TableCell>
                  <TableCell className="text-right font-semibold text-success-600">
                    {money(result.cacheCreateCost)}
                  </TableCell>
                </TableRow>
                <TableRow key="cacheRead">
                  <TableCell>缓存读取</TableCell>
                  <TableCell className="text-right">
                    {formatNumber(result.cacheReadTokens, 0)}
                  </TableCell>
                  <TableCell className="text-right">
                    {unitPrice(state.cacheReadPrice)}
                  </TableCell>
                  <TableCell className="text-right font-semibold text-success-600">
                    {money(result.cacheReadCost)}
                  </TableCell>
                </TableRow>
                <TableRow key="output">
                  <TableCell>输出</TableCell>
                  <TableCell className="text-right">
                    {formatNumber(result.outputTokens, 0)}
                  </TableCell>
                  <TableCell className="text-right">
                    {unitPrice(state.outputPrice)}
                  </TableCell>
                  <TableCell className="text-right font-semibold text-success-600">
                    {money(result.outputCost)}
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardBody>
          <CardFooter className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <Button color="primary" onPress={copyResult}>
              复制结果
            </Button>
            <Button variant="bordered" onPress={resetDefault}>
              重置默认
            </Button>
            <Button variant="light" color="danger" onPress={clearStorage}>
              清空缓存
            </Button>
          </CardFooter>
        </Card>
      </div>

      <Accordion variant="bordered">
        <AccordionItem
          key="guide"
          aria-label="计算口径与基础说明"
          title="计算口径与基础说明"
          subtitle="了解 TPM、输入输出比例、缓存和支付折扣的计算方式"
        >
          <div className="flex flex-col gap-4 pb-2 text-sm leading-6 text-default-600">
            <Alert
              color="primary"
              variant="flat"
              title="估算结果说明"
              description="结果由你填写的价格、用量和比例计算得出，不代表模型厂商后台的实时账单。"
            />
            <ul className="list-disc space-y-2 pl-5">
            <li>
              <span className="font-semibold text-foreground">TPM</span>
              ：Token Per Minute，每分钟处理的 token 数。本工具里的“总
              TPM”按输入 token + 输出 token 的合计吞吐量理解，输入单位是“万
              token / 分钟”。例如填 500，实际按 5,000,000 token /
              分钟参与计算。
            </li>
            <li>
              <span className="font-semibold text-foreground">TPM 如何拆分</span>
              ：行业里做成本估算时，通常先按总吞吐量估算，再用输入输出比例拆成输入和输出。本工具也是这个口径：
              输入 token = 总 token x 输入比例 /（输入比例 + 输出比例），输出
              token = 总 token x 输出比例 /（输入比例 + 输出比例）。例如总
              TPM 为 500 万、输入输出比例为 8:1，则每分钟约 444.44 万输入
              token、55.56 万输出 token。
            </li>
            <li>
              <span className="font-semibold text-foreground">token</span>
              ：模型计费的基础单位，输入和输出都会消耗 token。不同模型的中文、英文、代码换算比例不同，所以这里按你输入的总
              TPM 和输入输出比例估算。
            </li>
            <li>
              <span className="font-semibold text-foreground">输入输出比例</span>
              ：用于把总 token 拆成输入 token 和输出 token。比如 8:1
              表示约 8 份输入、1 份输出。
            </li>
            <li>
              <span className="font-semibold text-foreground">缓存</span>
              ：通常指 Prompt Cache。重复或相似的输入内容被模型服务复用时，可能按更低的缓存读取价格计费；首次写入缓存时，可能按缓存创建价格计费。
            </li>
            <li>
              <span className="font-semibold text-foreground">缓存读取率 / 缓存创建率</span>
              ：这是估算参数，用来拆分输入 token 中有多少按缓存读取、缓存创建或普通输入计费。普通输入率由工具按
              100% - 缓存读取率 - 缓存创建率 自动计算。缓存只作用在输入
              token 上，输出 token 不参与缓存拆分。
            </li>
            <li>
              <span className="font-semibold text-foreground">和厂商限流口径的区别</span>
              ：不同平台的限流指标可能分开写成输入 TPM、输出 TPM、缓存读取
              TPM 或总 TPM。本工具为了快速估算成本，使用“总 TPM + 输入输出比例”的统一口径；如果你的平台已经给了独立的输入/输出
              TPM，可以先折算成合计 TPM，再按相同输入输出比例填入。
            </li>
            <li>
              <span className="font-semibold text-foreground">实际支付比例</span>
              ：用于模拟商务折扣、代金券或渠道折扣。45% 等于 4.5
              折，折后实际消耗 = 折前费用 x 实际支付比例。
            </li>
            </ul>
          </div>
        </AccordionItem>
      </Accordion>
    </div>
  );
}

// —— 局部小组件 ——

function PriceInput(props: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  onBlur: () => void;
  unit: string;
}) {
  return (
    <Input
      label={props.label}
      labelPlacement="outside"
      type="number"
      min="0"
      step="0.0001"
      inputMode="decimal"
      value={props.value}
      onValueChange={props.onChange}
      onBlur={props.onBlur}
      variant="bordered"
      endContent={
        <span className="whitespace-nowrap text-xs text-default-400">
          {props.unit}
        </span>
      }
    />
  );
}

function NumInput(props: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  onBlur: () => void;
  endText?: string;
  step?: string;
}) {
  return (
    <Input
      label={props.label}
      labelPlacement="outside"
      type="number"
      min="0"
      step={props.step ?? "0.01"}
      inputMode="decimal"
      value={props.value}
      onValueChange={props.onChange}
      onBlur={props.onBlur}
      variant="bordered"
      endContent={
        props.endText ? (
          <span className="whitespace-nowrap text-xs text-default-400">
            {props.endText}
          </span>
        ) : undefined
      }
    />
  );
}

function Metric({
  label,
  value,
  primary,
}: {
  label: string;
  value: string;
  primary?: boolean;
}) {
  return (
    <Card
      shadow="none"
      className={primary ? "col-span-2 border border-primary-200" : "border border-default-200"}
    >
      <CardBody className={primary ? "gap-1 bg-primary-50 p-4" : "gap-1 p-4"}>
        <span className="text-xs font-medium text-default-500">{label}</span>
        <span
          className={
            primary
              ? "break-words text-3xl font-extrabold text-primary sm:text-4xl"
              : "break-words text-xl font-bold text-foreground"
          }
        >
          {value}
        </span>
      </CardBody>
    </Card>
  );
}

function InfoRow({
  label,
  value,
  last,
}: {
  label: string;
  value: string;
  last?: boolean;
}) {
  return (
    <div
      className={
        "flex items-center justify-between gap-3 px-3 py-2.5 text-sm" +
        (last ? "" : " border-b border-default-200")
      }
    >
      <span className="text-default-500">{label}</span>
      <span className="min-w-0 break-words text-right font-semibold text-foreground">
        {value}
      </span>
    </div>
  );
}
