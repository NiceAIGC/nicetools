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

export function processDelimitedText(
  source: string,
  inputDelimiter: string,
  outputDelimiter: string,
  indexes: number[],
): DelimitedTextResult {
  const lines = source.trim().replace(/\r\n?/g, "\n").split("\n");
  const columnCounts: number[] = [];
  const invalidLines: number[] = [];

  const output = lines.map((line, lineIndex) => {
    const segments = inputDelimiter === ""
      ? Array.from(line)
      : line.split(inputDelimiter);
    columnCounts.push(segments.length);

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
    minColumns: Math.min(...columnCounts),
    maxColumns: Math.max(...columnCounts),
    invalidLines,
    indexes,
  };
}
