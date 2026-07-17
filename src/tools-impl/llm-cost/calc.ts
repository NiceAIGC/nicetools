// 大模型费用计算器 —— 纯计算逻辑（无框架依赖，可单测）
// 从原始 1.html 完整移植。

export type Currency = "CNY" | "USD";
export type DurationUnit = "minute" | "hour" | "day" | "month";
export type NumericInput = number | string;

export interface CalcState {
  modelName: string;
  currency: Currency;
  inputPrice: NumericInput;
  outputPrice: NumericInput;
  cacheCreatePrice: NumericInput;
  cacheReadPrice: NumericInput;
  tpm: NumericInput;
  durationValue: NumericInput;
  durationUnit: DurationUnit;
  monthDays: NumericInput;
  inputRatio: NumericInput;
  outputRatio: NumericInput;
  cacheReadRate: NumericInput;
  cacheCreateRate: NumericInput;
  paymentRate: NumericInput;
}

export const defaults: CalcState = {
  modelName: "glm-5.2",
  currency: "CNY",
  inputPrice: 8,
  outputPrice: 28,
  cacheCreatePrice: 8,
  cacheReadPrice: 2,
  tpm: 500,
  durationValue: 1,
  durationUnit: "month",
  monthDays: 30,
  inputRatio: 8,
  outputRatio: 1,
  cacheReadRate: 70,
  cacheCreateRate: 0,
  paymentRate: 45,
};

// 数值参与计算的字段（其余为文本/枚举）。
export const numericFields: (keyof CalcState)[] = [
  "inputPrice",
  "outputPrice",
  "cacheCreatePrice",
  "cacheReadPrice",
  "tpm",
  "durationValue",
  "monthDays",
  "inputRatio",
  "outputRatio",
  "cacheReadRate",
  "cacheCreateRate",
  "paymentRate",
];

export function toNumber(value: unknown, fallback: number): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

export function nonNegative(value: unknown): number {
  return Math.max(0, toNumber(value, 0));
}

export function percent(value: unknown): number {
  return Math.min(100, Math.max(0, toNumber(value, 0)));
}

export function paymentRateValue(value: unknown): number {
  return percent(toNumber(value, defaults.paymentRate as number));
}

function paymentRatio(value: unknown): number {
  return paymentRateValue(value) / 100;
}

export interface CurrencyMeta {
  symbol: string;
  unit: string;
  name: string;
}

export function getCurrencyMeta(currency: Currency): CurrencyMeta {
  if (currency === "USD") {
    return { symbol: "$", unit: "美元 / 百万 token", name: "美金" };
  }
  return { symbol: "¥", unit: "元 / 百万 token", name: "人民币" };
}

function durationToMinutes(
  value: unknown,
  unit: DurationUnit,
  monthDays: unknown,
): number {
  const safeValue = nonNegative(value);
  const safeMonthDays = Math.max(1, nonNegative(monthDays) || (defaults.monthDays as number));
  const unitMap: Record<DurationUnit, number> = {
    minute: 1,
    hour: 60,
    day: 1440,
    month: 1440 * safeMonthDays,
  };
  return safeValue * (unitMap[unit] || 1);
}

function actualTpm(state: CalcState): number {
  return nonNegative(state.tpm) * 10000;
}

export interface CalcResult {
  monthDays: number;
  tpm: number;
  paymentRatio: number;
  durationMinutes: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  normalInputRate: number;
  rateOverflow: boolean;
  normalInputTokens: number;
  cacheCreateTokens: number;
  cacheReadTokens: number;
  normalInputCost: number;
  cacheCreateCost: number;
  cacheReadCost: number;
  outputCost: number;
  totalCost: number;
  actualTotalCost: number;
  minuteCost: number;
  hourlyCost: number;
  dailyCost: number;
  monthlyCost: number;
  actualMinuteCost: number;
  actualHourlyCost: number;
  actualDailyCost: number;
  actualMonthlyCost: number;
}

// 单分钟折前费用（当时长为 0 时的兜底口径，保持与原实现一致）。
function calculateMinuteCost(state: CalcState): number {
  const tpm = actualTpm(state);
  const inputRatio = nonNegative(state.inputRatio);
  const outputRatio = nonNegative(state.outputRatio);
  const ratioTotal = inputRatio + outputRatio;
  if (ratioTotal <= 0) return 0;

  const inputTokens = (tpm * inputRatio) / ratioTotal;
  const outputTokens = (tpm * outputRatio) / ratioTotal;
  const cacheReadRate = percent(state.cacheReadRate) / 100;
  const cacheCreateRate = percent(state.cacheCreateRate) / 100;
  const cachedRateTotal = cacheReadRate + cacheCreateRate;
  const normalInputRate = Math.max(0, 1 - cachedRateTotal);

  const normalInputTokens = inputTokens * normalInputRate;
  let cacheCreateTokens = inputTokens * cacheCreateRate;
  let cacheReadTokens = inputTokens * cacheReadRate;
  if (cachedRateTotal > 1) {
    const scale =
      inputTokens > 0 ? inputTokens / (cacheCreateTokens + cacheReadTokens) : 0;
    cacheCreateTokens *= scale;
    cacheReadTokens *= scale;
  }

  return (
    (normalInputTokens / 1000000) * nonNegative(state.inputPrice) +
    (cacheCreateTokens / 1000000) * nonNegative(state.cacheCreatePrice) +
    (cacheReadTokens / 1000000) * nonNegative(state.cacheReadPrice) +
    (outputTokens / 1000000) * nonNegative(state.outputPrice)
  );
}

