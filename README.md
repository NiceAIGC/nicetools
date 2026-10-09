# NiceTools

NiceTools 是一个带后台管理和细粒度权限控制的在线工具集。现有工具默认允许游客使用；后续工具可以按游客、登录用户和用户组分别配置可见与可用权限。

## 技术栈

- React 18 + TypeScript + Vite
- HeroUI + Tailwind CSS
- Go 1.26 + Gin + GORM + SQLite
- 单 Docker 容器：Go 服务提供 API、静态前端和受权限保护的工具代码资源

界面使用 HeroUI 原生 Navbar、头像账户菜单和 Tabs；管理后台在桌面使用左侧导航，移动端使用横向栏目切换，搜索和账号操作均保留。
头部使用工具箱标志；品牌和账户操作区保留实际宽度，搜索区只占剩余空间，窄屏将搜索移至下一行。

## 本地开发

先生成前端产物，再启动 Go API。数据库默认写入 `backend/data/nicetools.db`。

```bash
pnpm install
pnpm build
cd backend
ADMIN_PASSWORD='change-this-password' go run ./cmd/server
```

另开终端启动 Vite：

```bash
pnpm dev
```

Vite 会把 `/api` 请求代理到 `127.0.0.1:8080`。`ADMIN_PASSWORD` 只在空数据库首次启动时用于创建管理员，必须为 12–72 字节；系统没有公开注册接口。

## 生产部署

服务器安装 Docker Engine、Docker Compose v2 和 curl 后，在项目根目录执行：

```bash
sh deploy.sh
```

`sh deply.sh` 也可用，这是兼容用户现有拼写的入口。首次部署会生成权限为 `0600` 的 `.env`，并生成随机初始管理员密码；密码只写入该文件，不打印到日志。之后部署会复用同一个 `data` Docker 卷，数据库和账号不会因更新镜像丢失。

部署脚本会依次执行前端构建、Go 测试、Docker 镜像构建、容器健康等待和宿主机 `/api/health` 检查。失败时不会删除数据卷，并输出最近容器日志。

生产 HTTPS 反向代理场景：在 `.env` 设置 `COOKIE_SECURE=true` 和实际的 `PUBLIC_ORIGIN=https://tools.example.com`，可将 `BIND_ADDRESS` 设为 `127.0.0.1`。

## 用户与权限

- 游客不需要登录；游客只能看到 `guest_visible` 工具，并且只有 `guest_use` 为真时可以使用。
- 登录用户按 `member_visible`、`member_use` 和其所属的全部用户组规则计算权限。
- 组规则按每个旗标独立计算：显式拒绝优先，之后是显式允许，最后回退到会员默认；可用权限必须同时可见。
- 管理员可以查看全部工具策略，但停用的工具即使管理员也不能运行。
- 管理员在 `/admin` 创建账号、管理用户组、配置工具策略和查看审计记录；不提供注册页面。
- 删除用户组会同时移除成员关系和该组工具覆盖规则，权限立即按剩余组重新计算。
- 修改密码、重置密码、禁用账号或修改角色会撤销相关账号的会话。

工具代码懒加载资源由后端按权限保护。浏览器已经下载的本地代码无法被远程收回，这是浏览器端工具的固有边界。

## 添加工具

1. 在 `src/tools-impl/<your-tool>/index.tsx` 实现默认导出的 React 组件。
2. 在 `src/tools/catalog.json` 添加 `id`、名称、说明、emoji、分类和标签。
3. 执行 `pnpm build`。Vite 会根据目录生成懒加载资源和 `dist/tool-assets.json`，Go 服务会据此保护工具代码。

新工具默认关闭游客和会员权限；管理员保存策略后才会对相应受众开放。现有 7 个工具的初始策略保持游客和会员可见可用。

## 现有工具

| 工具 | 说明 |
| --- | --- |
| 🖼️ 离线照片水印打码工具 | 浏览器本地为图片加水印，支持平铺、单个、自定义数量布局与批量打包下载 |
| ⏰ 在线闹钟 | 创建多个闹钟，支持重复提醒、贪睡和声音通知 |
| 🔌 大模型连通性测试 | 用 OpenAI 或 Claude 格式探测模型接口，支持流式输出与 curl 复测 |
| 💰 大模型费用计算器 | 按 TPM、时长、输入输出比例、缓存命中率和支付折扣估算 token 成本 |
| 🧪 Prompt 缓存探测器 | 预热固定前缀后发起并发请求，检测上游 API 缓存命中情况 |
| 🔎 JSON 值提取工具 | 逐行解析 JSON 对象并提取指定顶层键 |
| ✂️ 多行文本分隔工具 | 按自定义分隔符拆分多行文本并按索引重排拼接 |
