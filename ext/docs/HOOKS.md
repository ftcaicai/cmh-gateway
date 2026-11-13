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
| R4 | 自有插件工厂清单 | 测试 | `plugins/builtin_plugins_test.go` | requestPluginHooks 字典中的 cmh-param-normalize 条目 | 2026-11-13 | 保留精确目录集合断言并验证自有 outbound 工厂插件 | `rfc/2026-11-13-plugin-factory-inventory.md` | ftcaicai |
| H1-A | 候选与目录准入 | 准入型 | `model/channel_constraint.go` | ChannelSatisfiesFilters / filterCandidateIDs / ResolveChannelModel / ChannelModelAdmission | 2026-11-13 | 共享映射解析与候选身份投影；DB/cache/pin候选在原过滤前准入 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 模型能力目录准入 | 准入型 | `model/ability.go` | GetGroupEnabledModels / filterAbilitiesByConstraints / IsModelAdmittedForPublication | 2026-11-13 | 按允许候选过滤客户目录；已安装门禁的查询错误fail-closed | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 客户静态模型目录 | 准入型 | `controller/model.go` | DashboardListModels / RetrieveModel | 2026-11-13 | 默认透传，激活时批量过滤静态/插件目录和单模型retrieve；ListModels经分组能力投影复用 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 官方模型映射复用 | 准入桥接 | `relay/helper/model_mapped.go` | ModelMappedHelper | 2026-11-13 | 从原循环提取host解析API，保持默认mapping/selfmap语义；不代表预扣/派发前完整接线 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 价格目录关联投影 | 准入型 | `model/pricing.go` | GetPricing / GetVendors / updatePricing / GetModelSupportEndpointTypes / GetSupportedEndpointMap | 2026-11-13 | 过滤实际允许能力后构建价格、vendor/group/endpoint；失败不回退旧目录，默认noop保持原行为 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 允许候选owner投影 | 准入型 | `model/model_meta.go` | GetPreferredModelOwnerChannelTypes | 2026-11-13 | 仅允许候选参与优先级owner选择，批量查询不逐模型N+1 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 渠道写前预验 | 准入型 | `model/channel.go` | BatchInsertChannels / Insert / Update / EditChannelByTag / EnableChannelByTag / UpdateChannelStatus / UpdateChannelOtherInfo / UpdateWithAdmissionKey / ValidateChannelAdmissionSnapshot | 2026-11-13 | 安装门禁时按有效快照整批校验、事务内复核；未激活路径透传；控制器更早side effects另续接 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 凭据恢复启用预验 | 准入型 | `model/channel_plugin_credential.go` | UpdateChannelLocked / UpdateTaskPluginChannelLocked / UpdateScheduledChannelLocked | 2026-11-13 | callback后写前核对实际持久化快照，拒绝不得先发布Key/cache | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 控制器变更前准入 | 准入型 | `controller/channel.go` | AddChannel / UpdateChannel / normalizeChannel | 2026-11-13 | 规范化后、凭据检查与首次Key写之前预验；激活时Key+补丁+Ability通过UpdateWithAdmissionKey同事务 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 上游模型自动与手工应用 | 准入型 | `controller/channel_upstream_update.go` | updateChannelUpstreamModelSettings / checkAndPersistChannelUpstreamModelUpdates / applyChannelUpstreamModelUpdates | 2026-11-13 | 将选定模型delta应用于锁内当前行、持久化前准入；能力同步同事务，原noop路径保留 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | 分发与凭据选择前准入 | 准入型 | `middleware/distributor.go` | Distribute / SetupContextForSelectedChannel | 2026-11-13 | 公开名覆盖pin分支；选中渠道DB刷新及实际执行者复核在Key取用前；Setup错误不再忽略 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | 估价与预扣前准入 | 准入型 | `relay/request_billing.go` | CheckRequestAdmission / PrepareRequestBilling | 2026-11-13 | 当前持久化身份复核，拒绝发生在token估算/免费跳过/预扣前；不改资金生命周期 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | 重试追加预留前准入 | 准入型 | `controller/relay.go` | Relay重试循环getChannel后 | 2026-11-13 | 以本次选择身份快照复核，先于PrepareTieredBillingForSelectedGroup，保留原退款策略 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |

## 预留（尚未实施，实施前须按 §3.4 补 RFC）

| 编号 | 名称 | 形态 | 预计文件 |
|---|---|---|---|
| H1 | `ModelGate` | 准入型（专项已批准，core/domestic纯合同先行，官方接线待验） | 按 `rfc/2026-11-13-admission-hook-budget.md` A/B/C逐项登记；未接线文件不进入当前白名单 |
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
plugins/builtin_plugins_test.go
model/channel_constraint.go
model/ability.go
controller/model.go
relay/helper/model_mapped.go
model/pricing.go
model/model_meta.go
model/channel.go
model/channel_plugin_credential.go
controller/channel.go
controller/channel_upstream_update.go
middleware/distributor.go
relay/request_billing.go
controller/relay.go
```
