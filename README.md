# NiceTools

简洁好用的在线工具集合，安全快捷，注重隐私保护。

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

产物在 `dist/`。默认构建适合部署到域名根路径，例如 `https://tools.nicecode.ai/`。

如果部署到 GitHub Pages 的 `/nicetools/` 子路径，使用：

```bash
pnpm build:gh-pages
```

## 添加新工具

所有工具通过 `src/tools/registry.ts` 注册。新增一个工具只需：

1. 在 `src/tools-impl/<your-tool>/` 下实现组件（默认导出）。
2. 在 `registry.ts` 的 `tools` 数组追加一条记录（id / 名称 / 说明 / emoji / 分类 / 标签 / 懒加载组件）。

首页搜索、分类导航和路由都会自动读取注册表，标签会显示在工具卡片上并参与搜索。工具页面路由为 `/tools/<id>`。

## 现有工具

| 工具 | 说明 |
| --- | --- |
| 🖼️ 离线照片水印打码工具 | 浏览器本地为图片加水印，支持平铺/单个/自定义数量布局与批量打包下载 |
| 🔌 大模型连通性测试 | 用 OpenAI/Claude 格式探测模型接口，支持流式、curl 复测与本地记录配置 |
| 💰 大模型费用计算器 | 按 TPM、时长、输入输出比例、缓存命中率和支付折扣估算 token 成本 |
| 🔎 JSON 值提取工具 | 逐行解析 JSON 对象并提取指定顶层键的值 |
| ✂️ 多行文本分隔工具 | 按自定义分隔符拆分文本并按索引重排拼接 |
