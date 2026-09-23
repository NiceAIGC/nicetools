import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// 只把 React 内核与路由单独成块：
// 这部分体积稳定、几乎不随业务改动变化，拆出后可以长期缓存，
// 应用代码更新时浏览器只需重新下载入口块。
// 其余依赖（HeroUI / react-aria / jszip 等）交给 Rollup 处理，
// 这样只有某个工具用到的组件仍留在该工具的懒加载 chunk 里，不会进首屏。
function splitVendorChunks(id: string): string | undefined {
  if (!id.includes("node_modules")) return undefined;

  if (
    /[\\/]node_modules[\\/](react|react-dom|scheduler|react-router|react-router-dom|@remix-run)[\\/]/.test(id)
  ) {
    return "vendor-react";
  }

  return undefined;
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");

  return {
    base: env.VITE_BASE_PATH || "/",
    plugins: [react(), tailwindcss()],
    build: {
      minify: "terser",
      terserOptions: {
        compress: { passes: 2 },
        format: { comments: false },
      },
      rollupOptions: {
        output: {
          manualChunks: splitVendorChunks,
        },
      },
    },
  };
});
