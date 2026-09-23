// 站点级深浅色主题：HeroUI 通过 html.dark 切换配色（见 src/index.css 的 dark 变体）。
export type Theme = "light" | "dark";

const storageKey = "nicetools.theme";

export function currentTheme(): Theme {
  try {
    const stored = localStorage.getItem(storageKey);
    if (stored === "dark" || stored === "light") return stored;
  } catch {
    // 隐私模式等场景下 localStorage 不可用，回退到系统偏好
  }
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle("dark", theme === "dark");
  document.documentElement.style.colorScheme = theme;
}

/** 首屏渲染前调用，避免主题闪烁；返回当前主题。 */
export function initTheme(): Theme {
  const theme = currentTheme();
  applyTheme(theme);
  return theme;
}

export function storeTheme(theme: Theme): void {
  applyTheme(theme);
  try {
    localStorage.setItem(storageKey, theme);
  } catch {
    // 存储失败不影响本次切换
  }
}
