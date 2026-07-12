# NiceTools

简洁好用的在线工具集合，纯前端运行、无需登录，数据仅保存在本地浏览器。

## 技术栈

- React 18 + TypeScript
- Vite
- HeroUI（原生组件）+ Tailwind CSS
- React Router（BrowserRouter，GitHub Pages 使用 `404.html` 兜底深链）

## 本地开发

```bash
pnpm install
pnpm dev
```

## 构建

```bash
pnpm build
```

产物在 `dist/`，可部署到任意静态托管。

## 添加新工具

所有工具通过 `src/tools/registry.ts` 注册。新增一个工具只需：

1. 在 `src/tools-impl/<your-tool>/` 下实现组件（默认导出）。
2. 在 `registry.ts` 的 `tools` 数组追加一条记录（id / 名称 / 说明 / emoji / 分组 / 分类 / 懒加载组件）。

首页搜索、分组导航和路由都会自动读取注册表，无需改动其它文件。工具页面路由为 `/tools/<id>`。

## 现有工具

| 工具 | 说明 |
| --- | --- |
| 💰 大模型费用计算器 | 按 TPM、时长、输入输出比例、缓存命中率和支付折扣估算 token 成本 |
