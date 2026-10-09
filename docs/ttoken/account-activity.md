# 账号近期使用：独立定制边界

账号管理增加近 1 小时 / 24 小时统计列，点击打开详情。每 30 秒刷新当前页可见账号；隐藏浏览器页时暂停刷新。详情显示请求量、记录成功率、首字及总耗时均值、账号口径费用、12 段时间分布和最近 30 条记录。异常筛选仅作用于这 30 条记录。

## 接入点

- 后端新增 `ttoken_account_activity.go`，分别位于 `internal/{handler/admin,service,repository}`。通过独立的 repository 接口断言扩展现有 Ops 服务，不修改上游公共接口或依赖注入。
- `server/routes/admin.go` 仅注册 `POST /admin/accounts/activity/batch`，复用账号路由组的管理员鉴权。
- 前端独立文件：`api/ttoken/accountActivity.ts`、`components/ttoken/account/AccountActivity{Cell,Toolbar,Dialog}.vue`、`useAccountActivity.ts`、中英文 `admin/accountActivity.ts`。
- `AccountsView.vue` 接入工具栏、统计列、详情弹窗和 composable；中英文 `admin/index.ts` 各增加文案导入。
- 复用 TToken 的局部 `batch-test.css`，组件样式为 scoped。详情和列表使用局部滚动。
- 不新增迁移、不修改日志写入或网关调度流程。

## 统计口径

- 成功：`usage_logs`，排除在同一时间窗口、同一账号及 request ID 下存在最终失败的记录。失败：`ops_error_logs` 中 HTTP >= 400 或 cyber_policy 的终态记录，以账号和 request ID 去重；排除 count_tokens 请求。
- 成功率 = 成功 / (成功 + 最终失败)。无请求显示 `—`；监控当前关闭也显示 `—`。向下保留一位小数，避免少量失败被四舍五入为 100%。
- 恢复：HTTP < 400 的 provider 归属 upstream/account_auth 错误日志。单独显示、不计入最终失败。账号归属与上游 Ops 保持一致，使用日志 `account_id`；不会展开 `upstream_errors` 中每次重试，也不声称是逐次尝试的成功率。
- 429：以上最终失败或恢复记录中的 upstream_status_code 优先，终态失败缺省使用 status_code。
- 费用沿用上游账号口径 `COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)`；已计费但最终失败的请求仍计入费用。
- 延迟均值仅使用成功记录；首字均值只计算具有 first_token_ms 的记录，缺失值不当作零。
- 依赖已保存日志，受监控开关、过滤、保留期限和异步落盘影响；历史关闭后重新开启无法补齐缺失数据。时间窗口以服务器 UTC 为准，前端按本地时区展示。
- 时间范围限制为 1 / 24 小时；一次最多 100 个账号，详情仅允许 1 个；数据库读取有 8 秒超时。使用既有账号时间索引。

## 手动调度

暂停 / 恢复复用 `accounts/:id/schedulable`；优先级复用原账号更新 API。只提交对应字段，更新后同步列表。展示顺序仍受分组、模型、配额、分组优先级等调度条件影响；暂停不会取消已有请求，恢复不会清除其他临时限制。本功能不自动禁用账号或修改路由。

合并上游时保留独立文件并复核 4 类入口：路由注册、AccountsView、文案 index、Ops repository 的现有日志字段与计费口径。
