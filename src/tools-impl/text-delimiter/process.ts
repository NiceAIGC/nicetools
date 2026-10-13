export interface ParsedIndexes {
  indexes: number[];
  error: string | null;
}

export interface DelimitedTextResult {
  output: string;
  lineCount: number;
  minColumns: number;
  maxColumns: number;
  invalidLines: number[];
  indexes: number[];
}

export function parseIndexOrder(raw: string): ParsedIndexes {
  const order = raw.trim();
  if (!order) return { indexes: [], error: "请输入索引顺序" };

  // Keep the original tool's shorthand: "021" means indexes 0, 2, 1.
  const parts = !order.includes(",") && order.length > 1
    ? Array.from(order)
    : order.split(",");

  const indexes: number[] = [];
  for (const part of parts) {
    const token = part.trim();
    if (!/^\d+$/.test(token)) {
      return {
        indexes: [],
        error: `无效索引“${token || "空值"}”，请输入非负整数`,
      };
    }
    const index = Number(token);
    if (!Number.isSafeInteger(index)) {
      return { indexes: [], error: `索引“${token}”超出有效范围` };
    }
    indexes.push(index);
  }

  return { indexes, error: null };
}

export interface IndexPreset {
  label: string;
  value: string;
}

/** 按实际列数生成常用重排方案；尚未识别列数时给固定示例。 */
export function indexPresets(columnCount: number): IndexPreset[] {
  if (columnCount <= 1) {
    return [
      { label: "0", value: "0" },
      { label: "0,1", value: "0,1" },
      { label: "1,0", value: "1,0" },
      { label: "2,1,0", value: "2,1,0" },
    ];
  }

  const columns = Array.from({ length: columnCount }, (_, index) => index);
  const candidates: IndexPreset[] = [
    { label: `全部 ${columnCount} 列`, value: columns.join(",") },
    { label: "倒序", value: [...columns].reverse().join(",") },
    { label: "仅第 1 列", value: "0" },
    { label: "前 2 列", value: "0,1" },
    { label: "2↔1", value: "1,0" },
    { label: "去掉第 1 列", value: columns.slice(1).join(",") },
  ];
  const used = new Set<string>();
  return candidates.filter((preset) => {
    if (used.has(preset.value)) return false;
    used.add(preset.value);
    return true;
  });
}

export function processDelimitedText(
  source: string,
  inputDelimiter: string,
  outputDelimiter: string,
  indexes: number[],
): DelimitedTextResult {
  const lines = source.trim().replace(/\r\n?/g, "\n").split("\n");
  const invalidLines: number[] = [];
  let minColumns = Number.POSITIVE_INFINITY;
  let maxColumns = 0;

  const output = lines.map((line, lineIndex) => {
    const segments = inputDelimiter === ""
      ? Array.from(line)
      : line.split(inputDelimiter);
    if (segments.length < minColumns) minColumns = segments.length;
    if (segments.length > maxColumns) maxColumns = segments.length;

    let invalid = false;
    const selected = indexes.map((index) => {
      if (index >= segments.length) {
        invalid = true;
        return "";
      }
      return segments[index];
    });

    if (invalid) invalidLines.push(lineIndex + 1);
    return selected.join(outputDelimiter);
  });

  return {
    output: output.join("\n"),
    lineCount: lines.length,
    minColumns,
    maxColumns,
    invalidLines,
    indexes,
  };
}
