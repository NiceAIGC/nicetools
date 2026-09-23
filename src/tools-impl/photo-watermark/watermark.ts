// 水印绘制核心逻辑，移植自 cuijianzhuang/photo-watermark（GPL-3.0）的 js/script.js：
// 文本换行、字号换算、平铺/单个/自定义数量三种布局与文件命名规则保持一致。

export type WatermarkPattern = "tile" | "single" | "custom";

export type WatermarkPosition =
  | "center"
  | "top-left"
  | "top-right"
  | "bottom-left"
  | "bottom-right";

export interface WatermarkOptions {
  text: string;
  color: string;
  alpha: number;
  space: number;
  size: number;
  rotate: number;
  fontFamily: string;
  bold: boolean;
  italic: boolean;
  pattern: WatermarkPattern;
  count: number;
  position: WatermarkPosition;
}

export const MAX_WATERMARK_TEXT = 100;

export const defaultWatermarkOptions: WatermarkOptions = {
  text: "版权所有 侵权必究",
  color: "#ffffff",
  alpha: 0.3,
  space: 1.5,
  size: 0.8,
  rotate: 45,
  fontFamily: "黑体",
  bold: false,
  italic: false,
  pattern: "tile",
  count: 1,
  position: "center",
};

export const watermarkFonts = [
  "黑体",
  "宋体",
  "仿宋",
  "楷体",
  "隶书",
  "幼圆",
  "Arial",
  "Helvetica",
  "Tahoma",
  "Verdana",
  "Georgia",
  "TimesNewRoman",
];

export const watermarkTemplates: { label: string; text: string }[] = [
  { label: "办事模板", text: "仅供办理XX使用，他用无效" },
  { label: "认证模板", text: "仅用于XX认证，他用无效" },
  { label: "复印核验", text: "复印件与原件相符" },
  { label: "证件模板", text: "该证件仅供XX查看，不得他用" },
  { label: "内部资料", text: "内部资料，请勿外传" },
  { label: "版权声明", text: "版权所有，严禁外传" },
];

export const watermarkPatterns: { key: WatermarkPattern; label: string }[] = [
  { key: "tile", label: "平铺模式" },
  { key: "single", label: "单个水印" },
  { key: "custom", label: "自定义数量" },
];

export const watermarkPositions: { key: WatermarkPosition; label: string }[] = [
  { key: "center", label: "居中" },
  { key: "top-left", label: "左上角" },
  { key: "top-right", label: "右上角" },
  { key: "bottom-left", label: "左下角" },
  { key: "bottom-right", label: "右下角" },
];

const supportedImageTypes = [
  "image/jpeg",
  "image/png",
  "image/gif",
  "image/webp",
  "image/bmp",
  "image/x-icon",
  "image/vnd.microsoft.icon",
];

export const supportedImageFormatLabel = "jpg、jpeg、png、gif、webp、bmp、ico";

export function isSupportedImage(file: File): boolean {
  return supportedImageTypes.includes(file.type);
}

/** canvas → Blob；toBlob 返回空时最多重试 3 次（间隔 1 秒）。 */
export async function canvasToBlob(
  canvas: HTMLCanvasElement,
  type = "image/png",
  quality?: number,
): Promise<Blob> {
  for (let attempt = 0; ; attempt += 1) {
    const { promise, resolve, reject } = Promise.withResolvers<Blob>();
    canvas.toBlob(
      (blob) => {
        if (blob) resolve(blob);
        else reject(new Error("转换图片失败"));
      },
      type,
      quality,
    );
    try {
      return await promise;
    } catch (error) {
      if (attempt >= 3) throw error;
      const { promise: wait, resolve: done } = Promise.withResolvers<void>();
      window.setTimeout(done, 1000);
      await wait;
    }
  }
}

/** 与源项目一致：白色 / 十六进制颜色 → rgba(...) */
export function formatWatermarkColor(color: string, alpha: number): string {
  const match = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(color.trim());
  const [r, g, b] = match
    ? [
        parseInt(match[1], 16),
        parseInt(match[2], 16),
        parseInt(match[3], 16),
      ]
    : [255, 255, 255];
  return `rgba(${r},${g},${b},${alpha})`;
}