export function calculate(state: CalcState): CalcResult {
  const tpm = actualTpm(state);
  const monthDays = Math.max(1, nonNegative(state.monthDays) || (defaults.monthDays as number));
  const durationMinutes = durationToMinutes(
    state.durationValue,
    state.durationUnit,
    monthDays,
  );
  const totalTokens = tpm * durationMinutes;

  const inputRatio = nonNegative(state.inputRatio);
  const outputRatio = nonNegative(state.outputRatio);
  const ratioTotal = inputRatio + outputRatio;
  const inputShare = ratioTotal > 0 ? inputRatio / ratioTotal : 0;
  const outputShare = ratioTotal > 0 ? outputRatio / ratioTotal : 0;

  const inputTokens = totalTokens * inputShare;
  const outputTokens = totalTokens * outputShare;

  const cacheReadRate = percent(state.cacheReadRate) / 100;
  const cacheCreateRate = percent(state.cacheCreateRate) / 100;
  const cachedRateTotal = cacheReadRate + cacheCreateRate;
  const normalInputRate = Math.max(0, 1 - cachedRateTotal);

  const normalInputTokens = inputTokens * normalInputRate;
  let cacheCreateTokens = inputTokens * cacheCreateRate;
  let cacheReadTokens = inputTokens * cacheReadRate;

  if (cachedRateTotal > 1) {
    const scale =
      inputTokens > 0 ? inputTokens / (cacheCreateTokens + cacheReadTokens) : 0;
    cacheCreateTokens *= scale;
    cacheReadTokens *= scale;
  }

  const inputPrice = nonNegative(state.inputPrice);
  const outputPrice = nonNegative(state.outputPrice);
  const cacheCreatePrice = nonNegative(state.cacheCreatePrice);
  const cacheReadPrice = nonNegative(state.cacheReadPrice);

  const normalInputCost = (normalInputTokens / 1000000) * inputPrice;
  const cacheCreateCost = (cacheCreateTokens / 1000000) * cacheCreatePrice;
  const cacheReadCost = (cacheReadTokens / 1000000) * cacheReadPrice;
  const outputCost = (outputTokens / 1000000) * outputPrice;
  const totalCost =
    normalInputCost + cacheCreateCost + cacheReadCost + outputCost;
  const paymentRatioValue = paymentRatio(state.paymentRate);
  const actualTotalCost = totalCost * paymentRatioValue;

  const minuteCost =
    durationMinutes > 0 ? totalCost / durationMinutes : calculateMinuteCost(state);
  const hourlyCost = minuteCost * 60;
  const dailyCost = minuteCost * 1440;
  const monthlyCost = dailyCost * monthDays;
  const actualMinuteCost = minuteCost * paymentRatioValue;
  const actualHourlyCost = hourlyCost * paymentRatioValue;
  const actualDailyCost = dailyCost * paymentRatioValue;
  const actualMonthlyCost = monthlyCost * paymentRatioValue;

  return {
    monthDays,
    tpm,
    paymentRatio: paymentRatioValue,
    durationMinutes,
    totalTokens,
    inputTokens,
    outputTokens,
    normalInputRate,
    rateOverflow: cachedRateTotal > 1,
    normalInputTokens,
    cacheCreateTokens,
    cacheReadTokens,
    normalInputCost,
    cacheCreateCost,
    cacheReadCost,
    outputCost,
    totalCost,
    actualTotalCost,
    minuteCost,
    hourlyCost,
    dailyCost,
    monthlyCost,
    actualMinuteCost,
    actualHourlyCost,
    actualDailyCost,
    actualMonthlyCost,
  };
}

// ---- 格式化 ----

export function formatNumber(value: number, digits: number): string {
  return new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: digits,
    minimumFractionDigits: 0,
  }).format(Number.isFinite(value) ? value : 0);
}

export function formatPercent(value: number): string {
  return formatNumber(value * 100, 2) + "%";
}

export function formatDiscountFromRate(value: unknown): string {
  return formatNumber(paymentRateValue(value) / 10, 2) + "折";
}

export function formatMoney(value: number, currency: Currency): string {
  const meta = getCurrencyMeta(currency);
  const safe = Number.isFinite(value) ? value : 0;
  const digits = Math.abs(safe) >= 100 ? 2 : 4;
  return (
    meta.symbol +
    new Intl.NumberFormat("zh-CN", {
      minimumFractionDigits: 2,
      maximumFractionDigits: digits,
    }).format(safe)
  );
}

export function formatUnitPrice(value: unknown, currency: Currency): string {
  return formatMoney(nonNegative(value), currency);
}

