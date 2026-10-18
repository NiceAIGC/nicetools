import { useEffect, useRef, useState } from "react";
import {
  addToast,
  Button,
  Card,
  CardBody,
  Chip,
  Divider,
  Input,
  Select,
  SelectItem,
  Slider,
  Switch,
  Tab,
  Tabs,
} from "@heroui/react";

import { CARD_H, CARD_W, drawCard } from "./render";
import {
  bgPresets,
  colorPresets,
  defaultState,
  deleteSaved,
  exportFileName,
  fieldDefs,
  fontOptions,
  loadSaved,
  loadState,
  putSaved,
  saveState,
  sizeDefs,
  type CardState,
  type SavedCard,
} from "./model";

// HeroUI 没有取色器组件：预设色板用 Button 实现，自定义色用原生 type=color 兜底（唯一非 HeroUI 控件）。
function ColorField({
  label,
  presets,
  value,
  onChange,
}: {
  label: string;
  presets: string[];
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      <span className="text-sm text-foreground-600">{label}</span>
      <div className="flex flex-wrap items-center gap-2">
        {presets.map((preset) => (
          <Button
            key={preset}
            aria-label={`${label} ${preset}`}
            className={`h-7 w-7 min-w-7 rounded-full p-0 ${value.toLowerCase() === preset ? "ring-2 ring-primary ring-offset-2 ring-offset-background" : ""}`}
            style={{ backgroundColor: preset }}
            onPress={() => onChange(preset)}
          />
        ))}
        <input
          type="color"
          aria-label={`${label}自定义`}
          className="h-7 w-7 cursor-pointer rounded-full border border-default-200 bg-transparent p-0"
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      </div>
    </div>
  );
}

