// 名片制作：数据模型、默认值与 localStorage 持久化。
// 纯前端工具，所有数据只存在浏览器本地。

export interface CardFields {
  /** 顶部标识文字（logo 位） */
  logo: string;
  /** 公司名称 */
  company: string;
  /** 公司英文/副标题 */
  companyEn: string;
  /** 姓名 */
  name: string;
  /** 职位 */
  title: string;
  /** 电话 */
  phone: string;
  /** 邮箱 */
  email: string;
  /** 地址 */
  address: string;
  /** 网站 */
  website: string;
}

export type FieldKey = keyof CardFields;

export type SizeKey =
  | "logoSize"
  | "companySize"
  | "enSize"
  | "nameSize"
  | "titleSize"
  | "contactSize";

export interface CardStyle {
  /** 版式 */
  layout: "center" | "left";
  /** 字体栈 */
  fontFamily: string;
  /** 主色：姓名/公司/标识/规则线/水印 */
  primary: string;
  /** 卡片背景色 */
  background: string;
  /** 次要文字色：职位/联系方式/英文名 */
  textColor: string;
  /** 圆角半径 px */
  radius: number;
  /** 描边 */
  border: boolean;
  /** 各分组字号（基准 1050×600 画布） */
  logoSize: number;
  companySize: number;
  enSize: number;
  nameSize: number;
  titleSize: number;
  contactSize: number;
  /** 水印 */
  wmEnabled: boolean;
  wmText: string;
  wmSize: number;
  wmOpacity: number;
  wmGap: number;
  wmAngle: number;
}

export interface CardState {
  fields: CardFields;
  show: Record<FieldKey, boolean>;
  style: CardStyle;
}

export interface SavedCard {
  name: string;
  savedAt: number;
  state: CardState;
}

export const fieldDefs: { key: FieldKey; label: string; placeholder: string }[] = [
  { key: "logo", label: "标识文字", placeholder: "MINGSHU" },
  { key: "company", label: "公司名称", placeholder: "广州明枢科技有限公司" },
  { key: "companyEn", label: "公司英文", placeholder: "GUANGZHOU MINGSHU TECHNOLOGY CO., LTD." },
  { key: "name", label: "姓名", placeholder: "姓名占位" },
  { key: "title", label: "职位", placeholder: "职位占位" },
  { key: "phone", label: "电话", placeholder: "138 0000 0000" },
  { key: "email", label: "邮箱", placeholder: "name@mingshu.tech" },
  { key: "address", label: "地址", placeholder: "广州市 · 天河区" },
  { key: "website", label: "网站", placeholder: "www.mingshu.tech" },
];

export const sizeDefs: { key: SizeKey; label: string; min: number; max: number }[] = [
  { key: "logoSize", label: "标识字号", min: 12, max: 40 },
  { key: "companySize", label: "公司字号", min: 16, max: 60 },
  { key: "enSize", label: "英文名字号", min: 8, max: 24 },
  { key: "nameSize", label: "姓名字号", min: 24, max: 100 },
  { key: "titleSize", label: "职位字号", min: 12, max: 40 },
  { key: "contactSize", label: "联系方式字号", min: 10, max: 32 },
];

export const fontOptions = [
  {
    key: "sans",
    label: "黑体（无衬线）",
    stack: '-apple-system,"PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif',
  },
  { key: "serif", label: "宋体（衬线）", stack: '"Songti SC","STSong","SimSun",serif' },
  { key: "kai", label: "楷体", stack: '"Kaiti SC","STKaiti","KaiTi",serif' },
];

/** 预设色板；自定义颜色用原生取色器（HeroUI 无该组件，见 index.tsx 注释）。 */
export const colorPresets = [
  "#1c2b4a",
  "#111827",
  "#7f1d1d",
  "#065f46",
  "#78350f",
  "#1e3a8a",
];

export const bgPresets = ["#ffffff", "#faf7f2", "#f1f5f9", "#0f172a", "#111827"];

export function defaultState(): CardState {
  return {
    fields: {
      logo: "MINGSHU",
      company: "广州明枢科技有限公司",
      companyEn: "GUANGZHOU MINGSHU TECHNOLOGY CO., LTD.",
      name: "姓名占位",
      title: "职位占位",
      phone: "138 0000 0000",
      email: "name@mingshu.tech",
      address: "广州市 · 天河区",
      website: "www.mingshu.tech",
    },
    show: {
      logo: true,
      company: true,
      companyEn: true,
      name: true,
      title: true,
      phone: true,
      email: true,
      address: true,
      website: false,
    },
    style: {
      layout: "center",
      fontFamily: fontOptions[0].stack,
      primary: "#1c2b4a",
      background: "#ffffff",
      textColor: "#5a6785",
      radius: 24,
      border: false,
      logoSize: 20,
      companySize: 30,
      enSize: 13,
      nameSize: 58,
      titleSize: 20,
      contactSize: 16,
      wmEnabled: true,
      wmText: "MINGSHU",
      wmSize: 46,
      wmOpacity: 6,
      wmGap: 110,
      wmAngle: 0,
    },
  };
}

const STATE_KEY = "business-card:state";
const SAVED_KEY = "business-card:saved";

/** 与默认值深合并，容忍旧版本存档缺字段。 */
function mergeState(raw: unknown): CardState {
  const base = defaultState();
  if (raw === null || typeof raw !== "object") return base;
  const partial = raw as Partial<CardState>;
  return {
    fields: { ...base.fields, ...partial.fields },
    show: { ...base.show, ...partial.show },
    style: { ...base.style, ...partial.style },
  };
}

export function loadState(): CardState {
  try {
    const raw = localStorage.getItem(STATE_KEY);
    return raw ? mergeState(JSON.parse(raw)) : defaultState();
  } catch {
    return defaultState();
  }
}

export function saveState(state: CardState) {
  try {
    localStorage.setItem(STATE_KEY, JSON.stringify(state));
  } catch {
    // 存储满或被禁用时静默失败，不影响编辑。
  }
}

export function loadSaved(): SavedCard[] {
  try {
    const raw = localStorage.getItem(SAVED_KEY);
    if (!raw) return [];
    const list = JSON.parse(raw) as SavedCard[];
    if (!Array.isArray(list)) return [];
    return list
      .filter((item) => item && typeof item.name === "string" && item.state)
      .map((item) => ({ ...item, state: mergeState(item.state) }));
  } catch {
    return [];
  }
}

function persistSaved(list: SavedCard[]) {
  try {
    localStorage.setItem(SAVED_KEY, JSON.stringify(list));
  } catch {
    // 同上。
  }
}

export function putSaved(name: string, state: CardState): SavedCard[] {
  const list = loadSaved().filter((item) => item.name !== name);
  list.unshift({ name, savedAt: Date.now(), state });
  persistSaved(list);
  return list;
}

export function deleteSaved(name: string): SavedCard[] {
  const list = loadSaved().filter((item) => item.name !== name);
  persistSaved(list);
  return list;
}

/** 导出文件名：名片-姓名-时间戳.ext；姓名含非法字符时替换。 */
export function exportFileName(name: string, format: "png" | "jpeg"): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  const who = name.trim().replace(/[\\/:*?"<>|]/g, "_") || "未命名";
  return `名片-${who}-${stamp}.${format === "png" ? "png" : "jpg"}`;
}