export function durationUnitText(unit: DurationUnit): string {
  const map: Record<DurationUnit, string> = {
    minute: "分钟",
    hour: "小时",
    day: "天",
    month: "月",
  };
  return map[unit] || "分钟";
}

// 生成“复制结果”的纯文本报告，与原实现口径一致。
export function resultText(state: CalcState): string {
  const result = calculate(state);
  const meta = getCurrencyMeta(state.currency);
  const money = (v: number) => formatMoney(v, state.currency);
  const unitPrice = (v: unknown) => formatUnitPrice(v, state.currency);

  const lines = [
    "大模型费用计算结果",
    "模型：" + state.modelName,
    "币种：" + meta.name,
    "",
    "价格设置",
    "输入价格：" + unitPrice(state.inputPrice) + " / 百万 token",
    "输出价格：" + unitPrice(state.outputPrice) + " / 百万 token",
    "缓存创建价格：" + unitPrice(state.cacheCreatePrice) + " / 百万 token",
    "缓存读取价格：" + unitPrice(state.cacheReadPrice) + " / 百万 token",
    "",
    "用量设置",
    "总 TPM 输入值：" + formatNumber(nonNegative(state.tpm), 2) + " 万 token / 分钟",
    "实际 TPM：" + formatNumber(result.tpm, 0) + " token / 分钟",
    "运行时长：" + state.durationValue + " " + durationUnitText(state.durationUnit),
    "每月天数：" + result.monthDays + " 天",
    "输入输出比例：" + state.inputRatio + ":" + state.outputRatio,
    "缓存读取率：" + formatNumber(percent(state.cacheReadRate), 2) + "%",
    "缓存创建率：" + formatNumber(percent(state.cacheCreateRate), 2) + "%",
    "普通输入率：" + formatPercent(result.normalInputRate),
    "实际支付比例：" + formatPercent(result.paymentRatio),
    "折扣：" + formatDiscountFromRate(state.paymentRate),
    "",
    "Token 明细",
    "当前时长总 token：" + formatNumber(result.totalTokens, 0),
    "输入 token：" + formatNumber(result.inputTokens, 0),
    "输出 token：" + formatNumber(result.outputTokens, 0),
    "普通输入 token：" + formatNumber(result.normalInputTokens, 0),
    "缓存创建 token：" + formatNumber(result.cacheCreateTokens, 0),
    "缓存读取 token：" + formatNumber(result.cacheReadTokens, 0),
    "",
    "折前费用明细",
    "普通输入费用：" + money(result.normalInputCost),
    "缓存创建费用：" + money(result.cacheCreateCost),
    "缓存读取费用：" + money(result.cacheReadCost),
    "输出费用：" + money(result.outputCost),
    "当前时长折前总费用：" + money(result.totalCost),
    "每分钟折前费用：" + money(result.minuteCost),
    "每小时折前费用：" + money(result.hourlyCost),
    "每日折前费用：" + money(result.dailyCost),
    "每月折前费用：" + money(result.monthlyCost),
    "",
    "折后费用明细",
    "普通输入实际消耗：" + money(result.normalInputCost * result.paymentRatio),
    "缓存创建实际消耗：" + money(result.cacheCreateCost * result.paymentRatio),
    "缓存读取实际消耗：" + money(result.cacheReadCost * result.paymentRatio),
    "输出实际消耗：" + money(result.outputCost * result.paymentRatio),
    "",
    "折后实际消耗",
    "当前时长实际消耗：" + money(result.actualTotalCost),
    "每分钟实际消耗：" + money(result.actualMinuteCost),
    "每小时实际消耗：" + money(result.actualHourlyCost),
    "每日实际消耗：" + money(result.actualDailyCost),
    "每月实际消耗：" + money(result.actualMonthlyCost),
  ];
  return lines.join("\n");
}

// ---- 持久化（含 v1 → v2 迁移） ----

export const STORAGE_KEY = "llm-cost-calculator:v2";
export const LEGACY_STORAGE_KEY = "llm-cost-calculator:v1";

export function loadState(): CalcState {
  try {
    let raw = window.localStorage.getItem(STORAGE_KEY);
    let isLegacy = false;
    if (!raw) {
      raw = window.localStorage.getItem(LEGACY_STORAGE_KEY);
      isLegacy = Boolean(raw);
    }
    if (!raw) return { ...defaults };
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (isLegacy && typeof parsed.tpm === "number") {
      parsed.tpm = (parsed.tpm as number) / 10000;
    }
    if (parsed.paymentRate == null && parsed.finalDiscount != null) {
      parsed.paymentRate =
        toNumber(parsed.finalDiscount, (defaults.paymentRate as number) / 10) * 10;
    }
    delete parsed.finalDiscount;
    delete parsed.discountPresets;
    return { ...defaults, ...(parsed as Partial<CalcState>) };
  } catch {
    return { ...defaults };
  }
}

export function hasSavedState(): boolean {
  try {
    return Boolean(
      window.localStorage.getItem(STORAGE_KEY) ||
        window.localStorage.getItem(LEGACY_STORAGE_KEY),
    );
  } catch {
    return false;
  }
}