export default function BusinessCard() {
  const [state, setState] = useState<CardState>(loadState);
  const [saved, setSaved] = useState<SavedCard[]>(loadSaved);
  const [saveName, setSaveName] = useState("");
  const [format, setFormat] = useState<"png" | "jpeg">("png");
  const [scale, setScale] = useState(2);
  const canvasRef = useRef<HTMLCanvasElement>(null);

  // 预览与导出共用 drawCard，改任何配置立即重绘。
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas) drawCard(canvas, state, 1);
  }, [state]);

  // 编辑防抖自动保存到 localStorage。
  useEffect(() => {
    const timer = setTimeout(() => saveState(state), 300);
    return () => clearTimeout(timer);
  }, [state]);

  const setStyle = (patch: Partial<CardState["style"]>) =>
    setState((prev) => ({ ...prev, style: { ...prev.style, ...patch } }));

  const exportCard = () => {
    const canvas = document.createElement("canvas");
    drawCard(canvas, state, scale);
    canvas.toBlob(
      (blob) => {
        if (!blob) {
          addToast({ title: "导出失败，请重试", color: "danger" });
          return;
        }
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = exportFileName(state.fields.name, format);
        link.click();
        URL.revokeObjectURL(url);
        addToast({ title: "名片已导出" });
      },
      format === "png" ? "image/png" : "image/jpeg",
      0.92,
    );
  };

  return (
    <div className="flex flex-col gap-6 lg:flex-row">
      <Card className="min-w-0 flex-1">
        <CardBody className="gap-4">
          <div className="flex items-center justify-between gap-2">
            <Chip size="sm" variant="flat">
              {CARD_W} × {CARD_H}（90 × 54 mm）
            </Chip>
            <span className="text-xs text-default-400">修改自动保存在本机</span>
          </div>
          <canvas
            ref={canvasRef}
            className="w-full rounded-lg shadow-md"
            style={{ aspectRatio: `${CARD_W} / ${CARD_H}` }}
          />
          <div className="flex flex-wrap items-end gap-3">
            <Select
              label="导出格式"
              size="sm"
              className="w-36"
              selectedKeys={[format]}
              onChange={(event) => setFormat(event.target.value as "png" | "jpeg")}
            >
              <SelectItem key="png">PNG（无损）</SelectItem>
              <SelectItem key="jpeg">JPG</SelectItem>
            </Select>
            <Select
              label="导出倍率"
              size="sm"
              className="w-36"
              selectedKeys={[String(scale)]}
              onChange={(event) => setScale(Number(event.target.value))}
            >
              <SelectItem key="1">1×（1050px）</SelectItem>
              <SelectItem key="2">2×（2100px）</SelectItem>
              <SelectItem key="3">3×（3150px）</SelectItem>
            </Select>
            <Button color="primary" className="ml-auto" onPress={exportCard}>
              导出图片
            </Button>
          </div>
        </CardBody>
      </Card>

      <Card className="w-full shrink-0 lg:w-[400px]">
        <CardBody>
          <Tabs aria-label="名片配置" fullWidth>
            <Tab key="fields" title="内容">
              <div className="flex flex-col gap-3 pt-2">
                {fieldDefs.map((def) => (
                  <div key={def.key} className="flex items-center gap-2">
                    <Input
                      label={def.label}
                      placeholder={def.placeholder}
                      size="sm"
                      value={state.fields[def.key]}
                      onValueChange={(value) =>
                        setState((prev) => ({
                          ...prev,
                          fields: { ...prev.fields, [def.key]: value },
                        }))
                      }
                    />
                    <Switch
                      aria-label={`显示${def.label}`}
                      size="sm"
                      isSelected={state.show[def.key]}
                      onValueChange={(value) =>
                        setState((prev) => ({
                          ...prev,
                          show: { ...prev.show, [def.key]: value },
                        }))
                      }
                    />
                  </div>
                ))}
              </div>
            </Tab>

            <Tab key="style" title="样式">
              <div className="flex flex-col gap-4 pt-2">
                <Select
                  label="版式"
                  size="sm"
                  selectedKeys={[state.style.layout]}
                  onChange={(event) =>
                    setStyle({ layout: event.target.value as "center" | "left" })
                  }
                >
                  <SelectItem key="center">居中（水印回纹）</SelectItem>
                  <SelectItem key="left">左对齐</SelectItem>
                </Select>
                <Select
                  label="字体"
                  size="sm"
                  selectedKeys={[state.style.fontFamily]}
                  onChange={(event) => setStyle({ fontFamily: event.target.value })}
                >
                  {fontOptions.map((font) => (
                    <SelectItem key={font.stack}>{font.label}</SelectItem>
                  ))}
                </Select>
                <ColorField label="主色" presets={colorPresets} value={state.style.primary} onChange={(value) => setStyle({ primary: value })} />
                <ColorField label="背景色" presets={bgPresets} value={state.style.background} onChange={(value) => setStyle({ background: value })} />
                <ColorField label="次要文字色" presets={colorPresets} value={state.style.textColor} onChange={(value) => setStyle({ textColor: value })} />
                <Divider />
                {sizeDefs.map((def) => (
                  <Slider
                    key={def.key}
                    label={def.label}
                    size="sm"
                    minValue={def.min}
                    maxValue={def.max}
                    value={state.style[def.key]}
                    onChange={(value) =>
                      setStyle({ [def.key]: Array.isArray(value) ? value[0] : value } as Partial<CardState["style"]>)
                    }
                  />
                ))}
                <Slider
                  label="圆角"
                  size="sm"
                  minValue={0}
                  maxValue={40}
                  value={state.style.radius}
                  onChange={(value) => setStyle({ radius: Array.isArray(value) ? value[0] : value })}
                />
                <Switch
                  size="sm"
                  isSelected={state.style.border}
                  onValueChange={(value) => setStyle({ border: value })}
                >
                  描边
                </Switch>
              </div>
            </Tab>

            <Tab key="watermark" title="水印">
              <div className="flex flex-col gap-4 pt-2">
                <Switch
                  size="sm"
                  isSelected={state.style.wmEnabled}
                  onValueChange={(value) => setStyle({ wmEnabled: value })}
                >
                  启用回纹水印
                </Switch>
                <Input
                  label="水印文字"
                  placeholder="MINGSHU"
                  size="sm"
                  value={state.style.wmText}
                  onValueChange={(value) => setStyle({ wmText: value })}
                />
                <Slider label="水印字号" size="sm" minValue={20} maxValue={120} value={state.style.wmSize} onChange={(value) => setStyle({ wmSize: Array.isArray(value) ? value[0] : value })} />
                <Slider label="水印透明度" size="sm" minValue={0} maxValue={30} value={state.style.wmOpacity} onChange={(value) => setStyle({ wmOpacity: Array.isArray(value) ? value[0] : value })} />
                <Slider label="水印行距" size="sm" minValue={40} maxValue={200} value={state.style.wmGap} onChange={(value) => setStyle({ wmGap: Array.isArray(value) ? value[0] : value })} />
                <Slider label="水印角度" size="sm" minValue={-30} maxValue={30} value={state.style.wmAngle} onChange={(value) => setStyle({ wmAngle: Array.isArray(value) ? value[0] : value })} />
              </div>
            </Tab>

            <Tab key="saved" title="名片夹">
              <div className="flex flex-col gap-3 pt-2">
                <div className="flex items-end gap-2">
                  <Input
                    label="名称"
                    placeholder="如：张三名片"
                    size="sm"
                    value={saveName}
                    onValueChange={setSaveName}
                  />
                  <Button
                    color="primary"
                    size="md"
                    onPress={() => {
                      const name = saveName.trim();
                      if (!name) {
                        addToast({ title: "请先填写名称", color: "warning" });
                        return;
                      }
                      setSaved(putSaved(name, state));
                      addToast({ title: `已保存「${name}」` });
                    }}
                  >
                    保存
                  </Button>
                </div>
                <Divider />
                {saved.length === 0 && (
                  <p className="text-sm text-default-400">还没有保存的名片</p>
                )}
                {saved.map((item) => (
                  <div key={item.name} className="flex items-center gap-2">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm">{item.name}</p>
                      <p className="text-xs text-default-400">
                        {new Date(item.savedAt).toLocaleString()}
                      </p>
                    </div>
                    <Button size="sm" variant="flat" onPress={() => setState(item.state)}>
                      加载
                    </Button>
                    <Button
                      size="sm"
                      variant="light"
                      color="danger"
                      onPress={() => setSaved(deleteSaved(item.name))}
                    >
                      删除
                    </Button>
                  </div>
                ))}
                <Divider />
                <Button
                  size="sm"
                  variant="light"
                  color="danger"
                  onPress={() => {
                    setState(defaultState());
                    addToast({ title: "已恢复默认名片" });
                  }}
                >
                  恢复默认
                </Button>
              </div>
            </Tab>
          </Tabs>
        </CardBody>
      </Card>
    </div>
  );
}
