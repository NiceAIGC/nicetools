import { useCallback, useEffect, useRef, useState, type DragEvent, type ReactNode } from "react";
import {
  addToast,
  Button,
  Card,
  CardBody,
  Code,
  Divider,
  Kbd,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  NumberInput,
  Progress,
  Select,
  SelectItem,
  Slider,
} from "@heroui/react";

import {
  MAX_WATERMARK_TEXT,
  canvasToBlob,
  defaultWatermarkOptions,
  drawWatermark,
  generateFileName,
  isSupportedImage,
  supportedImageFormatLabel,
  watermarkFonts,
  watermarkPatterns,
  watermarkPositions,
  watermarkTemplates,
  type WatermarkOptions,
  type WatermarkPattern,
  type WatermarkPosition,
} from "./watermark";
import {
  DownloadCancelledError,
  downloadZip,
  saveBlob,
  type ZipProgress,
  type ZipSource,
} from "./zip";

interface WatermarkImage {
  id: string;
  fileName: string;
  img: HTMLImageElement;
}

interface UploadState {
  percent: number;
  text: string;
  failed: boolean;
}

const shortcuts: { keys: string; description: string }[] = [
  { keys: "⌘/Ctrl + D", description: "下载全部图片" },
  { keys: "⌘/Ctrl + V", description: "粘贴图片" },
  { keys: "Delete", description: "删除全部图片" },
  { keys: "⌘/Ctrl + Z", description: "撤销删除" },
  { keys: "Esc", description: "关闭预览" },
  { keys: "←/→", description: "预览时切换图片" },
  { keys: "1-9", description: "快速调整水印大小" },
];

// 单值滑块的 onChange 可能给出数组，这里统一取值。
function sliderValue(value: number | number[]): number {
  return Array.isArray(value) ? value[0] : value;
}

function createId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

// 水印绘制在卡片内的真实画布上，导出/预览统一从这里取同一份画布。
type CanvasLookup = Map<string, HTMLCanvasElement>;

function Section({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <Card shadow="sm" className="min-w-0 border border-default-200">
      <CardBody className="flex flex-col gap-3 p-4 md:flex-row md:items-start md:gap-6">
        <div className="md:w-44 md:shrink-0">
          <p className="text-sm font-semibold text-foreground">{label}</p>
          {hint ? <p className="mt-1 text-xs text-default-400">{hint}</p> : null}
        </div>
        <div className="min-w-0 flex-1">{children}</div>
      </CardBody>
    </Card>
  );
}

function ImageCard({
  item,
  options,
  index,
  isDragging,
  dropSide,
  registerCanvas,
  onPreview,
  onCopy,
  onDownload,
  onDelete,
  onDragStart,
  onDragEnd,
}: {
  item: WatermarkImage;
  options: WatermarkOptions;
  index: number;
  isDragging: boolean;
  dropSide: "before" | "after" | null;
  registerCanvas: (id: string, canvas: HTMLCanvasElement | null) => void;
  onPreview: () => void;
  onCopy: () => void;
  onDownload: () => void;
  onDelete: () => void;
  onDragStart: () => void;
  onDragEnd: () => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const registerCanvasRef = useCallback(
    (node: HTMLCanvasElement | null) => {
      canvasRef.current = node;
      registerCanvas(item.id, node);
    },
    [item.id, registerCanvas],
  );

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    drawWatermark(canvas, item.img, options);
  }, [item, options]);

  return (
    <Card
      shadow="sm"
      draggable
      data-image-index={index}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      className={`min-w-0 border border-default-200 transition-all ${
        dropSide === "before"
          ? "border-l-4 border-l-primary"
          : dropSide === "after"
            ? "border-r-4 border-r-primary"
            : ""
      } ${isDragging ? "opacity-50" : ""}`}
    >
      <CardBody className="gap-3 p-3">
        <canvas
          ref={registerCanvasRef}
          width={item.img.naturalWidth}
          height={item.img.naturalHeight}
          title="点击预览"
          onClick={onPreview}
          className="h-auto w-full cursor-zoom-in rounded-small border border-default-200 bg-default-100 transition-transform hover:scale-[1.02]"
        />
        <Divider />
        <p className="truncate text-xs text-default-500" title={item.fileName}>
          {item.fileName}
        </p>
        <div className="flex gap-2">
          <Button size="sm" variant="flat" className="flex-1" onPress={onCopy}>
            复制
          </Button>
          <Button size="sm" color="primary" className="flex-1" onPress={onDownload}>
            下载
          </Button>
          <Button size="sm" color="danger" variant="flat" className="flex-1" onPress={onDelete}>
            删除
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}

