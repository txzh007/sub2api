# TToken 模型批测：定制边界与上游接入

本功能保持独立文件，便于以后合并 Sub2API。账户批测复用上游测试接口；鹈鹕工作台使用独立路由和表，通过现有网关执行请求。

## 独立维护的文件

| 功能 | 文件位置 |
| --- | --- |
| 账户批测弹窗、队列 | `frontend/src/components/ttoken/account/` |
| 账户测试 SSE 客户端 | `frontend/src/api/ttoken/accountTestStream.ts` |
| 批测共用模型选择器与局部样式 | `frontend/src/components/ttoken/BatchModelPicker.vue`、`batch-test.css`（仅作用于 `.tt-batch`） |
| 鹈鹕工作台、结果卡片、状态管理 | `frontend/src/components/ttoken/pelican/` |
| 鹈鹕页面入口 | `frontend/src/views/admin/PelicanTestView.vue` |
| 鹈鹕 API 客户端 | `frontend/src/api/admin/pelicanTests.ts` |
| 中英文文案 | `frontend/src/i18n/locales/{zh,en}/admin/{batchTest,pelicanTest}.ts` |
| 鹈鹕网关执行与结果读取 | `backend/internal/handler/admin/pelican_test_handler.go` |
| 鹈鹕持久队列、取消、提示词模板 | `backend/internal/handler/admin/pelican_batch_handler.go` |
| 鹈鹕路由注册 | `backend/internal/server/routes/pelican_tests.go` |
| 本次增量迁移 | `backend/migrations/ttoken_001_pelican_batch_prompts.sql` |

## 上游文件中的接入点

1. `AccountsView.vue`：仅将测试弹窗 import 指向 TToken 的 `AccountTestDialog.vue`。上游 `AccountTestModal.vue` 保留原样，仍可从“单模型测试”进入。
2. 中英文 `admin/index.ts`：导入并展开独立文案模块。
3. 既有前端 `router/index.ts` / `AppSidebar.vue`：保留鹈鹕页面路由和导航入口。
4. 既有后端 `handler.go` / `wire.go` / `wire_gen.go`：保留 `PelicanTestHandler` 注入。
5. `server/routes/admin.go`：调用独立文件内的 `registerPelicanTestRoutes`。
6. `server/router.go`：网关路由注册后注入网关并调用 `StartWorkers()`。

上面的第 3、4 项属于已有鹈鹕功能的接入点。本次没有增加账户批测后端接口，也没有修改账户、模型或分组核心表。

## 数据与执行约定

- 已部署的 `241_pelican_test_runs.sql` 不重命名、不修改。新迁移使用 `ttoken_` 前缀，与上游数字编号分开；迁移器按完整文件名识别并按文件名排序执行。后续独立迁移递增为 `ttoken_002_...sql`。
- 新增字段都在鹈鹕专用表内；`pelican_prompt_templates` 按管理员保存模板，不保存 API Key 密钥。
- 每条任务记录完整提示词、输出格式、分组、Key ID、模型和批次 ID。重试创建新记录，使用原任务配置。
- 每批最多 50 个不同模型，每个管理员最多 100 条待执行任务；每个应用实例有 2 个 worker。多实例共享数据库时使用 `SKIP LOCKED` 防止重复领取，并发上限是实例数量乘 2。
- 已排队任务重启后继续执行。运行中任务超过 4 分钟后由历史查询标记为中断，避免自动重复计费。
- 取消先持久化终态，再取消本实例请求；停止不会撤销上游已产生的用量，多实例时其他实例的请求依靠超时退出。
- 账户批测在浏览器中执行，默认串行，可选 1–3 并发，关闭弹窗会取消；鹈鹕在服务端执行，离开页面仍继续，页面可以恢复最近批次。
- SVG 沿用后端校验与前端清理，以图片方式预览；文本和代码按纯文本显示。

## 后续合并方式

合并上游后保留本目录列出的定制文件，逐项复核上述入口，以及 `/admin/accounts/:id/test` 的 SSE 字段、`/v1/models` 和 `/v1/chat/completions` 的兼容性。已执行的迁移不可改名或改内容，需要调整时追加新迁移。

当前仅用于本地预览。真实模型调用与计费结果由管理员在本地页面主动启动后查验。
