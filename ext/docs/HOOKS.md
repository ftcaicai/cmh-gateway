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
| H1-A | 候选与目录准入 | 准入型 | `model/channel_constraint.go` | ChannelSatisfiesFilters / ChannelSatisfiesHostFilters / filterCandidateIDs / ResolveChannelModel / ChannelModelAdmission / PluginExecutorIdentity / ChannelPluginAdmission / publication identity / CheckPluginExecution | 2026-11-13 | 共享映射解析与候选身份投影；当前legacy by-type或实际静态platform resolver owner一律unknown（含未声明模型，双用途保守拒绝）；type61 Save/无request目录用独立publication:/v1/models制品摘要审批，不推出task/provider执行权；DB/cache/pin候选原过滤前准入；CheckPluginExecution独立审批fresh base与最终effective，仅ParamOverride可不同且其它配置投影全等 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 模型能力目录准入 | 准入型 | `model/ability.go` | GetGroupEnabledModels / filterAbilitiesByConstraints / IsModelAdmittedForPublication | 2026-11-13 | 按允许候选过滤客户目录；已安装门禁的查询错误fail-closed | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 客户静态模型目录 | 准入型 | `controller/model.go` | DashboardListModels / RetrieveModel | 2026-11-13 | 默认透传，激活时批量过滤静态/插件目录和单模型retrieve；ListModels经分组能力投影复用 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 官方模型映射复用 | 准入桥接 | `relay/helper/model_mapped.go` | ModelMappedHelper | 2026-11-13 | 从原循环提取host解析API，保持默认mapping/selfmap语义；不代表预扣/派发前完整接线 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 价格目录关联投影 | 准入型 | `model/pricing.go` | GetPricing / GetVendors / updatePricing / GetModelSupportEndpointTypes / GetSupportedEndpointMap | 2026-11-13 | 过滤实际允许能力后构建价格、vendor/group/endpoint；失败不回退旧目录，默认noop保持原行为 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 允许候选owner投影 | 准入型 | `model/model_meta.go` | GetPreferredModelOwnerChannelTypes | 2026-11-13 | 仅允许候选参与优先级owner选择，批量查询不逐模型N+1 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 渠道写前预验 | 准入型 | `model/channel.go` | BatchInsertChannels / Insert / Update / EditChannelByTag / EnableChannelByTag / UpdateChannelStatus / UpdateChannelOtherInfo / UpdateWithAdmissionKey / ValidateChannelAdmissionSnapshot | 2026-11-13 | 安装门禁时按有效快照整批校验、事务内复核；未激活路径透传；控制器更早side effects另续接 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 凭据恢复启用预验 | 准入型 | `model/channel_plugin_credential.go` | UpdateChannelLocked / UpdateTaskPluginChannelLocked / UpdateScheduledChannelLocked | 2026-11-13 | callback后写前核对实际持久化快照，拒绝不得先发布Key/cache | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 控制器变更前准入 | 准入型 | `controller/channel.go` | AddChannel / UpdateChannel / normalizeChannel | 2026-11-13 | 规范化后、凭据检查与首次Key写之前预验；激活时Key+补丁+Ability通过UpdateWithAdmissionKey同事务 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-A | 上游模型自动与手工应用 | 准入型 | `controller/channel_upstream_update.go` | updateChannelUpstreamModelSettings / checkAndPersistChannelUpstreamModelUpdates / applyChannelUpstreamModelUpdates | 2026-11-13 | 将选定模型delta应用于锁内当前行、持久化前准入；能力同步同事务，原noop路径保留 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | 分发与凭据选择前准入 | 准入型 | `middleware/distributor.go` | Distribute / SetupContextForSelectedChannel / bindPluginCandidateView | 2026-11-13 | 公开名覆盖pin分支；选中渠道DB刷新及实际执行者复核在Key取用前；affinity官方merge一次的effective独立审批在新selection context发布前；Setup错误不再忽略 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | 估价与预扣前准入 | 准入型 | `relay/request_billing.go` | CheckRequestAdmission / PrepareRequestBilling | 2026-11-13 | 当前持久化身份复核，拒绝发生在token估算/免费跳过/预扣前；type61精确pinned源/角色/path/有效配置与冻结context比对，不用fresh配置授权旧context；不改资金生命周期 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | 重试追加预留前准入 | 准入型 | `controller/relay.go` | Relay重试循环getChannel后 | 2026-11-13 | 以本次选择身份快照复核，先于PrepareTieredBillingForSelectedGroup，保留原退款策略 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | Responses长连接事件准入 | 准入型 | `relay/responses_websocket.go` | runCall / restoreConnectionContext / checkResponsesWSModelAccess / buildResponsesWSCreatePayload | 2026-11-13 | 每create在ingress前复核、首次选中快照含rawOther冻结恢复、锁定连接DB状态与冻结映射一致检查；payload后/新连接握手后create前复核，拒绝关闭未接管target；native最终create顶层model唯一字符串精确等于权威upstream，noop保持；重试topup前复核 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | Realtime每事件准入 | 准入型 | `relay/channel/openai/relay_realtime.go` | CheckRealtimeAdmission / OpenaiRealtimeHandler | 2026-11-13 | 约束固定握手模型、按解码后大小写敏感规范key检查原始event身份及路径重复键，在plugin前与patch后计量/转发前复核；不改usage逻辑 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-B | Realtime握手前准入 | 准入型 | `relay/websocket.go` | WssHelper初始化ChannelMeta后、DoRequest前 | 2026-11-13 | 实际握手逻辑目标必须先通过固定模型准入，不能联网后才拒绝映射不一致 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | 不可变插件源身份 | 身份投影 | `pkg/jsplugin/registry.go` | LoadedPlugin.SourceSHA256 / SourceBytes / CompilePlugin / ResolveTaskPluginForPlatform / TaskPluginKeyForPlatform | 2026-11-13 | 编译时记录原始源字节SHA256，版本标签不代替制品身份；不自动批准或激活插件 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | 共用插件hook源制品拒绝点 | 准入型（仅源身份） | `pkg/jsplugin/engine.go` | Compile / Engine.SourceSHA256 / Engine.call | 2026-11-13 | Engine保存源SHA，普通source veto typed；显式authenticated task scope精确hook/source/expiry入口及排队后preJS复核，附scope始终nofetch/不回退global绕失效 | `rfc/2026-11-14-task-continuation-authority.md` | ftcaicai |
| H1-C | Provider实际配置执行前准入 | 准入型 | `relay/channel/providerplugin/adaptor.go` | bind / checkExecution / prepareKey / renew | 2026-11-13 | actual RelayInfo/已绑定a.config与fresh行双投影，credential refresh/key exchange/convert前及prepared复用重新检查；不覆盖后台credential服务或opaque最终正文 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task实际提交hook前后准入 | 准入型 | `relay/channel/task/jsplugin/adaptor.go` | checkSubmitAdmission / ValidateRequestAndSetAction / buildSubmit / DoRequest / applyChannelContext / checkQueryDestination / doFetchDescriptor / CaptureContinuationArtifact / RestoreContinuationArtifact / BindContinuationTask / ContinuationChannel / AttachContinuationCredentials / continuationHookError / BindContinuationScope / checkScopedPollKey / pollHookContext | 2026-11-14 | 冻结禁止newsubmit；typedveto只跳本次；有限taskscope绑定精确runtime/task/ref/purpose，static/per_task/no fetch/OAuth；签名dispatch/redirect前期限和exacttarget检查，所有archived真实目标Policy veto跳本次，lateusage不重复JS | `rfc/2026-11-14-task-continuation-authority.md` | ftcaicai |
| H1-C | Task attempt与价格前准入 | 准入型 | `relay/relay_task.go` | RelayTaskSubmit | 2026-11-13 | late TaskAttempt前、validation前后复核；拒绝先于新预扣，不改资金生命周期 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task签名与凭据刷新冻结 | 准入桥接 | `relay/channel/task/jsplugin/credential.go` | signTaskRequest / taskCredentialLine | 2026-11-14 | submit签名冻结info；archived poll/content刷新携原source/base/proxy/config期望，仍精确原account，不用current版本替换旧refresh | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Request stage不可选制品预验 | 准入型 | `relay/requestplugin/plan.go` | Plan.Run | 2026-11-13 | 本stage全部实际scope实例源/原配置/实例角色路径审批先于任何hook/effect；host拒绝不被optional吞掉；仅stage不等于reply/report | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Reply不可选制品预验 | 准入型 | `relay/requestplugin/reply.go` | replyCheck.run | 2026-11-13 | 每个实际reply batch的全部runnable实例先验再hook/释放正文，host拒绝不被optional吞，保留原中断结算 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Report worker消费前预验 | 准入型 | `relay/requestplugin/report.go` | reportJob.run | 2026-11-13 | ordinary/task共用worker消费时整组源+冻结实例配置预验，任一拒绝整个report不联网，不改请求钱账 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Report执行组回归 | 测试 | `relay/requestplugin/requestplugin_test.go` | TestReportExecutorPreflight | 2026-11-13 | 真实optional report双实例本地fetch计数；ordinary/task共用worker、消费时撤销与源/配置/路径变化拒绝 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Provider多步/WS派发准入 | 准入型 | `relay/channel/providerplugin/chain.go` | sendStep / readStep / dialWebSocket / openWebSocket | 2026-11-13 | 每step签名联网/响应转换前、每次dial与初始message前复用actual runtime检查；不等于最终目的地审批 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Provider流转换/回写准入 | 准入型 | `relay/channel/providerplugin/stream.go` | streamConverter runtime check / Read / convertNext | 2026-11-13 | actual info与冻结hook配置一致复核，在读事件前后、转换/queue/回写与pending输出前检查；保留原中断计费 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Relay最终目的地准入 | 准入型 | `relay/channel/api_request.go` | CheckDestination / doRequest / DoApiRequest / DoWssRequest / DoWssRequestWithAdmission / ConnectedTargetAdmission.Check | 2026-11-13 | actual signed URL+effective Host摘要与channel/model/method关系审批，HTTP发送及WS握手前拒绝；active type60已知chat/completions、embedding、Responses/compact、Alpha Search按最终format+mode精确组合校验独立replay的canonical model唯一且精确upstream（含chat→Responses与compact投影）；Moderations+最终OpenAI仅校验存在的canonical model，缺失保持旧语义/默认目标未证，不消费发送体、不泛化opaque/multipart；成功握手mint私有conn绑定proof，每Responses create对原hash重验；普通relay保留3xx不跟随 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Plugin fetch目的地准入 | 准入型 | `service/provider_plugin.go` | PluginFetchExecutor / RequestPluginFetchExecutor / PluginFetcher / RequestPluginFetcher / sendPluginFetch / FreshPluginCredentialLineForContinuation / PluginCredentialRefresher | 2026-11-14 | fetch实际source/capability/target检查；archivedrefresh精确原source/base/proxy/config预验及写前复核，独立flightidentity；普通refresh原合同保留 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task内容最终目标准入 | 准入型 | `controller/video_proxy.go` | proxyTaskMedia / checkTaskContentDestination / taskMediaRedirectClient | 2026-11-14 | 绑定持久任务模型+实际渠道与最终URL/Host/method，发送及redirect前检查；不套新发布门禁或改变钱账 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task内容目标回归 | 测试 | `controller/task_generic_test.go` | TestTaskContentDestinationAdmission | 2026-11-14 | 真实loopback初始拒绝0、批准Range媒体以及第二redirect目标单独审批 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task延续引用持久化 | 值投影 | `model/task.go` | TaskExecutionSnapshot.Continuation | 2026-11-14 | existing private JSON只保存non-secret ref/digest，不新增表列或复制源/配置 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task已存档引用投影 | 值投影 | `service/task_plugin_audit.go` | TaskExecutionSnapshotFromContext | 2026-11-14 | 只有Save成功contextref才随既有controller插入TaskPrivate Execution，失败清空 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task-bound延续解析 | 值投影/准入 | `relay/relay_adaptor.go` | GetTaskAdaptorForTask / ResolveTaskPluginForPlatform / TaskPlatformUnavailableError | 2026-11-14 | 按持久task/user/channel/ref经core可信Load认证后恢复；显式operator有限poll grant才附scope，RestoreBind/ref本身不授权，默认当前policy | `rfc/2026-11-14-task-continuation-authority.md` | ftcaicai |
| H1-C | Archived任务轮询派发 | 准入/值投影 | `service/task_polling.go` | GetTaskContinuationFunc / DispatchPlatformUpdate / updateVideoSingleTask | 2026-11-14 | ref任务独立task-aware解析并复用原poll/CAS；Unavailable本次不计失败，不回当前factory；原timeout保持 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Policy-only启动屏障 | 准入启动桥 | `main.go` | InitResources：359–364，InitEnv/logger之后、DB与worker启动之前 | 2026-11-14 | 显式环境快照经已认证loader一致安装Models/PluginHooks/Executors/Destinations；错误即启动失败，拒绝非空grant并禁用archive；全部缺省保持noop；package init内可信factory编译/top-level元数据不在该屏障之后 | `rfc/2026-11-14-startup-policy-installation-boundary.md` | ftcaicai |
| H1-C | Task-aware factory启动桥 | 值投影 | `main.go` | service.GetTaskContinuationFunc | 2026-11-14 | 注入relay task-bound resolver与frozenchannel，当前channel只取绑定匹配的现有credentialKey；不安装Models/Store | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Archived内容读取恢复 | 值投影/准入 | `controller/task.go` | projectTaskArtifacts / initTaskArtifactAdaptor / taskHasPluginExecution / TaskArtifactContent | 2026-11-14 | ref任务按content用途加载旧源与当前account key，不fallback currentRegistry；缺档固定503 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Task报告原源配置冻结 | 值投影/准入 | `relay/requestplugin/task_report.go` | TaskReport / freezeTaskReports / frozenTaskReportTargets / ReportTask | 2026-11-14 | 可选archive冻结实际source+originalInstance，private safe refs，整批恢复后queue；Frozen缺档/失败不回退当前settings | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Pinned候选typedview | 值DTO接口 | `dto/channel_constraints.go` | ChannelCandidateSnapshot / PluginCandidateView / ChannelFilter.PluginView | 2026-11-14 | key-free host snapshot+coreAdmission接口，真实generation保留于model私有wrapper，不引dto/jsplugin循环或name-number反查 | `rfc/2026-11-13-admission-hook-budget.md` | ftcaicai |
| H1-C | Shared endpoint提前预选同源 | 值投影/准入 | `middleware/task_plugin.go` | RouteTaskPluginEndpoint / PinTaskPluginEndpoint | 2026-11-14 | 预选filter携实际generation typedview，Pin沿selected/captured/current优先序；仅≤40行命名第49文件例外，不改decoder/排序/计费 | `rfc/2026-11-14-shared-endpoint-file-exception.md` | ftcaicai |

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
relay/responses_websocket.go
relay/channel/openai/relay_realtime.go
relay/websocket.go
pkg/jsplugin/registry.go
pkg/jsplugin/engine.go
relay/channel/providerplugin/adaptor.go
relay/channel/task/jsplugin/adaptor.go
relay/relay_task.go
relay/channel/task/jsplugin/credential.go
relay/requestplugin/plan.go
relay/requestplugin/reply.go
relay/requestplugin/report.go
relay/requestplugin/requestplugin_test.go
relay/channel/providerplugin/chain.go
relay/channel/providerplugin/stream.go
relay/channel/api_request.go
service/provider_plugin.go
controller/video_proxy.go
controller/task_generic_test.go
model/task.go
service/task_plugin_audit.go
relay/relay_adaptor.go
service/task_polling.go
controller/task.go
relay/requestplugin/task_report.go
dto/channel_constraints.go
middleware/task_plugin.go
```
