import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// 使用相对基路径，产物可部署到任意静态托管（含 GitHub Pages 子路径）。
export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss()],
});
