import { useEffect, useMemo, useRef, useState } from "react";
import {
  Accordion,
  AccordionItem,
  addToast,
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Progress,
  Select,
  SelectItem,
  Slider,
  Switch,
} from "@heroui/react";
import {
  buildPdf,
  loadSavedSettings,
  renderScanCanvas,
  renderScannedPage,
  saveSettings,
  SCAN_PRESETS,
  type PdfPageSpec,
  type ScanSettings,
} from "./scan";
import { loadDocument, SUPPORTED_INPUT_LABEL, type LoadedDocument } from "./readers";

const PRESET_LABELS: Record<string, string> = {
  soft: "轻度扫描",
  standard: "标准扫描",
  vintage: "复古泛黄",
  copy: "黑白复印",
  custom: "自定义",
};

const MODE_LABELS: Record<string, string> = {
  color: "彩色",
  gray: "灰度",
  bw: "黑白",
};

const DPI_OPTIONS = [
  { value: "96", label: "轻量 96 DPI" },
  { value: "150", label: "标准 150 DPI" },
  { value: "200", label: "高清 200 DPI" },
];

const ACCEPT_TYPES = ".pdf,image/*,.docx,.txt,.md,.markdown,.csv,.tsv,.json,.log,.text";
const ZOOM_STEP = 1.25;
const ZOOM_MIN = 25;
const ZOOM_MAX = 400;

function sliderValue(value: number | number[]): number {
  return Array.isArray(value) ? value[0] : value;
}

function pdfFileName(documents: LoadedDocument[]): string {
  const now = new Date();
  const pad = (value: number) => String(value).padStart(2, "0");
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  const source = (documents[0]?.name ?? "文档").replace(/\.[^.]+$/, "");
  const merged = documents.length > 1 ? `_等${documents.length}份` : "";
  return `扫描件${stamp}_${source}${merged}.pdf`;
}

