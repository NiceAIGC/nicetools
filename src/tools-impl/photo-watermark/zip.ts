// 批量打包下载：移植自源项目 downloadAll() 的分片、进度、ETA 与取消逻辑，
// 压缩部分改用 JSZip（原项目同样是 JSZip，仅由 CDN 改为本地依赖）。
import JSZip from "jszip";
import { canvasToBlob, generateZipName } from "./watermark";

export interface ZipSource {
  canvas: HTMLCanvasElement;
  fileName: string;
}

export interface ZipProgress {
  percent: number;
  text: string;
  eta: string;
}

export class DownloadCancelledError extends Error {
  constructor() {
    super("用户取消下载");
    this.name = "DownloadCancelledError";
  }
}

/** 根据设备内存与核心数自适应批次大小（3-10）。 */
function determineChunkSize(): number {
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory ?? 4;
  const cores = navigator.hardwareConcurrency || 4;
  const base = Math.floor(Math.min(memory, cores) * 1.5);
  return Math.max(3, Math.min(base, 10));
}

function formatDuration(ms: number): string {
  return ms < 60000 ? `${Math.ceil(ms / 1000)}秒` : `${Math.ceil(ms / 60000)}分钟`;
}

export function saveBlob(blob: Blob, fileName: string): void {
  const link = document.createElement("a");
  link.href = URL.createObjectURL(blob);
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(link.href);
}

/** 生成 zip 并直接触发下载；取消时抛出 DownloadCancelledError。 */
export async function downloadZip(
  sources: ZipSource[],
  handlers: { onProgress?: (progress: ZipProgress) => void; signal?: AbortSignal } = {},
): Promise<void> {
  const { onProgress, signal } = handlers;
  const total = sources.length;
  if (total === 0) return;

  const zip = new JSZip();
  const chunkSize = determineChunkSize();
  const startTime = Date.now();
  let completed = 0;

  const throwIfAborted = () => {
    if (signal?.aborted) throw new DownloadCancelledError();
  };

  for (let offset = 0; offset < total; offset += chunkSize) {
    const chunk = sources.slice(offset, offset + chunkSize);
    await Promise.all(
      chunk.map(async (source) => {
        throwIfAborted();
        const blob = await canvasToBlob(source.canvas);
        throwIfAborted();
        zip.file(`${source.fileName.replace(/\.[^/.]+$/, "")}.png`, blob);
        completed += 1;

        const percent = Number(((completed / total) * 100).toFixed(1));
        const remaining = total - completed;
        const rate = completed / (Date.now() - startTime);
        const eta = remaining > 0 && rate > 0 ? `预计剩余时间: ${formatDuration(remaining / rate)}` : "";
        onProgress?.({
          percent,
          text: `正在处理: ${completed}/${total} (${percent}%)`,
          eta,
        });
      }),
    );
  }

  throwIfAborted();
  onProgress?.({ percent: 0, text: "正在生成压缩包...", eta: "" });

  const content = await zip.generateAsync(
    { type: "blob", compression: "DEFLATE", compressionOptions: { level: 6 } },
    (metadata) => {
      throwIfAborted();
      const percent = Number(metadata.percent.toFixed(1));
      onProgress?.({ percent, text: `正在压缩: ${percent}%`, eta: "" });
    },
  );

  throwIfAborted();
  saveBlob(content, generateZipName());
}
