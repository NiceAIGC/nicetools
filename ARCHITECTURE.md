# 架构与开发规范

本文是 NiceTools 的架构契约：分层、工具后端模块机制、数据表规范与行为约定。
新增工具（尤其是带后端的工具）前必读。README 讲使用与部署，本文讲结构。

## 1. 分层

```
cmd/server            装配：config → store → Sync → 模块校验/迁移 → service → httpapi
internal/config       环境变量加载与校验
internal/validate     跨包共用的标识符规则（Slug / Username），全库只此一份
internal/model        只放共享表：User Group Session Audit Tool Category GroupRule + Catalog
internal/store        共享表的连接、迁移、目录播种（Sync）；不感知任何工具模块
internal/service      共享领域服务：会话、用户、分类、工具策略、审计
internal/module       工具后端模块契约（Module 接口、Deps、Fail、VerifyCatalog）
internal/tools        模块注册表 All() + 每个工具后端一个子目录
internal/httpapi      平台路由、安全中间件、模块装配
```

依赖方向只允许向下：`cmd → httpapi → module → service → store → model`，`validate` 可被任何层引用。
禁止反向引用，禁止工具模块互相 import。

## 2. 工具后端模块

一个带后端的工具 = `internal/tools/<id>/` 下一个实现 `module.Module` 的包，然后在
`internal/tools/tools.go` 的 `All()` 里加一行。不改 core 任何文件。

```go
type Module interface {
    ID() string                                    // 必须等于 catalog.json 中的 id
    Migrate(db *gorm.DB) error                     // 建表/迁移，启动时装配路由前调用
    Register(read, write, pub *gin.RouterGroup, d Deps) // 挂在 /api/tools/<id>/ 下
}
```

- `read` 已带平台「工具可见且可用」策略判定（与 `/api/tools/:id/access` 同一套 GroupRule 逻辑），
  不可见 404、可见不可用 403，模块自己不写权限判断（除非有工具内子权限）。
- `write` 在 `read` 之上再加 CSRF 校验；**所有 POST/PUT/DELETE 必须挂 `write`**。
- `pub`（`/api/tools/<id>/pub/`）免鉴权，仅供必须被外部系统直接拉取的资源
  （如被测网关拉测试媒体）；模块必须用一次性/任务级 token 自行保护，严禁放敏感数据。
- `Deps` 提供 `DB / Service / Config`；模块不得另开数据库连接。
- 模块 handler 用 `module.CurrentUser(c)` 取登录用户（游客为 nil）。

### 目录对齐（启动门禁）

`catalog.json` 中 `"backend": true` 声明工具带后端。启动时 `module.VerifyCatalog` 双向校验：

- 注册的模块必须对应目录里 `backend: true` 的工具；
- 目录里 `backend: true` 的工具必须有已注册模块。

不满足则进程拒绝启动。`httpapi.New` 另外校验模块 id 是合法 slug 且不重复。

### 新增带后端工具的清单

1. 前端 `src/tools-impl/<id>/` 照常实现组件；`src/tools/catalog.json` 条目加 `"backend": true`。
2. 后端建 `internal/tools/<id>/`：`module.go`（实现接口）、`model.go`（表）、
   `service.go`（业务）、`handler.go`（路由）；文件按需合并拆分，目录边界不动。
3. `internal/tools/tools.go` 的 `All()` 注册一行。
4. 模块带 `<id>_test.go`，只测自己的路由与边界（参照 `httpapi/module_test.go` 的 fakeModule）。

参考实现：`internal/tools/llmadmit/`（大模型准入测试）——模块内再按「契约子包 + 实现子包」
分层（`suite/` 抽象与客户端、`k3/` 标准实现），适合一个工具承载多套可插拔标准的场景。

## 3. 数据表规范

| 规则 | 内容 |
| --- | --- |
| 归属 | 共享表只放 `internal/model`；工具表只放本模块 `model.go` |
| 命名 | 工具表一律 `tool_<id>_` 前缀，id 中 `-` 转 `_`，如 `tool_docscan_jobs` |
| 迁移 | 各模块在自己的 `Migrate()` 里 `AutoMigrate`；禁止塞进 store 全局迁移 |
| 关联用户 | 用 `UserID uint` 外键指向共享 users 表，加 `constraint:OnDelete:CASCADE` |
| 隔离 | 一个模块绝不查询/迁移另一个模块的表 |

