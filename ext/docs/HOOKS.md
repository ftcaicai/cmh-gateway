# 挂载点登记表

官方区中**唯一允许被我们修改的文件**全部登记于此。CI 以本表为白名单（`ops/ci/check-upstream-diff.sh`）。规则见 `ext/AGENTS.md` §3。

## 登记格式

每个条目一行。`文件` 是相对仓库根的路径；`行号范围` 在实施后填写并随代码变化更新；`形态` 为 观察 / 接管 / 非代码 / 前端桥接 / 测试。

## 当前登记

| 编号 | 名称 | 形态 | 文件 | 行号范围 | 加入日期 | 用途 | RFC | 负责人 |
|---|---|---|---|---|---|---|---|---|
| R1 | 仓库指引 | 非代码 | `README.md` | 文件顶部 | 2026-11-13 | 在官方 README 顶部加一段指向 `ext/AGENTS.md` 的指引，让协作者和 AI 助手先读宪法 | `rfc/2026-11-13-readme-and-ci.md` | ftcaicai |
| R2 | CI 越界检查 | 非代码 | `.github/workflows/cmh-upstream-diff.yml` | 整个文件（新增） | 2026-11-13 | 对每个 PR 执行官方区 diff 检查，只允许命中本表登记的文件 | `rfc/2026-11-13-readme-and-ci.md` | ftcaicai |
| H2 | `TenantResolver` | 观察 | `main.go` | import 2 行（`extcore` + 空白导入 `_ ".../ext"` 用于启动接线）+ `server.Use(extcore.TenantMiddleware())` 1 行，位于 `RequestId()` 之后 | 2026-11-13 | 按 Host 识别租户写入 context；品牌域路径白名单与未开通域名拒绝；空白导入触发 `ext.init()` 安装全部挂载点实现 | `rfc/2026-11-13-h2-h5.md` | ftcaicai |
| H5 | `RouteRegistrar` | 观察 | `router/main.go` | import 1 行 + `extcore.RegisterRoutes(router)` 1 行，位于 `SetPluginRouter` 之后、web 兜底之前 | 2026-11-13 | 挂载扩展区全部路由：品牌 API、扩展 API、品牌静态站、`/cmh/*` | `rfc/2026-11-13-h2-h5.md` | ftcaicai |
| R3 | 跨应用登录返回 | 前端桥接 | `web/src/features/auth/lib/auth-redirect.ts` | 82–88（复用 sanitizer 后返回 href/document 标志） | 2026-11-13 | 仅规范化后的精确 `/cmh` 或 `/cmh/` 子树使用 document navigation，其他路径不变 | `rfc/2026-11-13-cross-app-login-return.md` | ftcaicai |
| R3 | 跨应用登录成功 | 前端桥接 | `web/src/features/auth/hooks/use-auth-redirect.ts` | 24–27、67–70 | 2026-11-13 | 成功完成官方登录后使用同一安全导航选项 | `rfc/2026-11-13-cross-app-login-return.md` | ftcaicai |
| R3 | 已登录回跳 | 前端桥接 | `web/src/routes/(auth)/sign-in.tsx` | 22、43–46 | 2026-11-13 | 回源确认登录后的 beforeLoad 使用同一安全导航选项 | `rfc/2026-11-13-cross-app-login-return.md` | ftcaicai |
| R3 | 桥接边界回归 | 测试 | `web/src/features/auth/lib/auth-redirect.test.ts` | 23–27、67–92 | 2026-11-13 | 复用既有测试文件验证路径边界及不安全回跳；计入官方文件与行数预算 | `rfc/2026-11-13-cross-app-login-return.md` | ftcaicai |

## 预留（尚未实施，实施前须按 §3.4 补 RFC）

| 编号 | 名称 | 形态 | 预计文件 |
|---|---|---|---|
| H1 | `ModelGate` | 观察 | `middleware/distributor.go`、`controller/channel.go`、`model/pricing.go` |
| H3 | `FundingHooks` | 接管 | `service/billing_session.go` |
| H4 | `PricingResolver` | 观察 | `relay/helper/price.go` |
| H6 | `Migrator` | 观察 | `model/main.go` |
| H7 | `TaskFundingHooks` | 接管 | `service/task_billing.go` |

## 白名单（供 CI 读取，与上表"当前登记"严格一致）

```
README.md
.github/workflows/cmh-upstream-diff.yml
main.go
router/main.go
web/src/features/auth/lib/auth-redirect.ts
web/src/features/auth/hooks/use-auth-redirect.ts
web/src/routes/(auth)/sign-in.tsx
web/src/features/auth/lib/auth-redirect.test.ts
```