// 分段规则：单字符标点/空白、数字与日期片段、英文单词、中文词组、其它符号各成一段。
const segmentPattern =
  /([，。！？；：、,.!?;:\s]|[0-9-/年月日]+|[a-zA-Z]+|[\u4e00-\u9fa5]+|[^\u4e00-\u9fa5a-zA-Z0-9\s])/g;
const punctuationPattern = /^[，。！？；：、,.!?;:\s]$/;

function isPunctuation(segment: string): boolean {
  return punctuationPattern.test(segment);
}

/**
 * 文本自动换行：优先在标点、数字/日期、英文单词之间断行，
 * 标点不与前一行分离，超长片段按字符拆分。
 */
export function wrapWatermarkText(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxWidth: number,
): string[] {
  if (!text || typeof text !== "string") return ["水印文字"];
  const width = Math.max(50, maxWidth || 100);
  const measure = (value: string) => ctx.measureText(value).width;
  const segments = text.match(segmentPattern) ?? [];
  const lines: string[] = [];
  let currentLine = "";

  for (let i = 0; i < segments.length; i += 1) {
    const segment = (segments[i] ?? "").trim();
    if (!segment) continue;

    const nextSegment = (segments[i + 1] ?? "").trim();
    const isNextPunctuation = isPunctuation(nextSegment);

    // 标点符号：尽量留在上一行末尾
    if (isPunctuation(segment)) {
      if (!currentLine && lines.length > 0) {
        const lastLine = lines[lines.length - 1];
        const merged = lastLine + segment;
        if (measure(merged) <= width) lines[lines.length - 1] = merged;
        else currentLine = segment;
      } else {
        currentLine += segment;
      }
      continue;
    }

    const testLine = currentLine + segment + (isNextPunctuation ? nextSegment : "");
    if (measure(testLine) <= width) {
      currentLine += segment;
      if (isNextPunctuation) {
        currentLine += nextSegment;
        i += 1;
      }
      continue;
    }

    if (currentLine) {
      if (!isPunctuation(currentLine)) {
        lines.push(currentLine);
        currentLine = segment;
        if (isNextPunctuation && measure(segment + nextSegment) <= width) {
          currentLine += nextSegment;
          i += 1;
        }
      } else {
        currentLine += segment;
      }
      continue;
    }

    // 没有当前行且片段超宽：短片段独立成行，长片段按字符拆分
    if (segment.length <= 4) {
      lines.push(segment);
      currentLine = "";
      continue;
    }

    let tempLine = "";
    for (const char of segment) {
      const candidate = tempLine + char;
      if (measure(candidate) <= width) {
        tempLine = candidate;
        continue;
      }
      if (!tempLine) {
        tempLine = char;
        continue;
      }
      if (isPunctuation(char) && measure(tempLine + char) <= width * 1.1) {
        tempLine += char;
      } else {
        lines.push(tempLine);
        tempLine = char;
      }
    }
    if (tempLine) currentLine = tempLine;
  }

  if (currentLine) {
    if (!isPunctuation(currentLine) || lines.length === 0) {
      lines.push(currentLine);
    } else {
      const lastLine = lines[lines.length - 1];
      const merged = lastLine + currentLine;
      if (measure(merged) <= width * 1.1) lines[lines.length - 1] = merged;
      else lines.push(currentLine);
    }
  }

  return lines.length > 0 ? lines : ["水印文字"];
}