## 4. 工具之间的关系

1. **无关**（默认）：独立模块，只用 `Deps.Service` 的会话/审计能力。
2. **共用同一领域概念**：抽共享领域服务到 `internal/service/` 或新建领域包，
   工具模块调用它。判断标准：同一概念出现第二次才抽，第一次写在模块内。
3. **强耦合的一组工具**：合并为一个模块，不互相 import。

## 5. 行为约定（全库统一）

- **错误**：业务错误用 `service.Problem(status, message)`；GORM 错误过 `service.DBError`；
  handler 错误出口统一 `module.Fail(c, err)`，禁止自行 `c.JSON` 写错误状态。
- **审计**：任何创建/修改/删除动作在同一事务内调 `service.Audit(tx, actor, action, target)`；
  action 命名 `<toolid>.<动作>`，如 `docscan.job.delete`。
- **输入**：JSON 绑定走 `bind()` 模式；长度校验用 `service.Bounded`；标识符用 `validate` 包。
- **路由**：工具后端地址空间是 `/api/tools/<id>/`；不得注册名为 `access` 的子路径
  （与平台 `/api/tools/:id/access` 冲突）。
- **CSRF**：写操作挂 `write` 组；读操作挂 `read` 组。guest 会话同样持有 CSRF token，无需特判。

## 6. 工具自定义配置规范

工具的可调行为（用例开关、阈值、默认参数等）遵循同一模式，后续工具一律照此设计：

1. **配置三层**：套件/工具自带默认值（代码）→ 管理员永久默认（模块私有表 + 管理后台配置页）→
   任务级覆盖（用户每次使用时调整，随任务入库，优先级最高）。三层按此顺序合并，高层覆盖低层。
   参考实现：`llmadmit` 的 `tool_llmadmit_casecfg` + 后台「准入配置」页 + 表单「用例自定义」。
2. **暴露方式**：模块通过 `GET` 接口返回配置 schema（本工具的用例列表/级别/说明即 schema），
   前端按 schema 渲染表单，不硬编码配置项。
3. **合法性**：模块在创建时校验覆盖项（键存在、取值在枚举内），非法直接 400，不静默忽略。
4. **语义区分**：「不测（skip）」= 不执行也不评；「豁免（exempt）」= 记录为豁免不计失败；
   两者都不改变裁决口径，裁决只由实际执行的结果产生。
5. **留档**：覆盖配置随任务持久化，报告必须能反映「哪些项被自定义过」，不得无痕。

参考实现：`llmadmit` 的用例级覆盖（`runs.case_overrides` + 表单「用例自定义」面板）。

## 7. 前端约定

| 约定 | 内容 |
| --- | --- |
| 组件库 | 一律 HeroUI 原生组件，除非单独指定；不引入第二个组件库，HeroUI 缺失的控件（如取色器）用原生元素兜底并注释原因 |
| API 访问 | 一律经 `src/api/client.ts`；工具后端用 `toolApi(id, path, init)`，禁止裸 `fetch` |
| 目录 | `src/tools-impl/<id>/`：`index.tsx` 入口 + `api.ts`（该工具后端调用）+ 逻辑文件；纯前端工具只是没有 `api.ts` |
| 访问控制 | 前端只信 `/api/tools/:id/access` 的判定，不自行推游客/会员逻辑 |
| 元数据 | 展示信息归后台数据库；`catalog.json` 只做种子、组件映射与构建期资源保护清单 |

## 8. 不做的事

- 不做插件化动态加载：模块编译期注册，加一行代码重编译。
- 不上消息队列/事件总线：工具间共享走 service 函数调用。
- 不给模块独立数据库连接：共享一个 `*gorm.DB`，表前缀隔离。
- 不上微前端/远程组件：继续 `import.meta.glob` 懒加载。
