import type { RawPage } from "./readers";

export type ScanMode = "color" | "gray" | "bw";

export interface ScanSettings {
  mode: ScanMode;
  border: boolean;
  rotate: number;
  rotateVariance: number;
  brightness: number;
  contrast: number;
  yellowish: number;
  blur: number;
  noise: number;
  dpi: number;
}

export const SCAN_PRESETS: Record<string, ScanSettings> = {
  soft: {
    mode: "color",
    border: true,
    rotate: 0.6,
    rotateVariance: 0.3,
    brightness: 1.02,
    contrast: 1.02,
    yellowish: 0.05,
    blur: 0.35,
    noise: 0.05,
    dpi: 150,
  },
  standard: {
    mode: "gray",
    border: true,
    rotate: 1,
    rotateVariance: 0.5,
    brightness: 1.05,
    contrast: 1.15,
    yellowish: 0.12,
    blur: 0.55,
    noise: 0.12,
    dpi: 150,
  },
  vintage: {
    mode: "color",
    border: true,
    rotate: 1.2,
    rotateVariance: 0.6,
    brightness: 1.08,
    contrast: 1.08,
    yellowish: 0.3,
    blur: 0.7,
    noise: 0.16,
    dpi: 150,
  },
  copy: {
    mode: "bw",
    border: true,
    rotate: 0.8,
    rotateVariance: 0.4,
    brightness: 1.1,
    contrast: 1.35,
    yellowish: 0,
    blur: 0.45,
    noise: 0.2,
    dpi: 150,
  },
};

export interface PdfPageSpec {
  jpeg: Uint8Array;
  pixelWidth: number;
  pixelHeight: number;
  pageWidth: number;
  pageHeight: number;
}

/** 预览与导出共用同一旋转抖动，保证所见即所得。 */
function rotationJitter(pageIndex: number, variance: number): number {
  const wave = Math.sin((pageIndex + 1) * 12.9898) * 43758.5453;
  return ((wave - Math.floor(wave)) * 2 - 1) * variance;
}

function canvasContext(canvas: HTMLCanvasElement): CanvasRenderingContext2D {
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("当前浏览器无法创建画布上下文");
  return ctx;
}

/** 把一页源内容渲染成带扫描质感的位图。 */
export async function renderScanCanvas(
  page: RawPage,
  settings: ScanSettings,
  pageIndex: number,
): Promise<HTMLCanvasElement> {
  const pixelWidth = Math.max(1, Math.round((page.widthPt * settings.dpi) / 72));
  const pixelHeight = Math.max(1, Math.round((pixelWidth * page.heightPt) / page.widthPt));
  const source = await page.render(pixelWidth);

  const output = document.createElement("canvas");
  output.width = pixelWidth;
  output.height = pixelHeight;
  const ctx = canvasContext(output);
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, pixelWidth, pixelHeight);

  const angle = ((settings.rotate + rotationJitter(pageIndex, settings.rotateVariance)) * Math.PI) / 180;
  const scale = settings.border ? 0.955 : 1;
  const drawWidth = pixelWidth * scale;
  const drawHeight = pixelHeight * scale;
  const filters = [
    settings.mode === "color" ? "" : "grayscale(1)",
    `brightness(${settings.brightness})`,
    `contrast(${settings.contrast})`,
    settings.blur > 0 ? `blur(${settings.blur * (settings.dpi / 96)}px)` : "",
  ]
    .filter(Boolean)
    .join(" ");

  ctx.save();
  ctx.translate(pixelWidth / 2, pixelHeight / 2);
  ctx.rotate(angle);
  ctx.filter = filters;
  ctx.drawImage(source, -drawWidth / 2, -drawHeight / 2, drawWidth, drawHeight);
  ctx.filter = "none";
  if (settings.border) {
    ctx.strokeStyle = "rgba(0, 0, 0, 0.18)";
    ctx.lineWidth = Math.max(1, pixelWidth * 0.0015);
    ctx.strokeRect(-drawWidth / 2, -drawHeight / 2, drawWidth, drawHeight);
  }
  ctx.restore();

  const image = ctx.getImageData(0, 0, pixelWidth, pixelHeight);
  const data = image.data;
  const yellowish = settings.yellowish;
  const noise = settings.noise * 90;
  const isBlackAndWhite = settings.mode === "bw";
  for (let i = 0; i < data.length; i += 4) {
    let red = data[i];
    let green = data[i + 1];
    let blue = data[i + 2];
    if (yellowish > 0) {
      red = red * (1 - yellowish) + (red * 1.05 + 14) * yellowish;
      green = green * (1 - yellowish) + (green * 1.01 + 9) * yellowish;
      blue = blue * (1 - yellowish) + blue * 0.88 * yellowish;
    }
    if (noise > 0) {
      const offset = (Math.random() - 0.5) * noise;
      red += offset;
      green += offset;
      blue += offset;
    }
    if (isBlackAndWhite) {
      const shade = 0.299 * red + 0.587 * green + 0.114 * blue > 148 ? 255 : 0;
      red = shade;
      green = shade;
      blue = shade;
    }
    data[i] = Math.max(0, Math.min(255, red));
    data[i + 1] = Math.max(0, Math.min(255, green));
    data[i + 2] = Math.max(0, Math.min(255, blue));
  }
  ctx.putImageData(image, 0, 0);
  return output;
}

