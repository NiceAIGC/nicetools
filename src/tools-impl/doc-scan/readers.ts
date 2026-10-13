import * as pdfjs from "pdfjs-dist";
import PdfWorker from "pdfjs-dist/build/pdf.worker.min.mjs?worker&inline";
import JSZip from "jszip";

export interface RawPage {
  widthPt: number;
  heightPt: number;
  render(widthPx: number): Promise<HTMLCanvasElement>;
}

export interface LoadedDocument {
  name: string;
  pages: RawPage[];
  dispose?: () => void;
}

export const SUPPORTED_INPUT_LABEL =
  "PDF、PNG/JPG/WebP/BMP/GIF 图片、Word(.docx)、TXT/Markdown/CSV/JSON";

const LONG_SIDE_PT = 841.89;
const PAGE_WIDTH_PT = 595.28;
const PAGE_HEIGHT_PT = 841.89;
const TEXT_MARGIN_PT = 54;
const TEXT_FONT_SIZE = 11;
const TEXT_LINE_HEIGHT = 16;
const TEXT_EXTENSIONS = /\.(txt|md|markdown|csv|tsv|json|log|text)$/i;

if (!pdfjs.GlobalWorkerOptions.workerPort) {
  pdfjs.GlobalWorkerOptions.workerPort = new PdfWorker();
}

function context2d(canvas: HTMLCanvasElement): CanvasRenderingContext2D {
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("当前浏览器无法创建画布上下文");
  return ctx;
}

async function loadPdfDocument(file: File): Promise<LoadedDocument> {
  const loadingTask = pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()) });
  const pdf = await loadingTask.promise;
  const pages: RawPage[] = [];
  for (let number = 1; number <= pdf.numPages; number += 1) {
    const page = await pdf.getPage(number);
    const base = page.getViewport({ scale: 1 });
    pages.push({
      widthPt: base.width,
      heightPt: base.height,
      async render(widthPx) {
        const canvas = document.createElement("canvas");
        canvas.width = widthPx;
        canvas.height = Math.max(1, Math.round((widthPx * base.height) / base.width));
        await page.render({ canvas, viewport: page.getViewport({ scale: widthPx / base.width }) })
          .promise;
        return canvas;
      },
    });
  }
  return { name: file.name, pages, dispose: () => void loadingTask.destroy() };
}

async function loadImageDocument(file: File): Promise<LoadedDocument> {
  const bitmap = await createImageBitmap(file);
  const aspect =
    bitmap.width >= bitmap.height
      ? { widthPt: LONG_SIDE_PT, heightPt: (LONG_SIDE_PT * bitmap.height) / bitmap.width }
      : { widthPt: (LONG_SIDE_PT * bitmap.width) / bitmap.height, heightPt: LONG_SIDE_PT };
  return {
    name: file.name,
    dispose: () => bitmap.close(),
    pages: [
      {
        ...aspect,
        async render(widthPx) {
          const canvas = document.createElement("canvas");
          canvas.width = widthPx;
          canvas.height = Math.max(1, Math.round((widthPx * aspect.heightPt) / aspect.widthPt));
          context2d(canvas).drawImage(bitmap, 0, 0, canvas.width, canvas.height);
          return canvas;
        },
      },
    ],
  };
}

function wrapParagraph(
  ctx: CanvasRenderingContext2D,
  paragraph: string,
  maxWidth: number,
): string[] {
  const lines: string[] = [];
  let current = "";
  for (const char of paragraph.replace(/\t/g, "    ")) {
    const candidate = current + char;
    if (current !== "" && ctx.measureText(candidate).width > maxWidth) {
      lines.push(current);
      current = char;
    } else {
      current = candidate;
    }
  }
  lines.push(current);
  return lines;
}

function textDocument(name: string, text: string): LoadedDocument {
  const ruler = context2d(document.createElement("canvas"));
  ruler.font = `${TEXT_FONT_SIZE}px sans-serif`;
  const maxWidth = PAGE_WIDTH_PT - TEXT_MARGIN_PT * 2;
  const lines = text
    .replace(/\r\n?/g, "\n")
    .split("\n")
    .flatMap((paragraph) => wrapParagraph(ruler, paragraph, maxWidth));
  const linesPerPage = Math.max(
    1,
    Math.floor((PAGE_HEIGHT_PT - TEXT_MARGIN_PT * 2) / TEXT_LINE_HEIGHT),
  );

  const pages: RawPage[] = [];
  for (let start = 0; start < Math.max(lines.length, 1); start += linesPerPage) {
    const slice = lines.slice(start, start + linesPerPage);
    pages.push({
      widthPt: PAGE_WIDTH_PT,
      heightPt: PAGE_HEIGHT_PT,
      async render(widthPx) {
        const canvas = document.createElement("canvas");
        canvas.width = widthPx;
        canvas.height = Math.max(1, Math.round((widthPx * PAGE_HEIGHT_PT) / PAGE_WIDTH_PT));
        const ctx = context2d(canvas);
        const scale = widthPx / PAGE_WIDTH_PT;
        ctx.fillStyle = "#ffffff";
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.fillStyle = "#111111";
        ctx.font = `${TEXT_FONT_SIZE * scale}px sans-serif`;
        ctx.textBaseline = "top";
        slice.forEach((line, index) => {
          ctx.fillText(
            line,
            TEXT_MARGIN_PT * scale,
            (TEXT_MARGIN_PT + index * TEXT_LINE_HEIGHT) * scale,
          );
        });
        return canvas;
      },
    });
  }
  return { name, pages };
}

async function loadTextDocument(file: File): Promise<LoadedDocument> {
  return textDocument(file.name, await file.text());
}

async function loadDocxDocument(file: File): Promise<LoadedDocument> {
  const zip = await JSZip.loadAsync(file);
  const entry = zip.file("word/document.xml");
  if (!entry) throw new Error("该 Word 文档缺少正文内容");
  const xml = await entry.async("string");
  const parsed = new DOMParser().parseFromString(xml, "application/xml");
  const paragraphs = Array.from(parsed.getElementsByTagName("w:p")).map((node) =>
    Array.from(node.getElementsByTagName("w:t"))
      .map((part) => part.textContent ?? "")
      .join(""),
  );
  return textDocument(file.name, paragraphs.join("\n"));
}

export async function loadDocument(file: File): Promise<LoadedDocument> {
  const name = file.name.toLowerCase();
  if (file.type === "application/pdf" || name.endsWith(".pdf")) {
    return loadPdfDocument(file);
  }
  if (file.type.startsWith("image/")) {
    return loadImageDocument(file);
  }
  if (name.endsWith(".docx") || file.type.includes("wordprocessingml")) {
    return loadDocxDocument(file);
  }
  if (TEXT_EXTENSIONS.test(name) || file.type.startsWith("text/")) {
    return loadTextDocument(file);
  }
  throw new Error(`暂不支持该格式，请使用 ${SUPPORTED_INPUT_LABEL}`);
}