export default function PhotoWatermark() {
  const [options, setOptions] = useState<WatermarkOptions>(defaultWatermarkOptions);
  const [items, setItems] = useState<WatermarkImage[]>([]);
  const [upload, setUpload] = useState<UploadState | null>(null);
  const [isFileDragOver, setIsFileDragOver] = useState(false);
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<{ index: number; side: "before" | "after" } | null>(
    null,
  );
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [preview, setPreview] = useState<{ src: string; sharp: boolean } | null>(null);
  const [zip, setZip] = useState<ZipProgress | null>(null);
  const [zipFailed, setZipFailed] = useState(false);
  const [confirmKind, setConfirmKind] = useState<"delete" | "cancel" | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const lastDeletedRef = useRef<WatermarkImage | null>(null);
  const retryCountRef = useRef(0);
  const touchStartRef = useRef(0);
  const canvasLookupRef = useRef<CanvasLookup>(new Map());

  const registerCanvas = useCallback((id: string, canvas: HTMLCanvasElement | null) => {
    if (canvas) canvasLookupRef.current.set(id, canvas);
    else canvasLookupRef.current.delete(id);
  }, []);

  function notifyUnsupported() {
    addToast({
      title: "不支持的图片格式",
      description: `支持的格式：${supportedImageFormatLabel}`,
      color: "danger",
    });
  }

  function readFile(file: File) {
    setUpload({ percent: 0, text: `准备处理 ${file.name}...`, failed: false });

    const reader = new FileReader();

    reader.onprogress = (event) => {
      if (!event.lengthComputable) return;
      const percent = Math.round((event.loaded / event.total) * 100);
      const loaded = (event.loaded / 1024 / 1024).toFixed(1);
      const total = (event.total / 1024 / 1024).toFixed(1);
      setUpload({
        percent,
        text: `正在处理 ${file.name}: ${percent}% (${loaded}MB/${total}MB)`,
        failed: false,
      });
    };

    reader.onerror = () => {
      setUpload({ percent: 100, text: `${file.name} 读取失败`, failed: true });
      window.setTimeout(() => setUpload(null), 2000);
    };

    reader.onload = () => {
      const img = new Image();
      img.onload = () => {
        setItems((previous) => [...previous, { id: createId(), fileName: file.name, img }]);
        setUpload({ percent: 100, text: `${file.name} 处理完成`, failed: false });
        window.setTimeout(() => setUpload(null), 1000);
      };
      img.onerror = () => {
        setUpload({ percent: 100, text: `${file.name} 处理失败`, failed: true });
        window.setTimeout(() => setUpload(null), 2000);
      };
      img.src = String(reader.result);
    };

    reader.readAsDataURL(file);
  }

  function addFiles(files: File[]) {
    files.forEach((file) => {
      if (!isSupportedImage(file)) {
        notifyUnsupported();
        return;
      }
      readFile(file);
    });
  }

  function addFilesFromDataTransfer(dataTransfer: DataTransfer) {
    const files = Array.from(dataTransfer.files);
    if (files.length > 0) addFiles(files);
  }

  function removeItem(item: WatermarkImage) {
    const index = items.findIndex((entry) => entry.id === item.id);
    if (index < 0) return;
    lastDeletedRef.current = item;
    setItems((previous) => previous.filter((entry) => entry.id !== item.id));
    if (previewIndex !== null) {
      if (previewIndex === index) setPreviewIndex(null);
      else if (previewIndex > index) setPreviewIndex(previewIndex - 1);
    }
    addToast({ title: "图片已删除，按 Ctrl/⌘+Z 可恢复" });
  }

  function undoDelete() {
    const last = lastDeletedRef.current;
    if (!last) return;
    lastDeletedRef.current = null;
    setItems((previous) =>
      previous.some((entry) => entry.id === last.id) ? previous : [...previous, last],
    );
    addToast({ title: "已恢复上一次删除的图片", color: "success" });
  }

  function requestDeleteAll() {
    if (items.length === 0) {
      addToast({ title: "没有可删除的图片", color: "warning" });
      return;
    }
    setConfirmKind("delete");
  }

  function deleteAll() {
    lastDeletedRef.current = null;
    setItems([]);
    setPreviewIndex(null);
    setConfirmKind(null);
  }

  function switchPreview(direction: number) {
    setPreviewIndex((current) => {
      if (current === null || items.length === 0) return current;
      return (current + direction + items.length) % items.length;
    });
  }

  async function copyItem(item: WatermarkImage) {
    const canvas = canvasLookupRef.current.get(item.id);
    if (!canvas) return;
    if (typeof ClipboardItem === "undefined" || !navigator.clipboard?.write) {
      addToast({ title: "您的浏览器不支持此功能", color: "danger" });
      return;
    }
    try {
      const blob = await canvasToBlob(canvas);
      await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
      addToast({ title: "图片已复制到剪贴板", color: "success" });
    } catch (error) {
      console.error("复制失败:", error);
      addToast({ title: "复制失败，请重试", color: "danger" });
    }
  }

  async function downloadItem(item: WatermarkImage) {
    const canvas = canvasLookupRef.current.get(item.id);
    if (!canvas) return;
    try {
      const blob = await canvasToBlob(canvas);
      saveBlob(blob, generateFileName(item.fileName));
    } catch (error) {
      console.error("下载失败:", error);
      addToast({ title: "下载失败，请重试", color: "danger" });
    }
  }

  async function downloadAll() {
    const sources: ZipSource[] = [];
    for (const item of items) {
      const canvas = canvasLookupRef.current.get(item.id);
      if (canvas) sources.push({ canvas, fileName: item.fileName });
    }

    if (sources.length === 0) {
      addToast({ title: "没有可下载的图片", color: "warning" });
      return;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    setZipFailed(false);
    setZip({ percent: 0, text: "准备下载...", eta: "" });

    try {
      await downloadZip(sources, {
        signal: controller.signal,
        onProgress: (progress) => setZip(progress),
      });
      retryCountRef.current = 0;
      setZip(null);
      addToast({ title: "下载完成！", color: "success" });
    } catch (error) {
      if (error instanceof DownloadCancelledError) {
        setZip(null);
        addToast({ title: "已取消下载" });
        return;
      }
      console.error("下载失败:", error);
      if (retryCountRef.current < 3) {
        setZipFailed(true);
        setZip((previous) => ({ ...(previous ?? { percent: 0, eta: "" }), text: "下载失败" }));
      } else {
        setZip(null);
        setZipFailed(false);
        addToast({ title: "下载失败，请重试", color: "danger" });
      }
    } finally {
      abortRef.current = null;
    }
  }

  function cancelDownload() {
    setConfirmKind(null);
    abortRef.current?.abort();
  }

  function retryDownload() {
    retryCountRef.current += 1;
    setZipFailed(false);
    void downloadAll();
  }

  function requestPattern(pattern: WatermarkPattern) {
    setOptions((previous) => ({ ...previous, pattern }));
  }

  function requestPosition(position: WatermarkPosition) {
    setOptions((previous) => ({ ...previous, position }));
  }

  // 预览：先用缩略图占位，再异步换成高质量图（与源项目一致的渐进式预览）
  useEffect(() => {
    if (previewIndex === null) {
      setPreview(null);
      return;
    }
    const item = items[previewIndex];
    const canvas = item ? canvasLookupRef.current.get(item.id) : undefined;
    if (!item || !canvas) {
      setPreviewIndex(null);
      return;
    }

    const scale = Math.min(1, 800 / Math.max(canvas.width, canvas.height));
    const thumbnail = document.createElement("canvas");
    thumbnail.width = Math.max(1, Math.round(canvas.width * scale));
    thumbnail.height = Math.max(1, Math.round(canvas.height * scale));
    thumbnail.getContext("2d")?.drawImage(canvas, 0, 0, thumbnail.width, thumbnail.height);
    setPreview({ src: thumbnail.toDataURL("image/jpeg", 0.5), sharp: false });

    let objectUrl: string | null = null;
    let cancelled = false;
    canvas.toBlob(
      (blob) => {
        if (cancelled || !blob) return;
        objectUrl = URL.createObjectURL(blob);
        setPreview({ src: objectUrl, sharp: true });
      },
      "image/jpeg",
      0.92,
    );

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [previewIndex, items]);

  // 键盘快捷键：不设依赖数组，保证回调始终读取最新状态
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)
      ) {
        return;
      }

      const withModifier = event.ctrlKey || event.metaKey;

      if (withModifier && event.key.toLowerCase() === "d") {
        event.preventDefault();
        void downloadAll();
        return;
      }
      if (event.key === "Delete") {
        event.preventDefault();
        requestDeleteAll();
        return;
      }
      if (event.key === "Escape") {
        if (previewIndex !== null) {
          event.preventDefault();
          setPreviewIndex(null);
        }
        return;
      }
      if (previewIndex !== null && (event.key === "ArrowLeft" || event.key === "ArrowRight")) {
        event.preventDefault();
        switchPreview(event.key === "ArrowLeft" ? -1 : 1);
        return;
      }
      if (!withModifier && !event.altKey && !event.shiftKey && /^[1-9]$/.test(event.key)) {
        event.preventDefault();
        setOptions((previous) => ({ ...previous, size: Number(event.key) / 3 }));
        return;
      }
      if (withModifier && event.key.toLowerCase() === "z") {
        event.preventDefault();
        undoDelete();
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  // 粘贴上传：仅在非输入状态下拦截，避免影响文本框正常粘贴
  useEffect(() => {
    function onPaste(event: ClipboardEvent) {
      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)
      ) {
        return;
      }

      const clipboardItems = event.clipboardData?.items;
      if (!clipboardItems) return;

      const files: File[] = [];
      for (const entry of clipboardItems) {
        if (!entry.type.startsWith("image/")) continue;
        const file = entry.getAsFile();
        if (file) files.push(file);
      }
      if (files.length === 0) return;

      event.preventDefault();
      addFiles(files);
    }

    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  });

  function handleGridDragOver(event: DragEvent<HTMLDivElement>) {
    if (dragIndex === null) return;
    event.preventDefault();
    const card = (event.target as HTMLElement).closest<HTMLElement>("[data-image-index]");
    if (!card) return;
    const index = Number(card.dataset.imageIndex);
    const rect = card.getBoundingClientRect();
    const side = event.clientX < rect.left + rect.width / 2 ? "before" : "after";
    setDropTarget((previous) =>
      previous && previous.index === index && previous.side === side ? previous : { index, side },
    );
  }

  function handleGridDrop(event: DragEvent<HTMLDivElement>) {
    if (dragIndex === null) return;
    event.preventDefault();
    event.stopPropagation();

    const target = dropTarget;
    const from = dragIndex;
    setDragIndex(null);
    setDropTarget(null);
    if (!target) return;

    const insertAt = target.index + (target.side === "after" ? 1 : 0);
    const destination = from < insertAt ? insertAt - 1 : insertAt;
    if (destination === from) return;

    setItems((previous) => {
      const next = [...previous];
      const [moved] = next.splice(from, 1);
      next.splice(destination, 0, moved);
      return next;
    });
  }

  const previewItem = previewIndex === null ? null : items[previewIndex];

  return (
    <div
      className="flex min-w-0 flex-col gap-4"
      onDragOver={(event) => {
        if (!event.dataTransfer.types.includes("Files")) return;
        event.preventDefault();
        setIsFileDragOver(true);
      }}
      onDragLeave={() => setIsFileDragOver(false)}
      onDrop={(event) => {
        if (event.dataTransfer.files.length === 0) return;
        event.preventDefault();
        setIsFileDragOver(false);
        addFilesFromDataTransfer(event.dataTransfer);
      }}
    >
      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardBody className="gap-3 p-4">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm font-semibold text-foreground">键盘快捷键</p>
            <span className="text-xs text-default-400">
              全程在浏览器本地处理，图片不会上传到任何服务器
            </span>
          </div>
          <div className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
            {shortcuts.map((shortcut) => (
              <div key={shortcut.keys} className="flex items-center gap-2 text-xs">
                <Kbd className="shrink-0">{shortcut.keys}</Kbd>
                <span className="text-default-500">{shortcut.description}</span>
              </div>
            ))}
          </div>
        </CardBody>
      </Card>

      <Section label="第一步：选择图片上传" hint={`支持 ${supportedImageFormatLabel} 格式`}>
        <div className="flex flex-col gap-3">
          <label
            htmlFor="photo-watermark-file"
            onDragOver={(event) => {
              event.preventDefault();
              setIsFileDragOver(true);
            }}
            onDragLeave={() => setIsFileDragOver(false)}
            onDrop={(event) => {
              event.preventDefault();
              event.stopPropagation();
              setIsFileDragOver(false);
              addFilesFromDataTransfer(event.dataTransfer);
            }}
            className={`flex min-h-[140px] cursor-pointer flex-col items-center justify-center gap-2 rounded-large border-2 border-dashed p-6 text-center transition-colors ${
              isFileDragOver
                ? "border-primary bg-primary-50 dark:bg-primary-50/10"
                : "border-default-300 bg-default-50 hover:border-primary hover:bg-default-100"
            }`}
          >
            <span className="text-3xl" aria-hidden>
              📁
            </span>
            <span className="text-sm text-foreground">点击或拖拽图片到此处上传</span>
            <span className="text-xs text-default-400">
              支持 {supportedImageFormatLabel} 格式，也可直接粘贴图片
            </span>
          </label>
          <input
            id="photo-watermark-file"
            type="file"
            multiple
            autoComplete="off"
            className="hidden"
            onChange={(event) => {
              addFiles(Array.from(event.target.files ?? []));
              event.target.value = "";
            }}
          />
          {upload ? (
            <div className="flex flex-col gap-1">
              <Progress
                aria-label="图片处理进度"
                value={upload.percent}
                size="sm"
                color={upload.failed ? "danger" : "primary"}
                showValueLabel={false}
              />
              <p className="text-center text-xs text-default-500">{upload.text}</p>
            </div>
          ) : null}
        </div>
      </Section>

      <Section label="第二步：输入需要打水印的文字">
        <div className="flex min-w-0 flex-col gap-3">
          <input
            type="text"
            aria-label="水印文字"
            value={options.text}
            maxLength={MAX_WATERMARK_TEXT}
            spellCheck={false}
            autoComplete="off"
            placeholder="请输入文字"
            onChange={(event) => {
              const text = event.target.value;
              setOptions((previous) => ({ ...previous, text }));
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                event.currentTarget.blur();
              }
            }}
            className="h-10 w-full min-w-0 rounded-medium border border-default-200 bg-default-50 px-3 text-sm text-foreground outline-none transition-colors focus:border-primary"
          />
          <div className="flex flex-wrap gap-2">
            {watermarkTemplates.map((template) => (
              <Button
                key={template.label}
                size="sm"
                variant="flat"
                onPress={() => setOptions((previous) => ({ ...previous, text: template.text }))}
              >
                {template.label}
              </Button>
            ))}
          </div>
        </div>
      </Section>

      <Section label="水印布局设置">
        <div className="flex flex-wrap gap-6">
          <Select
            aria-label="布局方式"
            label="布局方式"
            labelPlacement="outside"
            className="w-[200px]"
            disallowEmptySelection
            selectedKeys={[options.pattern]}
            onSelectionChange={(keys) => {
              const [key] = Array.from(keys as Iterable<string>);
              if (key) requestPattern(key as WatermarkPattern);
            }}
          >
            {watermarkPatterns.map((pattern) => (
              <SelectItem key={pattern.key}>{pattern.label}</SelectItem>
            ))}
          </Select>

          {options.pattern === "custom" ? (
            <NumberInput
              aria-label="水印数量"
              label="水印数量"
              labelPlacement="outside"
              className="w-[140px]"
              minValue={1}
              maxValue={100}
              value={options.count}
              onValueChange={(value) => {
                if (!Number.isFinite(value)) return;
                setOptions((previous) => ({ ...previous, count: Math.min(100, Math.max(1, value)) }));
              }}
            />
          ) : null}

          {options.pattern === "single" ? (
            <Select
              aria-label="位置"
              label="位置"
              labelPlacement="outside"
              className="w-[160px]"
              disallowEmptySelection
              selectedKeys={[options.position]}
              onSelectionChange={(keys) => {
                const [key] = Array.from(keys as Iterable<string>);
                if (key) requestPosition(key as WatermarkPosition);
              }}
            >
              {watermarkPositions.map((position) => (
                <SelectItem key={position.key}>{position.label}</SelectItem>
              ))}
            </Select>
          ) : null}
        </div>
      </Section>

      <Section label="字体与样式">
        <div className="flex flex-wrap items-center gap-6">
          <Select
            aria-label="字体"
            label="字体"
            labelPlacement="outside"
            className="w-[200px]"
            disallowEmptySelection
            selectedKeys={[options.fontFamily]}
            onSelectionChange={(keys) => {
              const [key] = Array.from(keys as Iterable<string>);
              if (key) setOptions((previous) => ({ ...previous, fontFamily: key }));
            }}
          >
            {watermarkFonts.map((font) => (
              <SelectItem key={font}>{font}</SelectItem>
            ))}
          </Select>

          <div className="flex flex-col gap-1.5">
            <span className="text-small text-foreground">字形</span>
            <div className="flex gap-1">
              <Button
                size="sm"
                isIconOnly
                aria-label="加粗"
                aria-pressed={options.bold}
                variant={options.bold ? "solid" : "flat"}
                color={options.bold ? "primary" : "default"}
                className="font-bold"
                onPress={() => setOptions((previous) => ({ ...previous, bold: !previous.bold }))}
              >
                B
              </Button>
              <Button
                size="sm"
                isIconOnly
                aria-label="斜体"
                aria-pressed={options.italic}
                variant={options.italic ? "solid" : "flat"}
                color={options.italic ? "primary" : "default"}
                className="italic"
                onPress={() =>
                  setOptions((previous) => ({ ...previous, italic: !previous.italic }))
                }
              >
                I
              </Button>
            </div>
          </div>
        </div>
      </Section>

      <Section label="水印外观" hint="颜色、透明度、间距、字号与旋转角度">
        <div className="flex flex-wrap items-start gap-6">
          <div className="flex w-[180px] flex-col gap-1.5">
            <span className="text-small text-foreground">颜色</span>
            <div className="flex items-center gap-3">
              <input
                type="color"
                aria-label="水印颜色"
                value={options.color}
                onChange={(event) =>
                  setOptions((previous) => ({ ...previous, color: event.target.value }))
                }
                className="h-10 w-16 cursor-pointer rounded-medium border border-default-200 bg-transparent p-1"
              />
              <Code size="sm">{options.color.toUpperCase()}</Code>
            </div>
          </div>

          <Slider
            aria-label="不透明度"
            label="不透明度"
            className="w-[200px]"
            minValue={0}
            maxValue={1}
            step={0.05}
            value={options.alpha}
            onChange={(value) =>
              setOptions((previous) => ({ ...previous, alpha: sliderValue(value) }))
            }
          />

          <Slider
            aria-label="间隔"
            label="间隔"
            className="w-[200px]"
            minValue={1}
            maxValue={8}
            step={0.2}
            value={options.space}
            onChange={(value) =>
              setOptions((previous) => ({ ...previous, space: sliderValue(value) }))
            }
          />

          <Slider
            aria-label="字号"
            label="字号"
            className="w-[200px]"
            minValue={0.5}
            maxValue={3}
            step={0.05}
            value={options.size}
            onChange={(value) =>
              setOptions((previous) => ({ ...previous, size: sliderValue(value) }))
            }
          />

          <Slider
            aria-label="旋转角度"
            label="旋转角度"
            className="w-[220px]"
            minValue={-90}
            maxValue={90}
            step={3}
            value={options.rotate}
            renderValue={() => `${options.rotate}°`}
            onChange={(value) =>
              setOptions((previous) => ({ ...previous, rotate: sliderValue(value) }))
            }
          />
        </div>
      </Section>

      <Section label="第三步：点击图片进行单个图片下载" hint="卡片可拖拽调整顺序">
        <div className="flex flex-wrap items-center gap-3">
          <Button color="primary" isLoading={zip !== null && !zipFailed} onPress={downloadAll}>
            下载全部
          </Button>
          <Button color="danger" variant="flat" onPress={requestDeleteAll}>
            删除全部
          </Button>
          {items.length > 0 ? (
            <span className="text-xs text-default-400">共 {items.length} 张图片</span>
          ) : null}
        </div>

        {zip ? (
          <div className="mt-3 flex flex-col gap-2 rounded-large border border-default-200 p-3">
            <Progress
              aria-label="打包下载进度"
              value={zip.percent}
              size="sm"
              color={zipFailed ? "danger" : "primary"}
              showValueLabel={false}
            />
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-xs text-default-500">{zip.text}</span>
              <span className="text-xs text-default-400">{zip.eta}</span>
              <div className="flex gap-2">
                {zipFailed ? (
                  <Button size="sm" variant="flat" onPress={retryDownload}>
                    重试
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="flat"
                    color="danger"
                    onPress={() => setConfirmKind("cancel")}
                  >
                    取消
                  </Button>
                )}
              </div>
            </div>
          </div>
        ) : null}
      </Section>

      <div
        onDragOver={(event) => {
          handleGridDragOver(event);
        }}
        onDragLeave={() => {
          setDropTarget(null);
        }}
        onDrop={(event) => {
          handleGridDrop(event);
        }}
        className={`grid min-h-[200px] grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4 rounded-large border-2 border-dashed p-4 transition-colors ${
          isFileDragOver && dragIndex === null
            ? "border-primary bg-primary-50 dark:bg-primary-50/10"
            : "border-default-300 bg-default-50"
        }`}
      >
        {items.length === 0 ? (
          <div className="col-span-full flex min-h-[180px] items-center justify-center text-sm text-default-400">
            拖拽图片到此处上传
          </div>
        ) : (
          items.map((item, index) => (
            <ImageCard
              key={item.id}
              item={item}
              options={options}
              index={index}
              isDragging={dragIndex === index}
              dropSide={dropTarget?.index === index ? dropTarget.side : null}
              registerCanvas={registerCanvas}
              onPreview={() => setPreviewIndex(index)}
              onCopy={() => void copyItem(item)}
              onDownload={() => void downloadItem(item)}
              onDelete={() => removeItem(item)}
              onDragStart={() => setDragIndex(index)}
              onDragEnd={() => {
                setDragIndex(null);
                setDropTarget(null);
              }}
            />
          ))
        )}
      </div>

      <Card shadow="sm" className="min-w-0 border border-default-200">
        <CardBody className="gap-2 p-4 text-xs text-default-500">
          <p className="text-sm font-semibold text-foreground">隐私说明</p>
          <p>
            所有水印绘制、打码与打包下载都在浏览器本地完成，图片不会上传到任何服务器，也不会保存任何用户数据。
          </p>
          <p>
            若水印文字重叠、角度或间距不合适，可调整「间隔」「字号」「旋转角度」实时预览效果；处理敏感证件（身份证、驾照、护照）时建议使用「单个水印 + 左上角/居中」。
          </p>
        </CardBody>
      </Card>

      <Modal
        isOpen={previewItem !== null}
        onOpenChange={(open) => {
          if (!open) setPreviewIndex(null);
        }}
        size="5xl"
        backdrop="blur"
        aria-label="图片预览"
        classNames={{ base: "bg-transparent shadow-none", body: "p-0" }}
      >
        <ModalContent>
          <ModalBody>
            <div
              className="relative flex items-center justify-center"
              onTouchStart={(event) => {
                touchStartRef.current = event.touches[0]?.clientX ?? 0;
              }}
              onTouchEnd={(event) => {
                const delta = (event.changedTouches[0]?.clientX ?? 0) - touchStartRef.current;
                if (Math.abs(delta) > 50) switchPreview(delta > 0 ? -1 : 1);
              }}
            >
              {previewItem && preview ? (
                <img
                  src={preview.src}
                  alt={previewItem.fileName}
                  onClick={() => setPreviewIndex(null)}
                  className={`max-h-[80vh] max-w-full cursor-zoom-out rounded-medium object-contain transition-[filter] duration-200 ${
                    preview.sharp ? "" : "blur-sm"
                  }`}
                />
              ) : null}

              {items.length > 1 ? (
                <>
                  <Button
                    isIconOnly
                    aria-label="上一张"
                    variant="flat"
                    className="absolute left-2 top-1/2 -translate-y-1/2"
                    onPress={() => switchPreview(-1)}
                  >
                    ‹
                  </Button>
                  <Button
                    isIconOnly
                    aria-label="下一张"
                    variant="flat"
                    className="absolute right-2 top-1/2 -translate-y-1/2"
                    onPress={() => switchPreview(1)}
                  >
                    ›
                  </Button>
                </>
              ) : null}
            </div>
            <p className="mt-2 text-center text-xs text-default-400">
              {previewIndex === null ? "" : `${previewIndex + 1} / ${items.length}`} · Esc 或点击图片关闭 ·
              左右方向键切换
            </p>
          </ModalBody>
        </ModalContent>
      </Modal>

      <Modal
        isOpen={confirmKind !== null}
        onOpenChange={(open) => {
          if (!open) setConfirmKind(null);
        }}
        size="sm"
        aria-label="操作确认"
      >
        <ModalContent>
          <ModalHeader>{confirmKind === "cancel" ? "取消下载" : "删除全部图片"}</ModalHeader>
          <ModalBody>
            <p className="text-sm text-default-500">
              {confirmKind === "cancel"
                ? "确定要取消下载吗？已处理的图片不会保存。"
                : "确定要删除所有图片吗？"}
            </p>
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={() => setConfirmKind(null)}>
              取消
            </Button>
            <Button
              color="danger"
              onPress={() => {
                if (confirmKind === "cancel") cancelDownload();
                else deleteAll();
              }}
            >
              确定
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
