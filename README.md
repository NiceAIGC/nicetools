# NiceTools

NiceTools 是一个带后台管理和细粒度权限控制的在线工具集。现有工具默认允许游客使用；后续工具可以按游客、登录用户和用户组分别配置可见与可用权限。

## 技术栈

- React 18 + TypeScript + Vite
- HeroUI + Tailwind CSS
- Go 1.26 + Gin + GORM + SQLite
- 单 Docker 容器：Go 服务提供 API、静态前端和受权限保护的工具代码资源

界面组件一律使用 HeroUI 原生组件，除非单独指定；不引入第二个组件库。
HeroUI 没有的控件（如取色器）用原生元素兜底，并在代码注释中说明原因。

界面使用 HeroUI 原生 Navbar、头像账户菜单和 Tabs；管理后台在桌面使用左侧导航，移动端使用横向栏目切换，搜索和账号操作均保留。
头部使用工具箱标志；品牌和账户操作区保留实际宽度，搜索区只占剩余空间，窄屏将搜索移至下一行。

## 架构与元数据归属

单一 Docker 容器：Go（Gin + GORM + SQLite）同时提供 `/api`、前端静态文件和按权限保护的工具 JS 资源。

**工具的展示元数据归后台所有**：名称、Logo/图标、说明、分类、标签、排序都存在 SQLite 中，管理员在 `/admin` 修改后首页立即生效。`src/tools/catalog.json` 只承担三个用途：

1. 新工具首次出现时的**默认元数据种子**；
2. 前端的 **工具 id → 懒加载组件映射**（`src/tools/registry.ts`）；
3. 构建期生成 `dist/tool-assets.json`，供 Go 服务按权限拦截各工具的代码资源。

启动时 `store.Sync` 只做三件事：播种新工具、清理代码里已删除的工具、把旧版 `tools.category` 文本列迁移到分类表。它**不会覆盖**已有工具的后台元数据；需要回到代码默认值时，用「工具管理 → 编辑信息 → 恢复默认信息」。

分类是独立实体（`categories` 表）：可新建、改名、删除、上移/下移；删除分类只把工具改成「未分类」，不影响工具本身与访问策略。

代码入口：

| 关注点 | 位置 |
| --- | --- |
| 分类 CRUD、工具信息、恢复默认 | `backend/internal/service/categories.go` |
| 工具策略与访问判定（游客/会员/组） | `backend/internal/service/permissions.go` |
| 目录播种与旧库迁移 | `backend/internal/store/store.go` |
| 工具后端模块契约与注册表 | `backend/internal/module/`、`backend/internal/tools/` |
| 后台 REST 接口 | `backend/internal/httpapi/admin.go` |
| 前端展示信息消费 | `src/pages/Home.tsx`、`src/pages/ToolPage.tsx` |
| 后台管理界面 | `src/pages/Admin.tsx`、`src/components/admin/` |

带后端的工具以模块形式挂在 `/api/tools/<id>/` 下，自动继承平台的访问策略与 CSRF 体系，
表结构由各模块自己迁移。分层、模块契约、表命名与行为约定见 [ARCHITECTURE.md](ARCHITECTURE.md)。

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
- 随版本默认开放的工具由后端 `initiallyPublic` 清单维护；清单外的新工具默认对游客和会员关闭，由管理员在后台开启。

工具代码懒加载资源由后端按权限保护。浏览器已经下载的本地代码无法被远程收回，这是浏览器端工具的固有边界。

## 后台自定义

管理员登录 `/admin` 后可以：

- **工具管理**：修改工具名称、Logo/图标、说明、所属分类、标签和排序值，一键恢复代码默认信息；「配置权限」独立控制游客、会员和各用户组的可见/可用。
- **工具分类**：新建、改名、删除分类，用「上移 / 下移」调整首页分类顺序；新建分类排在末尾，删除分类后其中的工具变为「未分类」。
- **用户账号**：创建、编辑、禁用、删除账号，重置密码，为一个账号分配多个用户组。
- **用户组**：维护组名与说明；删除组会同时清掉成员归属和该组的工具覆盖规则。
- **审计记录**：查看最近 200 条账号、权限与登录操作。

系统不提供注册入口，账号只能由管理员创建。

## 添加工具

1. 在 `src/tools-impl/<your-tool>/index.tsx` 实现默认导出的 React 组件。
2. 在 `src/tools/catalog.json` 添加 `id`、名称、说明、emoji、分类和标签——这些只作为首次播种的默认值。
3. 执行 `pnpm build`。Vite 会按目录生成懒加载资源和 `dist/tool-assets.json`，Go 服务据此保护工具代码。
4. 部署后工具出现在「后台 → 工具管理」，展示信息可继续在后台调整。代码目录清单（`store.initiallyPublic`）之外的新工具默认对所有人关闭，需在「配置权限」中打开。

工具需要后端时，在上面基础上：

1. `catalog.json` 条目加 `"backend": true`。
2. 在 `backend/internal/tools/<id>/` 实现 `module.Module` 接口（表迁移 + 路由），
   并在 `internal/tools/tools.go` 的 `All()` 注册。
3. 前端通过 `toolApi(id, path)` 调用 `/api/tools/<id>/`；访问策略、CSRF、审计由平台统一处理。

启动时会校验模块与 `catalog.json` 的双向对齐，缺任一侧直接拒绝启动。详细规范见
[ARCHITECTURE.md](ARCHITECTURE.md)。

首页搜索、分类导航和路由会自动读取后台数据，标签参与搜索，工具页面路由为 `/tools/<id>`。

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
| 📠 文档转扫描件 | PDF、图片、Word、TXT 转带扫描质感的 PDF，支持旋转、噪点、泛黄、黑白；方案保存在本机，预览可放大，文件名带时间戳 |
| 🪪 名片制作 | 水印回纹风格名片：内容逐项可显隐，版式/字体/配色/字号/水印全可配置，编辑与名片夹保存在本机，导出 PNG/JPG 最高 3 倍图 |
| 🛂 大模型准入测试 | 按准入标准对模型渠道做三档（F 功能 / M 中等 / S 压力）测试；服务端异步执行、单飞、可暂停续跑；每个用例带测试说明与判据，可逐项跳过/豁免；报告含脱敏 curl 与响应原文，支持 JSON/MD/PDF 与多任务合并导出，均带 niceaigc 水印；首个内置标准为 Kimi-K3 v1.0 |
