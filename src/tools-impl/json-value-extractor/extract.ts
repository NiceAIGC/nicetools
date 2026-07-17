export interface ExtractionError {
  line: number;
  message: string;
}

export interface ExtractionResult {
  output: string;
  total: number;
  success: number;
  errors: ExtractionError[];
}

function displayValue(value: unknown): string {
  if (typeof value === "object" && value !== null) {
    return JSON.stringify(value) ?? "";
  }
  return String(value);
}

export function extractJsonValues(
  source: string,
  key: string,
): ExtractionResult {
  const lines = source.trim().replace(/\r\n?/g, "\n").split("\n");
  const values: string[] = [];
  const errors: ExtractionError[] = [];
  let success = 0;

  lines.forEach((originalLine, index) => {
    const lineNumber = index + 1;
    const line = originalLine.trim();

    if (!line) {
      values.push("");
      errors.push({ line: lineNumber, message: "空行，已跳过" });
      return;
    }

    let parsed: unknown;
    try {
      parsed = JSON.parse(line);
    } catch (error) {
      values.push("");
      errors.push({
        line: lineNumber,
        message: `JSON 格式错误 - ${
          error instanceof Error ? error.message : "无法解析"
        }`,
      });
      return;
    }

    if (parsed === null || typeof parsed !== "object") {
      values.push("");
      errors.push({ line: lineNumber, message: "该行不是 JSON 对象" });
      return;
    }

    if (!Object.prototype.hasOwnProperty.call(parsed, key)) {
      values.push("");
      errors.push({ line: lineNumber, message: `未找到键 "${key}"` });
      return;
    }

    values.push(displayValue((parsed as Record<string, unknown>)[key]));
    success += 1;
  });

  return {
    output: values.join("\n"),
    total: lines.length,
    success,
    errors,
  };
}