/** 就地重绘：重置画布尺寸清空后按原图重画，再绘制水印。 */
export function drawWatermark(
  canvas: HTMLCanvasElement,
  img: HTMLImageElement,
  options: WatermarkOptions,
): void {
  if (!canvas || !img) return;

  const { width, height } = canvas;
  if (!width || !height) return;

  // 重新设置宽高会清空画布
  canvas.width = width;
  canvas.height = height;

  const ctx = canvas.getContext("2d");
  if (!ctx) return;

  ctx.drawImage(img, 0, 0);

  const pattern = options.pattern ?? "tile";
  const position = options.position ?? "center";
  const count = Math.max(1, Math.trunc(options.count) || 1);
  const fontFamily = options.fontFamily || "黑体";
  const fontWeight = options.bold ? "bold" : "normal";
  const fontStyle = options.italic ? "italic" : "normal";

  const diagonal = Math.sqrt(width * width + height * height) || 1000;
  const textSize = Math.max(12, (options.size || 1) * Math.max(15, diagonal / 25));

  ctx.font = `${fontStyle} ${fontWeight} ${textSize}px "${fontFamily}"`;
  ctx.fillStyle = formatWatermarkColor(options.color, options.alpha);

  const text = options.text?.trim() || "水印文字";
  const maxWidth = Math.min(Math.max(100, width * 0.8), 500);
  const lines = wrapWatermarkText(ctx, text, maxWidth);
  const lineHeight = textSize * 1.2;
  const totalHeight = lineHeight * lines.length;

  if (pattern === "tile") {
    ctx.save();
    ctx.translate(width / 2, height / 2);
    ctx.rotate((options.rotate * Math.PI) / 180);

    const maxLineWidth = Math.max(...lines.map((line) => ctx.measureText(line).width));
    const xStep = maxLineWidth + ctx.measureText("啊").width;
    const yStep = options.space * (totalHeight + textSize);
    const startX = -diagonal / 2;
    const startY = -diagonal / 2;
    const cols = Math.ceil(diagonal / xStep);
    const rows = Math.ceil(diagonal / yStep);

    for (let i = 0; i <= cols; i += 1) {
      for (let j = 0; j <= rows; j += 1) {
        const x = startX + i * xStep;
        const y = startY + j * yStep;
        lines.forEach((line, index) => {
          const lineY = y + (index - (lines.length - 1) / 2) * lineHeight;
          const lineWidth = ctx.measureText(line).width;
          ctx.fillText(line, x - lineWidth / 2, lineY + textSize / 2);
        });
      }
    }
    ctx.restore();
    return;
  }

  const positions: [number, number][] = [];
  const padding = Math.min(width, height) * 0.1;

  if (pattern === "single" || count === 1) {
    const maxLineWidth = Math.max(...lines.map((line) => ctx.measureText(line).width));
    switch (position) {
      case "top-left":
        positions.push([maxLineWidth / 2 + padding, totalHeight / 2 + padding]);
        break;
      case "top-right":
        positions.push([width - maxLineWidth / 2 - padding, totalHeight / 2 + padding]);
        break;
      case "bottom-left":
        positions.push([maxLineWidth / 2 + padding, height - totalHeight / 2 - padding]);
        break;
      case "bottom-right":
        positions.push([
          width - maxLineWidth / 2 - padding,
          height - totalHeight / 2 - padding,
        ]);
        break;
      default:
        positions.push([width / 2, height / 2]);
    }
  } else {
    const cols = Math.ceil(Math.sqrt((count * width) / height));
    const rows = Math.ceil(count / cols);
    const xStep = (width - 2 * padding) / (cols - 1 || 1);
    const yStep = (height - 2 * padding) / (rows - 1 || 1);

    let current = 0;
    for (let i = 0; i < cols && current < count; i += 1) {
      for (let j = 0; j < rows && current < count; j += 1) {
        positions.push([padding + i * xStep, padding + j * yStep]);
        current += 1;
      }
    }
  }

  positions.forEach(([x, y]) => {
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate((options.rotate * Math.PI) / 180);
    lines.forEach((line, index) => {
      const lineY = (index - (lines.length - 1) / 2) * lineHeight;
      const lineWidth = ctx.measureText(line).width;
      ctx.fillText(line, -lineWidth / 2, lineY + textSize / 2);
    });
    ctx.restore();
  });
}

/** 单个图片下载命名：原名 + 时间戳 + .png（与源项目一致）。 */
export function generateFileName(fileName: string): string {
  const now = new Date();
  const two = (value: number) => String(value).padStart(2, "0");
  const timeStr = `${now.getFullYear()}-${two(now.getMonth() + 1)}-${two(now.getDate())} ${two(
    now.getHours(),
  )}${two(now.getMinutes())}${two(now.getSeconds())}`;
  return `${fileName}_${timeStr}.png`;
}

/** 批量压缩包命名：水印图片_<时间戳>.zip（与源项目一致）。 */
export function generateZipName(): string {
  return `水印图片_${Date.now()}.zip`;
}
