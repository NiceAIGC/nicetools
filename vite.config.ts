import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import catalog from "./src/tools/catalog.json";

// 工具懒加载代码及仅供工具使用的依赖必须由 Go 服务执行权限校验。
function protectedToolAssets(): Plugin {
  return {
    name: "protected-tool-assets",
    apply: "build",
    generateBundle(_options, bundle) {
      const chunks = new Map(
        Object.values(bundle)
          .filter((item) => item.type === "chunk")
          .map((chunk) => [chunk.fileName, chunk]),
      );
      const roots = new Map<string, string>();
      for (const tool of catalog) {
        const root = [...chunks.values()].find((chunk) =>
          chunk.moduleIds.some((id) =>
            id.replace(/\\/g, "/").endsWith(`/tools-impl/${tool.id}/index.tsx`),
          ),
        );
        if (!root || root.isEntry) throw new Error(`工具 ${tool.id} 未生成独立加载资源`);
        if ([...roots.values()].includes(root.fileName)) {
          throw new Error(`工具 ${tool.id} 与其他工具合并，无法独立授权`);
        }
        roots.set(tool.id, root.fileName);
      }
      const rootFiles = new Set(roots.values());
      function reachable(start: string, blocked: ReadonlySet<string>): Set<string> {
        const visited = new Set<string>();
        function visit(file: string) {
          if (visited.has(file) || blocked.has(file)) return;
          const chunk = chunks.get(file);
          if (!chunk) return;
          visited.add(file);
          for (const imported of [...chunk.imports, ...chunk.dynamicImports]) visit(imported);
        }
        visit(start);
        return visited;
      }
      const publicFiles = new Set<string>();
      for (const chunk of chunks.values()) {
        if (chunk.isEntry) {
          for (const file of reachable(chunk.fileName, rootFiles)) publicFiles.add(file);
        }
      }
      for (const file of publicFiles) {
        if (chunks.get(file)?.moduleIds.some((id) => id.replace(/\\/g, "/").includes("/tools-impl/"))) {
          throw new Error(`工具实现进入公共资源 ${file}，必须保留独立懒加载`);
        }
      }
      const manifest: Record<string, string[]> = {};
      for (const [toolID, root] of roots) {
        if (publicFiles.has(root)) throw new Error(`工具 ${toolID} 被打包进公共入口`);
        for (const file of reachable(root, publicFiles)) {
          (manifest[file] ??= []).push(toolID);
        }
      }
      for (const chunk of chunks.values()) {
        for (const moduleID of chunk.moduleIds) {
          const match = moduleID.replace(/\\/g, "/").match(/\/tools-impl\/([^/]+)\//);
          if (match && !manifest[chunk.fileName]?.includes(match[1])) {
            throw new Error(`资源 ${chunk.fileName} 含未受保护的工具 ${match[1]}`);
          }
        }
      }
      this.emitFile({
        type: "asset",
        fileName: "tool-assets.json",
        source: JSON.stringify(manifest),
      });
    },
  };
}

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

export default defineConfig({
    plugins: [react(), tailwindcss(), protectedToolAssets()],
    server: {
      proxy: { "/api": "http://127.0.0.1:8080" },
    },
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
});
