# T1.6c — 用户 quota 写入清单（只读审计，2026-11-13）

**结论：三方法词法清单已逐项核对；不是“全部资金只有四出口”的证明。当前资金挂载点未实施；用户选择的同事务要求不变，本次不改架构、不新增接管点。**

快照：工作目录 `D:\work2\cmh-gateway`；HEAD `7ba2d94fd66dbd63fc11b6f9bc3c73eac1cb0cc3`，含并行工作区未提交内容，行号为本次读取时位置。已完整读取根 [AGENTS.md](<D:/work2/cmh-gateway/AGENTS.md>)、扩展 [AGENTS.md](<D:/work2/cmh-gateway/ext/AGENTS.md>)、[billing.md](<D:/work2/cmh-gateway/.agents/rules/billing.md>)；既有 [funding-lifecycle-audit.md:6–24](<D:/work2/cmh-gateway/ext/docs/acceptance/p1/funding-lifecycle-audit.md#L6-L24>) 已撤回四唯一出口假设，本清单补充实际匹配证据，不替它恢复该假设。

## 1. grep 口径与实数

使用 **grep 工具**，`path=D:\work2\cmh-gateway`、`include=*.go`，主正则：

```regex
\b(DecreaseUserQuota|IncreaseUserQuota|DeltaUpdateUserQuota)\s*\(
```

返回 **24 个匹配行、9 个 Go 文件**，未截断。这里计数是源码行/静态调用点，不是运行次数，也不是资金业务出口数。

| 方法 | 全部匹配 | 定义 | 注释中的伪调用 | 测试调用 | 活跃生产调用（含包装器内部） |
|---|---:|---:|---:|---:|---:|
| `DecreaseUserQuota` | 10 | 1 | 0 | 4 | 5 |
| `IncreaseUserQuota` | 13 | 1 | 1 | 1 | 10 |
| `DeltaUpdateUserQuota` | 1 | 1 | 0 | 0 | 0 |
| **合计** | **24** | **3** | **1** | **5** | **15** |

交叉检查：单独三方法正则分别返回 **10 / 13 / 1**；按目录同主正则：`model/` **11**、`service/` **10**、`controller/` **3**、`relay/` **0**，相加 24；仓库根搜索还包含其余目录，并非只搜这四目录。去掉括号约束的 `\b(DecreaseUserQuota|IncreaseUserQuota|DeltaUpdateUserQuota)\b` 返回 **26** 行，多出的只是 [billing_session.go:281](<D:/work2/cmh-gateway/service/billing_session.go#L281>) 与 [funding_source.go:72](<D:/work2/cmh-gateway/service/funding_source.go#L72>) 两条解释性注释；未见函数值赋值/别名引用。三方法检索在新增本交付文档前，`include=*.{md,js,ts,tsx,json,yml,yaml,sh,ps1}` 同主正则返回 **0**（新增文档本身包含该正则和符号，后续重跑文档过滤结果自然会变化）。无过滤全文件搜索曾超时，未用它声称全类型文件已扫完；Go 清单依据上述成功查询。

## 2. 三方法：全部活跃生产调用点（15）

`db=true` 强制同步 SQL（不表示与调用者事务共用）；`db=false` 允许 BATCH 队列。分类可多选；“direct/legacy”指没有通过 BillingSession 的兼容/独立路径，不代表已证明废弃。

| # | 精确调用位置 | 方法 / db | 所在函数与条件 | 分类 / 财务语义 |
|---:|---|---|---|---|
| 1 | [checkin.go:133](<D:/work2/cmh-gateway/model/checkin.go#L133>) | Increase / true | `userCheckinWithoutTransaction` | reward/direct；SQLite 签到授信，先插签到记录，授信失败删除记录补偿 |
| 2 | [user.go:722](<D:/work2/cmh-gateway/model/user.go#L722>) | Increase / true | `finishInsert`，invitee 奖励 | reward/direct；创建提交后的邀请奖励，错误被 `_` 忽略 |
| 3 | [user.go:779](<D:/work2/cmh-gateway/model/user.go#L779>) | Increase / true | `FinalizeOAuthUserCreation`，invitee 奖励 | reward/direct；另一创建后奖励点，错误被 `_` 忽略；不是鉴权安全审计 |
| 4 | [user.go:1417](<D:/work2/cmh-gateway/model/user.go#L1417>) | Increase / false | `DeltaUpdateUserQuota`，delta > 0 | wrapper/direct；通用正增量；本次全仓 Go 检索未见此包装器的调用者 |
| 5 | [user.go:1419](<D:/work2/cmh-gateway/model/user.go#L1419>) | Decrease / false | `DeltaUpdateUserQuota`，delta < 0 | wrapper/direct；通用负增量；同上，不当作已证明可达业务入口 |
| 6 | [billing_session.go:282](<D:/work2/cmh-gateway/service/billing_session.go#L282>) | Decrease / false | `reserveFunding`，wallet 且不要求可用余额 | reserve；追加预留允许欠费；与严格余额预留的另一路不同 |
| 7 | [billing_session.go:306](<D:/work2/cmh-gateway/service/billing_session.go#L306>) | Increase / false | `rollbackFundingReserve`，wallet | refund/reserve；资金追加预留成功、token 追加失败后的补偿 |
| 8 | [funding_source.go:63](<D:/work2/cmh-gateway/service/funding_source.go#L63>) | Decrease / false | `WalletFunding.Settle`，delta > 0 | settle；实际额高于预扣，补扣允许钱包为负 |
| 9 | [funding_source.go:65](<D:/work2/cmh-gateway/service/funding_source.go#L65>) | Increase / false | `WalletFunding.Settle`，delta < 0 | settle/refund；退结算差额 |
| 10 | [funding_source.go:74](<D:/work2/cmh-gateway/service/funding_source.go#L74>) | Increase / false | `WalletFunding.Refund`，consumed > 0 | refund；退全部预扣；源码注明非幂等，不可重试当作 exactly-once |
| 11 | [midjourney.go:102](<D:/work2/cmh-gateway/service/midjourney.go#L102>) | Increase / false | `RefundMidjourneyQuota`，task.Quota != 0 | MJ/refund/directlegacy；后续 token、统计、日志、清持久化标记独立进行 |
| 12 | [quota.go:457](<D:/work2/cmh-gateway/service/quota.go#L457>) | Decrease / false | `postConsumeQuotaWithResult`，wallet 且 quota > 0 | directlegacy/settle/realtime/violation/MJ；见下表各包装入口 |
| 13 | [quota.go:459](<D:/work2/cmh-gateway/service/quota.go#L459>) | Increase / false | 同函数，wallet 且 quota <= 0 | directlegacy/refund/settle；零额也经过 Increase(0) |
| 14 | [task_billing.go:125](<D:/work2/cmh-gateway/service/task_billing.go#L125>) | Decrease / false | `taskAdjustFunding`，非 subscription 且 delta > 0 | task/settle；异步任务正差额 |
| 15 | [task_billing.go:127](<D:/work2/cmh-gateway/service/task_billing.go#L127>) | Increase / false | 同函数，delta <= 0 | task/settle/refund；任务负差额及失败退款（零差额亦可能入包装器） |

### 定义、注释、测试单列（不混入 15 个生产调用）

| 种类 | 精确位置 | 数量 / 说明 |
|---|---|---|
| 定义 | [user.go:1340](<D:/work2/cmh-gateway/model/user.go#L1340>)、[user.go:1387](<D:/work2/cmh-gateway/model/user.go#L1387>)、[user.go:1412](<D:/work2/cmh-gateway/model/user.go#L1412>) | 3；Increase / Decrease / DeltaUpdate |
| 注释伪调用 | [user.go:726](<D:/work2/cmh-gateway/model/user.go#L726>) | 1；被注释的 inviter Increase，不执行；实际 inviter 写的是 aff_quota，并非 users.quota |
| 测试调用 | [plugin_native_e2e_test.go:49](<D:/work2/cmh-gateway/controller/plugin_native_e2e_test.go#L49>)、[同文件:60](<D:/work2/cmh-gateway/controller/plugin_native_e2e_test.go#L60>)、[同文件:78](<D:/work2/cmh-gateway/controller/plugin_native_e2e_test.go#L78>) | 3；`nativeRouteBilling` fixture 的补扣/退款/预留，db=true；不是生产资金实现 |
| 测试调用 | [quota_reserve_test.go:147–148](<D:/work2/cmh-gateway/model/quota_reserve_test.go#L147-L148>) | 2；两次最大请求扣款聚合测试，db=false；本次只读，未运行 |

## 3. 包装链：三方法 grep 不能替代的生命周期

| 分类 | 已读入口/调用点 | 实际链与边界 |
|---|---|---|
| preconsume | [billing.go:24–55](<D:/work2/cmh-gateway/service/billing.go#L24-L55>) → [billing_session.go:208–259](<D:/work2/cmh-gateway/service/billing_session.go#L208-L259>) → [funding_source.go:42–54](<D:/work2/cmh-gateway/service/funding_source.go#L42-L54>) | **钱包首次预扣不是 Decrease**：`WalletFunding.PreConsume` → `TryReserveUserQuota`；token 先预扣，资金失败后另行补 token |
| strict reserve | [billing_session.go:264–275](<D:/work2/cmh-gateway/service/billing_session.go#L264-L275>) | requireAvailableQuota=true 仍走 `funding.PreConsume` → 原子检查余额；其他追加预留走表中 #6 |
| reserve callers | [billing.go:42](<D:/work2/cmh-gateway/service/billing.go#L42>)、[image_billing.go:81](<D:/work2/cmh-gateway/service/image_billing.go#L81>)、[tiered_settle.go:233](<D:/work2/cmh-gateway/service/tiered_settle.go#L233>)、[controller/relay.go:782](<D:/work2/cmh-gateway/controller/relay.go#L782>) | 重试提交前、图片数量、所选组估计、任务提交后的上调；`reserve` 还依据图片请求强制严格预留，token 失败触发 #7 |
| settle/refund | [billing_session.go:47–84](<D:/work2/cmh-gateway/service/billing_session.go#L47-L84>)、[同文件:88–128](<D:/work2/cmh-gateway/service/billing_session.go#L88-L128>) | 分别调用 funding.Settle / funding.Refund；资金、token 两步，不是共同 SQL 事务；内存 settled/fundingSettled/refunded 不等于持久幂等账本 |
| directlegacy | [billing.go:103–106](<D:/work2/cmh-gateway/service/billing.go#L103-L106>) → [quota.go:435–476](<D:/work2/cmh-gateway/service/quota.go#L435-L476>) | 无 BillingSession 时 `SettleBilling` → `PostConsumeQuota` → #12/#13；资金成功后再 token，FundingApplied/TokenApplied 分开记录 |
| realtime | [relay_realtime.go:339](<D:/work2/cmh-gateway/relay/channel/openai/relay_realtime.go#L339>)、[同文件:425–448](<D:/work2/cmh-gateway/relay/channel/openai/relay_realtime.go#L425-L448>) → [quota.go:90–156](<D:/work2/cmh-gateway/service/quota.go#L90-L156>) | `preConsumeUsage` → `PreWssConsumeQuota` → `PostConsumeQuota`（151）→ #12；名称虽叫 Pre，语义是已返回用量逐 response 扣费，末尾残余用量也可进入；UsePrice 分支跳过此逐事件扣费 |
| violation | [violation_fee.go:101–125](<D:/work2/cmh-gateway/service/violation_fee.go#L101-L125>) | `ChargeViolationFeeIfNeeded` 在正常流程（包括退款）后独立调用 `PostConsumeQuota` → #12；不能假定与原请求退款同一财务事件 |
| MJ | [midjourney.go:62–92](<D:/work2/cmh-gateway/service/midjourney.go#L62-L92>)、[同文件:96–136](<D:/work2/cmh-gateway/service/midjourney.go#L96-L136>) | 持久任务后 `postConsumeQuotaWithResult` → #12；失败走 #11；订阅不支持；任务标记与扣退款非同一提交 |
| task | [task_billing.go:267–307](<D:/work2/cmh-gateway/service/task_billing.go#L267-L307>)、[同文件:314–380](<D:/work2/cmh-gateway/service/task_billing.go#L314-L380>) | `RefundTaskQuota`（274）/`RecalculateTaskQuota`（336）→ #14/#15；资金、token、统计、日志、task.Quota 回写独立；本次不证明后台恢复 exactly-once |
| task polling/recovery | [task_polling.go:130–141](<D:/work2/cmh-gateway/service/task_polling.go#L130-L141>)、[model/task.go:413–436](<D:/work2/cmh-gateway/model/task.go#L413-L436>) | 超时先写终态 CAS 再退款；常规扫描排除失败/成功终态；说明需要考虑终态已写、资金未写窗口，而不是宣称必然重试完成 |

## 4. users.quota 原语、ORM/字段赋值与 raw SQL（独立补查）

这是 **users.quota 相关补充清单，不是所有 money/subscription/token/渠道统计写入的穷尽证明**。已用根目录 `*.go` 检索 `Update("quota"`、`"quota":`、`["quota"] =`、`.Quota =/+=/-=`、`User{`、`Save(user)`、`BatchUpdateTypeUserQuota` 及包装器名，再读取上下文排除同名统计字段。

| 位置（实际写入点） | 分类 / 入口 | SQL/事务/缓存事实 |
|---|---|---|
| [user.go:1367–1370](<D:/work2/cmh-gateway/model/user.go#L1367-L1370>) | Increase 底层 | 全局 DB 条件 `quota <= MaxWalletQuota-quota` 后 `quota + ?`；不接收调用方 tx |
| [user.go:1404–1405](<D:/work2/cmh-gateway/model/user.go#L1404-L1405>) | Decrease 底层 | 全局 DB `quota - ?`；无余额条件，也未以 RowsAffected 校验用户存在 |
| [user.go:1472–1485](<D:/work2/cmh-gateway/model/user.go#L1472-L1485>)；调用 [utils.go:122](<D:/work2/cmh-gateway/model/utils.go#L122>) | batch | 一条用户行 Updates 合并 quota/used_quota/request_count；非整个账务事务，错误只记日志 |
| [quota_reserve.go:112](<D:/work2/cmh-gateway/model/quota_reserve.go#L112>)、[同文件:143–146](<D:/work2/cmh-gateway/model/quota_reserve.go#L143-L146>) | preconsume/reserve | `persistUserQuotaDelta` 缓存预扣后 SQL 加负增量或入队；`reserveUserQuotaDB` 条件 `quota >= ?` 同步扣款。入口 [TryReserveUserQuota:164–197](<D:/work2/cmh-gateway/model/quota_reserve.go#L164-L197>) |
| [checkin.go:103–104](<D:/work2/cmh-gateway/model/checkin.go#L103-L104>) | reward/direct | MySQL/PostgreSQL 签到：签到 Create + 用户增额在 tx；提交后异步 cache 增量（117）；SQLite 另走表 #1 |
| [topup.go:98–102](<D:/work2/cmh-gateway/model/topup.go#L98-L102>) | payment/admin/redeem | `creditTopUpQuota(tx,...)` 在传入 tx 中条件授信；调用者的订单/兑换状态与授信共 tx，但 Redis 在提交之后 |
| [subscription.go:808–809](<D:/work2/cmh-gateway/model/subscription.go#L808-L809>) | payment/direct | `PurchaseSubscriptionWithBalance`：锁用户并扣钱包 + 创建订阅/订单共 tx；提交后 [850](<D:/work2/cmh-gateway/model/subscription.go#L850>) cacheDecr |
| [user_quota_adjustment.go:62](<D:/work2/cmh-gateway/model/user_quota_adjustment.go#L62>) | admin | `AdjustUserQuota` 锁行后 add/subtract/override 算 after，绝对值写入；提交后 [81](<D:/work2/cmh-gateway/model/user_quota_adjustment.go#L81>) 只同步已提交差额；入口 [controller/user_quota.go:46](<D:/work2/cmh-gateway/controller/user_quota.go#L46>) |
| [user.go:613–616](<D:/work2/cmh-gateway/model/user.go#L613-L616>) | promotion/direct | `TransferAffQuotaToQuota`：锁用户，AffQuota 减、Quota 加，`tx.Save(user)` 后提交（621）；此函数未见 quota cache 同步；入口 [controller/user.go:432](<D:/work2/cmh-gateway/controller/user.go#L432>) |
| [user.go:681/691](<D:/work2/cmh-gateway/model/user.go#L681-L691>)、[同文件:745/754](<D:/work2/cmh-gateway/model/user.go#L745-L754>) | initial grant | `Insert` / `InsertWithTx` 创建用户时将 Quota 设为 QuotaForNewUser，Create 写入；不是已有余额 UPDATE |
| [model/main.go:86–88](<D:/work2/cmh-gateway/model/main.go#L86-L88>)、[controller/setup.go:97–99](<D:/work2/cmh-gateway/controller/setup.go#L97-L99>) | admin/bootstrap initial grant | 两处 rootUser 字面量 + Create 的初始余额；不复述同文件中的凭据信息 |
| [migration_v0.2-v0.3.sql:1–6](<D:/work2/cmh-gateway/bin/migration_v0.2-v0.3.sql#L1-L6>) | historical migration/raw SQL | **真实原始 SQL**：`UPDATE users SET quota = quota + (SELECT SUM(remain_quota) FROM tokens WHERE tokens.user_id = users.id)`；不是三方法调用；未执行，未建立其在当前启动流程的可达性 |

### payment/redeem 包装器的全部生产调用（本次该名字 grep：7 次调用 + 1 定义）

| 入口 | creditTopUpQuota 调用行 | 提交后的缓存同步行 |
|---|---|---|
| `RechargeEpay` | [topup.go:218](<D:/work2/cmh-gateway/model/topup.go#L218>) | [229](<D:/work2/cmh-gateway/model/topup.go#L229>) |
| `Recharge`（Stripe） | [topup.go:276](<D:/work2/cmh-gateway/model/topup.go#L276>) | [285](<D:/work2/cmh-gateway/model/topup.go#L285>) |
| 管理员补单 | [topup.go:506](<D:/work2/cmh-gateway/model/topup.go#L506>) | [521](<D:/work2/cmh-gateway/model/topup.go#L521>) |
| `RechargeCreem` | [topup.go:583](<D:/work2/cmh-gateway/model/topup.go#L583>) | [590](<D:/work2/cmh-gateway/model/topup.go#L590>) |
| `RechargeWaffo` | [topup.go:641](<D:/work2/cmh-gateway/model/topup.go#L641>) | [648](<D:/work2/cmh-gateway/model/topup.go#L648>) |
| `RechargeWaffoPancake` | [topup.go:701](<D:/work2/cmh-gateway/model/topup.go#L701>) | [708](<D:/work2/cmh-gateway/model/topup.go#L708>) |
| `Redeem` | [redemption.go:177](<D:/work2/cmh-gateway/model/redemption.go#L177>) | [183](<D:/work2/cmh-gateway/model/redemption.go#L183>) |

排除项：通用 [User.UpdateWithTx:835–844](<D:/work2/cmh-gateway/model/user.go#L835-L844>) 显式 Omit quota/用量/推广字段，[EditWithTx:880–901](<D:/work2/cmh-gateway/model/user.go#L880-L901>) 更新字段集不含 quota；[usedata.go:135–145](<D:/work2/cmh-gateway/model/usedata.go#L135-L145>) 是 quota_data 统计表，[task.go:578](<D:/work2/cmh-gateway/model/task.go#L578>) 是任务退款标记，均不能误算 users.quota。测试直接字段写入已与生产分离：例如 [plugin_native_e2e_test.go:378](<D:/work2/cmh-gateway/controller/plugin_native_e2e_test.go#L378>)、[responses_websocket_test.go:360](<D:/work2/cmh-gateway/controller/responses_websocket_test.go#L360>)、[redemption_test.go:153](<D:/work2/cmh-gateway/model/redemption_test.go#L153>)、[user_update_test.go:70](<D:/work2/cmh-gateway/model/user_update_test.go#L70>) / [168](<D:/work2/cmh-gateway/model/user_update_test.go#L168>) / [223](<D:/work2/cmh-gateway/model/user_update_test.go#L223>)；补查不试图逐行列出全部测试 User Create fixture。

raw SQL 证据：`*.sql` 的 `(?i)\busers\b|\bquota\b` 返回 **3 行**，均在上述历史迁移；根目录 `*.{go,sql,js,ts,tsx,ps1,sh}` 的 `(?i)\bUPDATE\s+["`]?users\b|\busers\.quota\b` 返回 **5 行**：迁移一行 + 四条 “No permission to update users...” 字符串误命中，已读相关上下文排除。Go 相同 UPDATE 前缀查询只有后四条字符串，**未发现该正则命中的 Go 原始 UPDATE users**。这不排除动态构造 SQL、其他文件类型、外部脚本或运维写库；曾尝试跨行正则被工具拒绝，已改用成功的单行前缀查询及迁移全文读取，不把失败查询当证据。

## 5. BATCH × Redis 分歧（源码事实，不冒充运行验收）

| 场景 | SQL / Redis 行为 | 对同事务要求的影响 |
|---|---|---|
| Increase，db=false + BATCH | [user.go:1347–1354](<D:/work2/cmh-gateway/model/user.go#L1347-L1354>) 先内存入队、异步 cache 增额，立即返回 nil；其他模式先 SQL 成功再异步 cache（1356–1363） | 返回成功不等于 SQL 已提交；入队路径不执行同步授信的余额上限谓词 |
| Decrease，任何 db | [user.go:1391–1401](<D:/work2/cmh-gateway/model/user.go#L1391-L1401>) 先启动异步 cache 扣减，再 SQL/入队；SQL 失败没有此处的 cache 补偿 | db=true 也不是 cache+SQL 原子；失败顺序与 Increase 不同 |
| reserve，Redis 关闭 | [quota_reserve.go:171–172](<D:/work2/cmh-gateway/model/quota_reserve.go#L171-L172>) 条件 SQL 同步扣款，**即使 BATCH 开也不入队** | 与 Decrease(false) 的 BATCH 语义不同 |
| reserve，Redis 命中 | [quota_reserve.go:175–197](<D:/work2/cmh-gateway/model/quota_reserve.go#L175-L197>) Lua 检查/扣缓存 → SQL 或 BATCH；同步 SQL 失败尝试反向 Lua | Lua 与 SQL 是补偿式两步，不是同一事务；BATCH 延后失败不在此同步补偿范围 |
| reserve，Redis 错误/水合失败 | [quota_reserve.go:181–185](<D:/work2/cmh-gateway/model/quota_reserve.go#L181-L185>) 降级条件 SQL | 已存在待刷队列时 SQL/缓存余额可能不同；本次未故障注入，不宣称已复现超扣或已消除风险 |
| cache miss / hydrate | [user_cache.go:99–126](<D:/work2/cmh-gateway/model/user_cache.go#L99-L126>) 从 DB 水合；[159–164](<D:/work2/cmh-gateway/model/user_cache.go#L159-L164>) 的 delta Lua 对缺失/不匹配 hash 跳过，错误与 cache miss 不同 | 待刷 SQL 增量与水合时序需验证；不伪造余额、不用缓存成功替代落库成功 |
| batch flush | [utils.go:24–31](<D:/work2/cmh-gateway/model/utils.go#L24-L31>) 进程内 map；[83–87](<D:/work2/cmh-gateway/model/utils.go#L83-L87>) 先换空 map，再 [122](<D:/work2/cmh-gateway/model/utils.go#L122>) 刷用户；[user.go:1483–1485](<D:/work2/cmh-gateway/model/user.go#L1483-L1485>) 失败只日志 | 源码此处未见持久重试/重新入队；进程崩溃/刷库失败恢复不能据此判定守恒已通过 |
| payment/admin | [syncCreditUserQuotaCache:171–180](<D:/work2/cmh-gateway/model/user_cache.go#L171-L180>) 和 [AdjustUserQuota:77–83](<D:/work2/cmh-gateway/model/user_quota_adjustment.go#L77-L83>) 在 tx 后同步增量；签到异步、推广转入未见同步 | 三方法覆盖不到这些分歧；“同步缓存”也不是 SQL 同事务 |
| token/任务标记 | [token.go:382–438](<D:/work2/cmh-gateway/model/token.go#L382-L438>) 独立 cache/批量；[task_billing.go:274–305](<D:/work2/cmh-gateway/service/task_billing.go#L274-L305>) 资金成功后再 token/清 quota | 用户钱包、API token 限额、任务终态/退款标记无共同提交证据；不能入口 handled-return 跳过其语义 |

## 6. rc51同步后的复核（2026-11-14）

在当前af96568fd /官方基线rc51重新用grep工具同正则全仓Go检索，仍为24匹配/9文件，具体行号与本清单一致（Increase13/Decrease10/Delta1；定义3/注释1/测试5/生产15）。另执行 `git diff v1.0.0-rc.50 v1.0.0-rc.51 --` 上述9个文件，输出为空，说明这组调用未随本次官方升级变更。本次只核对原始源码与版本差异，不新增金融hook或声称BATCH/Redis守恒已验。
## 7. 验收边界（原快照）

- 完成的是本快照 **三个明确 Go 符号的全部成功 grep 匹配分类**及相关 wrappers/用户余额写入补查；15 个生产调用逐点列出，5 个测试调用分组保留准确行号。不是 AST/运行时可达性证明，不声称覆盖仓库外、运行时 SQL、忽略/二进制/生成文件或全部资金实体。
- 不据三方法清单推导唯一共同资金出口；预扣原语、追加预留、直接结算、Realtime、违规费、MJ、后台任务、授信/管理员/迁移均有不同边界。H3/H7 当前仍为保留设计编号，**本次没有新增财务 hooks**，同事务选择未更改。
- 只做静态读取/grep/git 状态核对，仅新增本文件；不改 Go、测试、父代理的插件投影/HOOKS/CURRENT_TASK；未提交/推送，未运行测试、三库、Redis 故障注入、生产请求或付费请求。故没有运行时守恒、幂等、三库兼容性通过结论；后续若实现需独立 RFC 与真实三库、Redis×BATCH 四组合及失败/重放/重启证据。