function downloadPdf(pdf: Uint8Array<ArrayBuffer>, fileName: string): void {
  const url = URL.createObjectURL(new Blob([pdf], { type: "application/pdf" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export default function DocScan() {
  const [saved] = useState(loadSavedSettings);
  const [documents, setDocuments] = useState<LoadedDocument[]>([]);
  const [settings, setSettings] = useState<ScanSettings>(saved.settings);
  const [presetKey, setPresetKey] = useState(saved.presetKey);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [dragging, setDragging] = useState(false);
  const [zoomOpen, setZoomOpen] = useState(false);
  const [zoom, setZoom] = useState(100);
  const [zoomCanvas, setZoomCanvas] = useState<HTMLCanvasElement | null>(null);
  const previewRef = useRef<HTMLCanvasElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const pages = useMemo(() => documents.flatMap((doc) => doc.pages), [documents]);

  useEffect(() => {
    saveSettings({ settings, presetKey });
  }, [settings, presetKey]);

  useEffect(() => {
    const canvas = previewRef.current;
    const page = pages[0];
    if (!canvas || !page) return;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void renderScanCanvas(page, { ...settings, dpi: 72 }, 0)
        .then((rendered) => {
          if (cancelled) return;
          canvas.width = rendered.width;
          canvas.height = rendered.height;
          canvas.getContext("2d")?.drawImage(rendered, 0, 0);
        })
        .catch(() => undefined);
    }, 120);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [pages, settings]);

  useEffect(() => {
    if (!zoomOpen || !zoomCanvas || !pages[0]) return;
    let cancelled = false;
    const dpi = Math.min(Math.max(settings.dpi, 150), 200);
    void renderScanCanvas(pages[0], { ...settings, dpi }, 0)
      .then((rendered) => {
        if (cancelled) return;
        zoomCanvas.width = rendered.width;
        zoomCanvas.height = rendered.height;
        zoomCanvas.getContext("2d")?.drawImage(rendered, 0, 0);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [zoomOpen, zoomCanvas, pages, settings]);

  function update(patch: Partial<ScanSettings>) {
    setSettings((previous) => ({ ...previous, ...patch }));
    setPresetKey("custom");
  }

  async function addFiles(files: File[]) {
    for (const file of files) {
      try {
        const loaded = await loadDocument(file);
        setDocuments((previous) => [...previous, loaded]);
      } catch (error) {
        addToast({
          title: `无法读取 ${file.name}`,
          description: error instanceof Error ? error.message : String(error),
          color: "danger",
        });
      }
    }
  }

  function removeDocument(index: number) {
    documents[index]?.dispose?.();
    setDocuments((previous) => previous.filter((_, position) => position !== index));
  }

  function clearDocuments() {
    documents.forEach((doc) => doc.dispose?.());
    setDocuments([]);
    setProgress(0);
  }

  async function generate() {
    if (pages.length === 0 || busy) return;
    setBusy(true);
    setProgress(0);
    try {
      const specs: PdfPageSpec[] = [];
      for (let index = 0; index < pages.length; index += 1) {
        specs.push(await renderScannedPage(pages[index], settings, index));
        setProgress(Math.round(((index + 1) / pages.length) * 100));
      }
      downloadPdf(buildPdf(specs), pdfFileName(documents));
      addToast({ title: `已生成 ${specs.length} 页扫描件`, color: "success" });
    } catch (error) {
      addToast({
        title: "生成扫描件失败",
        description: error instanceof Error ? error.message : String(error),
        color: "danger",
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <Card
        shadow="sm"
        className={`border-2 border-dashed transition-colors ${
          dragging ? "border-primary bg-primary-50/40" : "border-default-300 bg-default-50"
        }`}
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(event) => {
          event.preventDefault();
          setDragging(false);
          void addFiles(Array.from(event.dataTransfer.files));
        }}
      >
        <CardBody className="flex-row items-center gap-3 py-3">
          <span className="shrink-0 text-xl" aria-hidden>📠</span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-foreground">
              把文档拖进来，或点击“选择文件”
            </p>
            <p
              className="truncate text-xs text-default-500"
              title={`支持 ${SUPPORTED_INPUT_LABEL}；全部在浏览器本地处理，文件不会上传`}
            >
              支持 {SUPPORTED_INPUT_LABEL} · 本地处理不上传
            </p>
          </div>
          <Button
            size="sm"
            color="primary"
            variant="flat"
            className="shrink-0"
            onPress={() => inputRef.current?.click()}
          >
            选择文件
          </Button>
          <input
            ref={inputRef}
            type="file"
            accept={ACCEPT_TYPES}
            multiple
            className="hidden"
            aria-label="选择待转换文档"
            onChange={(event) => {
              void addFiles(Array.from(event.target.files ?? []));
              event.target.value = "";
            }}
          />
        </CardBody>
      </Card>

      {documents.length > 0 && (
        <Card shadow="sm" className="border border-default-200">
          <CardHeader className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="font-semibold text-foreground">待转换文档</p>
              <p className="text-xs text-default-500">
                {documents.length} 个文件 · 共 {pages.length} 页，按添加顺序合并为一份 PDF
              </p>
            </div>
            <Button size="sm" variant="light" color="danger" onPress={clearDocuments}>
              清空列表
            </Button>
          </CardHeader>
          <CardBody className="gap-2">
            {documents.map((doc, index) => (
              <div
                key={`${doc.name}-${index}`}
                className="flex flex-wrap items-center justify-between gap-2 rounded-medium bg-default-50 px-3 py-2"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <Chip size="sm" variant="flat">{doc.pages.length} 页</Chip>
                  <p className="truncate text-sm text-default-700" title={doc.name}>
                    {doc.name}
                  </p>
                </div>
                <Button size="sm" variant="light" color="danger" onPress={() => removeDocument(index)}>
                  移除
                </Button>
              </div>
            ))}
          </CardBody>
        </Card>
      )}

      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader>
            <div>
              <p className="font-semibold text-foreground">扫描效果</p>
              <p className="text-xs text-default-500">套用旋转、噪点、泛黄等真实扫描质感</p>
            </div>
          </CardHeader>
          <CardBody className="gap-5">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Select
                label="预设"
                selectedKeys={[presetKey]}
                disallowEmptySelection
                onSelectionChange={(keys) => {
                  const next = Array.from(keys)[0] as string;
                  const preset = SCAN_PRESETS[next];
                  if (!preset) return;
                  setSettings(preset);
                  setPresetKey(next);
                }}
              >
                {Object.entries(PRESET_LABELS).map(([key, label]) => (
                  <SelectItem key={key}>{label}</SelectItem>
                ))}
              </Select>
              <Select
                label="色彩"
                selectedKeys={[settings.mode]}
                disallowEmptySelection
                onSelectionChange={(keys) =>
                  update({ mode: Array.from(keys)[0] as ScanSettings["mode"] })
                }
              >
                {Object.entries(MODE_LABELS).map(([key, label]) => (
                  <SelectItem key={key}>{label}</SelectItem>
                ))}
              </Select>
            </div>

            <Select
              label="输出清晰度"
              selectedKeys={[String(settings.dpi)]}
              disallowEmptySelection
              onSelectionChange={(keys) => update({ dpi: Number(Array.from(keys)[0]) })}
            >
              {DPI_OPTIONS.map((option) => (
                <SelectItem key={option.value}>{option.label}</SelectItem>
              ))}
            </Select>

            <Switch isSelected={settings.border} onValueChange={(value) => update({ border: value })}>
              添加扫描边框
            </Switch>

            <Accordion variant="bordered" selectionMode="single">
              <AccordionItem key="tune" aria-label="精细调节" title="精细调节">
                <div className="flex flex-col gap-5 pb-2">
                  <Slider
                    label={`旋转角度 ${settings.rotate.toFixed(1)}°`}
                    minValue={0}
                    maxValue={3}
                    step={0.1}
                    showTooltip
                    value={settings.rotate}
                    onChange={(value) => update({ rotate: sliderValue(value) })}
                  />
                  <Slider
                    label={`旋转随机 ${settings.rotateVariance.toFixed(1)}°`}
                    minValue={0}
                    maxValue={1}
                    step={0.1}
                    showTooltip
                    value={settings.rotateVariance}
                    onChange={(value) => update({ rotateVariance: sliderValue(value) })}
                  />
                  <Slider
                    label={`亮度 ${Math.round(settings.brightness * 100)}%`}
                    minValue={0.8}
                    maxValue={1.25}
                    step={0.01}
                    showTooltip
                    value={settings.brightness}
                    onChange={(value) => update({ brightness: sliderValue(value) })}
                  />
                  <Slider
                    label={`对比度 ${Math.round(settings.contrast * 100)}%`}
                    minValue={0.8}
                    maxValue={1.6}
                    step={0.01}
                    showTooltip
                    value={settings.contrast}
                    onChange={(value) => update({ contrast: sliderValue(value) })}
                  />
                  <Slider
                    label={`泛黄 ${Math.round(settings.yellowish * 100)}%`}
                    minValue={0}
                    maxValue={0.5}
                    step={0.01}
                    showTooltip
                    value={settings.yellowish}
                    onChange={(value) => update({ yellowish: sliderValue(value) })}
                  />
                  <Slider
                    label={`模糊 ${settings.blur.toFixed(2)}`}
                    minValue={0}
                    maxValue={1.5}
                    step={0.05}
                    showTooltip
                    value={settings.blur}
                    onChange={(value) => update({ blur: sliderValue(value) })}
                  />
                  <Slider
                    label={`噪点 ${Math.round(settings.noise * 100)}%`}
                    minValue={0}
                    maxValue={0.5}
                    step={0.01}
                    showTooltip
                    value={settings.noise}
                    onChange={(value) => update({ noise: sliderValue(value) })}
                  />
                </div>
              </AccordionItem>
            </Accordion>

            {busy && <Progress size="sm" color="primary" value={progress} showValueLabel />}
            <Button
              color="primary"
              isLoading={busy}
              isDisabled={pages.length === 0}
              onPress={() => void generate()}
            >
              生成扫描 PDF（{pages.length} 页）
            </Button>
          </CardBody>
        </Card>

        <Card shadow="sm" className="min-w-0 border border-default-200">
          <CardHeader className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="font-semibold text-foreground">效果预览</p>
              <p className="text-xs text-default-500">显示第一页，点击可放大查看细节</p>
            </div>
            {pages.length > 0 && (
              <Chip size="sm" color="primary" variant="flat">第 1 / {pages.length} 页</Chip>
            )}
          </CardHeader>
          <CardBody>
            {pages.length === 0 ? (
              <div className="flex min-h-[320px] flex-col items-center justify-center gap-2 rounded-medium border border-dashed border-default-300 text-default-500">
                <span className="text-3xl" aria-hidden>📄</span>
                <p className="text-sm">添加文档后这里会显示扫描效果</p>
              </div>
            ) : (
              <button
                type="button"
                aria-label="放大扫描效果预览"
                className="group flex w-full justify-center rounded-medium bg-default-100 p-3"
                onClick={() => {
                  setZoom(100);
                  setZoomOpen(true);
                }}
              >
                <canvas
                  ref={previewRef}
                  aria-hidden="true"
                  className="max-h-[560px] w-auto max-w-full rounded-small shadow-medium transition-opacity group-hover:opacity-90"
                />
              </button>
            )}
          </CardBody>
        </Card>
      </div>

      <Modal isOpen={zoomOpen} onOpenChange={setZoomOpen} size="5xl" scrollBehavior="inside">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-wrap items-center justify-between gap-3">
                <span>扫描效果预览</span>
                <Chip size="sm" variant="flat">{zoom}%</Chip>
              </ModalHeader>
              <ModalBody className="overflow-auto bg-default-100 p-4">
                <div className="w-full overflow-auto">
                  <canvas
                    ref={setZoomCanvas}
                    aria-label="放大后的扫描效果预览"
                    className="mx-auto block h-auto shadow-medium"
                    style={{ width: `${zoom}%`, maxWidth: "none" }}
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button
                  size="sm"
                  variant="flat"
                  isDisabled={zoom <= ZOOM_MIN}
                  onPress={() => setZoom((value) => Math.max(ZOOM_MIN, Math.round(value / ZOOM_STEP)))}
                >
                  缩小
                </Button>
                <Button size="sm" variant="flat" onPress={() => setZoom(100)}>适应宽度</Button>
                <Button
                  size="sm"
                  variant="flat"
                  isDisabled={zoom >= ZOOM_MAX}
                  onPress={() => setZoom((value) => Math.min(ZOOM_MAX, Math.round(value * ZOOM_STEP)))}
                >
                  放大
                </Button>
                <Button size="sm" color="primary" onPress={onClose}>完成</Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
