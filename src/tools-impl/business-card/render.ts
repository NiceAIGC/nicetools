// 名片渲染：预览与导出共用同一 canvas 绘制，杜绝两套渲染逻辑走样。
import type { CardState } from "./model";

export const CARD_W = 1050;
export const CARD_H = 600;

// 手动圆角路径：ctx.roundRect 在旧浏览器上缺失，自己画保证导出一致。
function roundRectPath(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  r: number,
) {
  const radius = Math.min(r, w / 2, h / 2);
  ctx.beginPath();
  ctx.moveTo(radius, 0);
  ctx.arcTo(w, 0, w, h, radius);
  ctx.arcTo(w, h, 0, h, radius);
  ctx.arcTo(0, h, 0, 0, radius);
  ctx.arcTo(0, 0, w, 0, radius);
  ctx.closePath();
}

// 字距：逐字测量 + 间隙累加；canvas 的 letterSpacing 属性有兼容性差异，手工实现。
function spacedWidth(ctx: CanvasRenderingContext2D, text: string, spacing: number) {
  let width = 0;
  for (const ch of text) width += ctx.measureText(ch).width;
  return width + spacing * Math.max(0, text.length - 1);
}

function drawSpaced(
  ctx: CanvasRenderingContext2D,
  text: string,
  cx: number,
  cy: number,
  spacing: number,
  align: "center" | "left",
) {
  let x = align === "center" ? cx - spacedWidth(ctx, text, spacing) / 2 : cx;
  for (const ch of text) {
    const w = ctx.measureText(ch).width;
    ctx.fillText(ch, x + w / 2, cy);
    x += w + spacing;
  }
}

function drawWatermark(ctx: CanvasRenderingContext2D, state: CardState) {
  const { style } = state;
  const text = style.wmText.trim() || "NICETOOLS";
  const unit = `${text} · `;
  const spacing = style.wmSize / 4;
  ctx.save();
  ctx.translate(CARD_W / 2, CARD_H / 2);
  ctx.rotate((style.wmAngle * Math.PI) / 180);
  ctx.font = `800 ${style.wmSize}px ${style.fontFamily}`;
  ctx.fillStyle = style.primary;
  ctx.globalAlpha = style.wmOpacity / 100;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  const extent = Math.hypot(CARD_W, CARD_H);
  const unitWidth = spacedWidth(ctx, unit, spacing) || 1;
  const line = unit.repeat(Math.ceil((extent * 2) / unitWidth) + 1);
  for (let y = -extent; y <= extent; y += style.wmGap) {
    drawSpaced(ctx, line, 0, y, spacing, "center");
  }
  ctx.restore();
}

interface Line {
  text: string;
  size: number;
  weight: number;
  color: string;
  /** 字距，单位 em 的比例换算在调用处完成 */
  spacing: number;
  /** 与前一行中心的间距 */
  gapBefore: number;
}

function drawContent(ctx: CanvasRenderingContext2D, state: CardState) {
  const { fields, show, style } = state;
  const visible = (key: keyof CardState["fields"]) =>
    show[key] && fields[key].trim() !== "";

  const lines: Line[] = [];
  if (visible("logo"))
    lines.push({ text: fields.logo.trim(), size: style.logoSize, weight: 700, color: style.primary, spacing: style.logoSize * 0.6, gapBefore: 0 });
  if (visible("company"))
    lines.push({ text: fields.company.trim(), size: style.companySize, weight: 700, color: style.primary, spacing: style.companySize / 6, gapBefore: 16 });
  if (visible("companyEn"))
    lines.push({ text: fields.companyEn.trim(), size: style.enSize, weight: 400, color: style.textColor, spacing: style.enSize / 5, gapBefore: 10 });

  const hasName = visible("name");
  const hasTitle = visible("title");
  const contacts = (["phone", "email", "address", "website"] as const).filter(visible);

  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  const padTop = 84;
  const center = style.layout === "center";
  const x = center ? CARD_W / 2 : 88;

  // 上半部分：标识 → 公司 → 英文名 → 规则线 → 姓名 → 职位
  let y = padTop;
  for (const line of lines) {
    y += line.gapBefore + line.size / 2;
    ctx.font = `${line.weight} ${line.size}px ${style.fontFamily}`;
    ctx.fillStyle = line.color;
    drawSpaced(ctx, line.text, x, y, line.spacing, center ? "center" : "left");
    y += line.size / 2;
  }
  if (hasName || hasTitle) {
    y += 30;
    if (center) {
      ctx.fillStyle = style.primary;
      ctx.fillRect(CARD_W / 2 - 30, y - 1, 60, 2);
    } else {
      ctx.fillStyle = style.primary;
      ctx.fillRect(88, y - 1, 60, 2);
    }
    y += 30;
  }
  if (hasName) {
    y += style.nameSize / 2;
    ctx.font = `800 ${style.nameSize}px ${style.fontFamily}`;
    ctx.fillStyle = style.primary;
    drawSpaced(ctx, fields.name.trim(), x, y, style.nameSize / 6, center ? "center" : "left");
    y += style.nameSize / 2;
  }
  if (hasTitle) {
    y += 12 + style.titleSize / 2;
    ctx.font = `400 ${style.titleSize}px ${style.fontFamily}`;
    ctx.fillStyle = style.textColor;
    drawSpaced(ctx, fields.title.trim(), x, y, style.titleSize / 5, center ? "center" : "left");
    y += style.titleSize / 2;
  }

  // 底部联系方式：居中版横排，左对齐版竖排。
  if (contacts.length === 0) return;
  ctx.font = `400 ${style.contactSize}px ${style.fontFamily}`;
  ctx.fillStyle = style.textColor;
  const spacing = style.contactSize / 8;
  const items = contacts.map((key) => fields[key].trim());
  if (center) {
    const gap = 44;
    const total =
      items.reduce((sum, item) => sum + spacedWidth(ctx, item, spacing), 0) +
      gap * (items.length - 1);
    let cx = CARD_W / 2 - total / 2;
    const cy = CARD_H - 84;
    for (const item of items) {
      const w = spacedWidth(ctx, item, spacing);
      drawSpaced(ctx, item, cx + w / 2, cy, spacing, "center");
      cx += w + gap;
    }
  } else {
    const lineHeight = style.contactSize * 1.8;
    let cy = CARD_H - 80 - ((items.length - 1) * lineHeight) / 2;
    for (const item of items) {
      drawSpaced(ctx, item, 88, cy, spacing, "left");
      cy += lineHeight;
    }
  }
}

/** 把名片画到给定画布；scale 控制导出倍率。 */
export function drawCard(
  canvas: HTMLCanvasElement,
  state: CardState,
  scale: number,
) {
  canvas.width = CARD_W * scale;
  canvas.height = CARD_H * scale;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  ctx.save();
  ctx.scale(scale, scale);
  roundRectPath(ctx, CARD_W, CARD_H, state.style.radius);
  ctx.fillStyle = state.style.background;
  ctx.fill();
  roundRectPath(ctx, CARD_W, CARD_H, state.style.radius);
  ctx.save();
  ctx.clip();
  if (state.style.wmEnabled) drawWatermark(ctx, state);
  drawContent(ctx, state);
  ctx.restore();
  if (state.style.border) {
    ctx.translate(1, 1);
    roundRectPath(ctx, CARD_W - 2, CARD_H - 2, state.style.radius);
    ctx.strokeStyle = state.style.primary;
    ctx.globalAlpha = 0.35;
    ctx.lineWidth = 2;
    ctx.stroke();
  }
  ctx.restore();
}