function encodeJpeg(canvas: HTMLCanvasElement): Promise<Uint8Array> {
  const { promise, resolve, reject } = Promise.withResolvers<Uint8Array>();
  canvas.toBlob(
    (blob) => {
      if (!blob) {
        reject(new Error("图片编码失败"));
        return;
      }
      void blob.arrayBuffer().then((buffer) => resolve(new Uint8Array(buffer)));
    },
    "image/jpeg",
    0.82,
  );
  return promise;
}

/** 逐页生成扫描位图并编码为可写入 PDF 的 JPEG。 */
export async function renderScannedPage(
  page: RawPage,
  settings: ScanSettings,
  pageIndex: number,
): Promise<PdfPageSpec> {
  const canvas = await renderScanCanvas(page, settings, pageIndex);
  return {
    jpeg: await encodeJpeg(canvas),
    pixelWidth: canvas.width,
    pixelHeight: canvas.height,
    pageWidth: page.widthPt,
    pageHeight: page.heightPt,
  };
}

/** 极简 PDF 写入器：每页一张 DCTDecode JPEG，不引入额外依赖。 */
export function buildPdf(pages: PdfPageSpec[]): Uint8Array<ArrayBuffer> {
  const encoder = new TextEncoder();
  const chunks: Uint8Array[] = [];
  const offsets: number[] = [];
  let length = 0;
  const push = (chunk: Uint8Array | string) => {
    const bytes = typeof chunk === "string" ? encoder.encode(chunk) : chunk;
    chunks.push(bytes);
    length += bytes.length;
  };

  push("%PDF-1.4\n%");
  push(new Uint8Array([0xe2, 0xe3, 0xcf, 0xd3]));
  push("\n");

  offsets.push(length);
  push("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n");
  offsets.push(length);
  const kids = pages.map((_, index) => `${3 + index * 3} 0 R`).join(" ");
  push(`2 0 obj\n<< /Type /Pages /Kids [${kids}] /Count ${pages.length} >>\nendobj\n`);

  pages.forEach((page, index) => {
    const pageId = 3 + index * 3;
    const contentId = pageId + 1;
    const imageId = pageId + 2;
    offsets.push(length);
    push(
      `${pageId} 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${page.pageWidth.toFixed(2)} ${page.pageHeight.toFixed(2)}] /Resources << /XObject << /Im${index} ${imageId} 0 R >> >> /Contents ${contentId} 0 R >>\nendobj\n`,
    );

    const content = `q ${page.pageWidth.toFixed(2)} 0 0 ${page.pageHeight.toFixed(2)} 0 0 cm /Im${index} Do Q\n`;
    offsets.push(length);
    push(`${contentId} 0 obj\n<< /Length ${encoder.encode(content).length} >>\nstream\n${content}endstream\nendobj\n`);

    offsets.push(length);
    push(
      `${imageId} 0 obj\n<< /Type /XObject /Subtype /Image /Width ${page.pixelWidth} /Height ${page.pixelHeight} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${page.jpeg.length} >>\nstream\n`,
    );
    push(page.jpeg);
    push("\nendstream\nendobj\n");
  });

  const xrefOffset = length;
  const size = offsets.length + 1;
  push(`xref\n0 ${size}\n0000000000 65535 f \n`);
  offsets.forEach((offset) => push(`${String(offset).padStart(10, "0")} 00000 n \n`));
  push(`trailer\n<< /Size ${size} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF\n`);

  const pdf = new Uint8Array(length);
  let cursor = 0;
  chunks.forEach((chunk) => {
    pdf.set(chunk, cursor);
    cursor += chunk.length;
  });
  return pdf;
}
