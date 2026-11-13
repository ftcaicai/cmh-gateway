# 以 new-api 官方为基座建设新系统：完整方案（v0.5，2026-11-13）

配套文件：`AGENTS.md`（新仓库宪法）、`OEM-MIGRATION-TASKS.md`（OEM 功能移植任务与验收）。
本文件回答：为什么这样做、做成什么样、挂在哪里、怎么分期、怎么验收、风险在哪、还有什么没定。

## 定位（2026-11-13 负责人确认）

**新仓库是一个全新系统，不是旧系统的替换品。**

- 旧系统继续独立运行，旧客户、旧余额、旧渠道、旧品牌都留在旧系统，**不做数据迁移、不做灰度、不做切流**。
- 新系统从零开始接客户，后续产品迭代以新仓库为主。
- 旧系统里有用的功能，由负责人逐项下指令"移植"到新系统；移植的是业务规则和能力，不是数据。
- 两套系统各自有自己的域名、数据库、渠道配置和客户。

这个定位去掉了原方案里风险最高的部分（数据迁移、并行对账、切流回滚），代价是新系统上线初期没有存量客户和存量流量，验收只能靠固定验收账号和合成数据。

事实来源：官方 `main`（2026-11-12，rc.49 之后）的 `AGENTS.md`、`.agents/rules/billing.md`、`docs/plugin-api/README.md` 与 `v1.md`、`main.go`、`router/main.go`、`model/main.go`、`relay/request_billing.go`、`service/quota.go`、`middleware/distributor.go`、`web/rsbuild.config.ts`、`web/src/routes/__root.tsx`。以下标注"[官方]"的结论都来自这些文件，不是推测。

---

## 第一部分：决策依据

### 1.1 为什么不能在现有仓库上同步官方

| 事实 | 数字 |
|---|---|
| 现仓库非测试 Go 文件 | 1,043 个，其中 576 个（55%）官方没有 |
| 核心文件被改写幅度 | `service/task_polling.go` 2.2 倍、`middleware/auth.go` 2.0 倍、`router/api-router.go` 1.9 倍、`controller/relay.go` 1.7 倍 |
| 版本差距 | 我们基于 rc.11，官方 rc.49；官方已重写任务架构（Go 适配器 → JS 插件）、抽出 `relaykit`/`tokenkit`、重做前端 |
| 编号冲突 | 我们的渠道类型 58（H3）、59（灵境）与官方 58（AdvancedCustom）、59（Sub2API）撞号 |

第一次 `merge upstream/main` 就会在鉴权、计费、路由上产生上千处冲突，而且这些正是最不能出错的地方。所以结论是：**另起仓库，业务按分层重建**。

### 1.2 官方提供了哪些可依赖的扩展面 [官方]

| 扩展面 | 位置 | 能做什么 | 不能做什么 |
|---|---|---|---|
| Task Plugin（JS） | `plugins/tasks/<key>/plugin.js`，走渠道类型 61 | 提交/轮询/解析上游异步任务，声明路由、模型、计费单位 `usageSchema` | 无 fetch/文件/异步/eval；不能算价、不能结算；只报用量事实 |
| Provider Plugin（JS） | 同上，`meta.relay` | 把 OpenAI/Claude/Gemini 请求转成私有协议中转 | 同上沙箱限制 |
| Request-processing Plugin（JS） | 同上，`ingress`/`outbound` 等阶段钩子 | 改写任意渠道的请求/响应（参数归一、角色转换、响应 model 归一） | 同上 |
| 高级自定义渠道（type 58） | 控制台配置 | 用路由预设接 OpenAI 兼容但路径/鉴权特殊的上游 | 不能写逻辑 |
| 计费表达式 | `pkg/billingexpr`、`setting/billing_setting` | 阶梯、分辨率、峰谷等动态定价 | — |
| Casbin 授权 | `service/authz/` | 角色与权限策略，多节点同步 | — |
| 节点同步主题 | `service.RegisterSyncTopic` | 配置变更秒级广播到所有节点 | — |
| 系统任务 | `controller.RegisterScheduledSystemTasks` + `service.StartSystemTaskRunner` | 带 DB 租约去重的定时任务 | — |

**以上这些都不需要改官方代码就能用。** 我们的目标是：能用这些做的全用这些做，剩下的收敛到少数挂载点。

### 1.3 官方规则对我们的硬约束 [官方]

- 官方 `AGENTS.md` 要求 `docs/` 及 `plugins/tasks/<plugin>/` 内不新增文档（原文留有"除非用户明确要求"的例外）。我们作为 fork 所有者可以豁免，但**主动选择不豁免**：这两个目录归官方区，我们的文档一律放 `ext/docs/`，避免同步时在这些目录产生冲突。
- 触碰计费的改动必须先完整读 `.agents/rules/billing.md`，并遵守其全部不变量（用量多重因子必须有上界、量化必须走 `common/quota_math.go`、不得手写 `int()` 转换等）。我们的 `ext/wallet` 同样要守这些规则。
- 鉴权相关改动必须符合 OWASP ASVS，并在 PR 里记录验证。OEM 品牌登录、团队登录都属于此范围。
- 数据库改动必须在真实 SQLite/MySQL/PostgreSQL 三库上验证，不接受只测一种。我们的 `cmh_*` 表同样适用。
- 前端 UI 必须优先复用官方 `web/src/components/` 既有组件，新造组件要说明理由。

---

## 第二部分：目标架构

### 2.1 仓库骨架（修订版）

```
<repo>/                              官方 fork
├── （官方全部目录）                   官方区，只读
├── ext/                             扩展区：全部后端业务
│   ├── core/                        挂载点接口、注册器、默认空实现
│   ├── domestic/                    国产模型白名单、渠道类型白名单
│   ├── pricing/                     售价/成本/结算价版本化、价格门禁
│   ├── wallet/                      统一钱包：预扣、结算、退款、对账、恢复器
│   ├── brand/                       OEM：租户、客户、团队、代理商、品牌价格、品牌站渲染
│   ├── promotion/                   推广人、返佣、提现
│   ├── invoice/                     发票
│   ├── settlement/                  官方 Key 结算
│   ├── publication/                 上架门禁、固定验收账号
│   ├── enterprise/                  企业空间
│   ├── maintenance/                 HMAC 本机控制面
│   ├── migrations/                  扩展区表（cmh_ 前缀）
│   ├── docs/                        我们的全部文档：规则、RFC、挂载点说明、同步日志
│   │   ├── HOOKS.md                 挂载点登记表
│   │   ├── rfc/                     改官方代码前的提案
│   │   └── sync-log.md              每次同步官方的记录
│   └── register.go                  唯一入口 ext.Register(...)
├── plugins/tasks/cmh-*/             我们的插件（只放 plugin.js、icon、fixture，不放文档）
├── web/                             官方前端，只读
├── web-cmh/                         我们的独立前端应用（见 2.4）
├── brandweb/                        OEM 品牌静态站（Go 模板，由 ext/brand 渲染）
└── ops/                             构建、发布、运维、CI 检查脚本
```

与 v0.1 的差别：`docs/` 改为 `ext/docs/`（官方禁止在 `docs/` 加文件）；插件目录不放 README；前端从"注入官方"改为"独立应用"。

### 2.2 挂载点：精确落点与形态 [官方位置已核对]

挂载点形态固定：`ext/core` 里定义接口和包级变量（默认空实现），官方文件里只加**一行调用**。

挂载点分两类形态：
- **观察型**（H1、H2、H4、H5、H6）：接口变量 + 默认空实现，官方代码加一行调用，不改变官方控制流。
- **接管型**（H3、H7）：接口返回 `handled bool`；返回 `true` 时官方**跳过**自己的资金操作，由 ext 完成；返回 `false` 时官方照常执行。只有这样品牌钱包才能与官方扣款不重复、不漏扣。

| 编号 | 名称 | 形态 | 官方文件 · 位置 | 注入内容 | 预计 diff |
|---|---|---|---|---|---|
| H1 | `ModelGate` | 观察 | `middleware/distributor.go` · `Distribute()` 选渠道前；`controller/channel.go` · `AddChannel`/`UpdateChannel` 保存前；`model/pricing.go` · `updatePricing()` 投影时过滤 | 国产白名单、渠道类型白名单、上架门禁 | 3 文件 × ≤10 行 |
| H2 | `TenantResolver` | 观察 | `main.go` · `server.Use(...)` 链中、`RequestId()` 之后 | 按 Host 识别品牌写入 context；品牌域路径白名单；未开通域名 421 | 1 文件 × 1 行 |
| H3 | `FundingHooks` | **接管** | `service/billing_session.go` · `preConsume`（预扣）、`Settle`（结算）、`Refund`（退款）三处 | 品牌客户：ext 从品牌钱包预扣/结算/退款，官方跳过 `DecreaseUserQuota`/`IncreaseUserQuota`；非品牌客户返回 `false`。推广返佣、发票额度、利润快照作为结算后的派生事件，由 ext 在自己的事务内写 | 1 文件 × ≤30 行 |
| H4 | `PricingResolver` | 观察 | `relay/helper/price.go` · `ModelPriceHelper` 与 `ModelPriceHelperPerCall` 返回前 | 分组+模型售价版本、客户专属价、OEM 结算价 | 1 文件 × ≤12 行 |
| H5 | `RouteRegistrar` | 观察 | `router/main.go` · `SetRouter` 内、`SetWebRouter` 之前 | `/api/brand/*`、`/api/cmh/*`、品牌静态站、`/cmh/*` 静态资源 | 1 文件 × 1 行 |
| H6 | `Migrator` | 观察 | `model/main.go` · `migrateDB()` 末尾 | `cmh_*` 表 | 1 文件 × 1 行 |
| H7 | `TaskFundingHooks` | **接管** | `service/task_billing.go` · `taskAdjustFunding`（任务结算/退款的唯一资金出口） | 同 H3，任务版 | 1 文件 × ≤12 行 |

合计 **≤ 9 个文件、≤ 80 行**。预留 3 个编号给后续 RFC。

三点说明：
- H1 分三处是因为"选渠道"、"存渠道"、"对客目录"是三个独立入口，少一处就有绕过。
- **H3 为什么是接管而不是"同事务"** [官方]：官方的预扣/结算没有数据库事务。`postConsumeQuotaWithResult` 直接调 `model.DecreaseUserQuota`，后者是一条独立 `UPDATE`；开启 `BATCH_UPDATE_ENABLED` 时甚至只写内存批量队列。没有事务可以"加入"，所以品牌客户的资金必须由 ext **完整接管**，在 ext 自己的事务里做；官方对该请求不再动 `users.quota`。非品牌客户走官方原路径，行为与官方完全一致。
- H3/H7 的 `handled` 判断只看 `TenantResolver` 写入 context 的租户标记，不读数据库，保证非品牌请求零额外开销。

### 2.3 自研渠道全部插件化

| 当前渠道 | 新形态 | 关键点 |
|---|---|---|
| 京东云灵境（type 59） | `plugins/tasks/cmh-lingjing/`，Task Plugin | 提交 `submitTask` + 轮询 `queryTasKResult` 完全在插件契约内；`usageSchema` 按千 Token 声明；宿主负责轮询、持久化、结算 |
| 昆仑万维 H3（type 58） | `plugins/tasks/cmh-skywork-h3/` | 同上，按秒计费 |
| Seedance 定制（草稿样片、真人素材库、回跳、幂等） | 协议部分复用官方 `doubao` 插件或写 `cmh-seedance/`；**状态部分放 `ext/brand`/`ext/seedance`** | 素材库、回跳验证、DNS TXT、幂等记录需要数据库，沙箱做不了，必须在 ext 用自己的路由实现 |
| 万相 WAN3 | 评估官方 `alibaba` 插件是否覆盖 `wan3.0-video-prime`，不覆盖则写 `cmh-wan3/` | 发布门禁放 `ext/publication` |
| NiceRouter/腾讯 ADP/乾坤等私有归一化 | `plugins/tasks/cmh-param-normalize/`，Request-processing | temperature 丢弃、developer→system、响应 model 归一 |
| 28 个 OpenAI 兼容文本渠道 | **不需要任何代码**，直接配置 | — |

插件自带 `golden.json` fixture，用官方 `new-api plugin test` 跑；不放 README（官方禁止）。

### 2.4 前端：独立应用，不注入官方前端 [官方前端结构已核对]

官方前端用 `@tanstack/router-plugin` 按 `web/src/routes/` 目录自动生成 `routeTree.gen.ts`，**没有外部路由目录的注入口**。往 `web/src/routes/` 加文件就是改官方区，违反宪法。所以：

- `web-cmh/`：独立的 React 应用，自己的 rsbuild 配置，构建产物由 `ext` 在 `/cmh/*` 路径下提供。承载：品牌管理 `/brand-console`、OEM 统计、推广、发票、财务报表、官方 Key 结算、企业空间后台。
- 与官方前端的衔接 [官方已核对]：官方 `HeaderNavModules` **只是对固定几个模块（home/console/pricing/rankings/docs/about）的开关**，前端 `nav-modules.ts` 不渲染未知键，**不能用来加自定义入口**。因此入口方案为：
  1. `web-cmh` 有自己的完整导航，是运营/品牌主/推广人日常工作的主入口，地址 `/cmh/`。
  2. 官方前端侧不加任何入口；运营人员通过 `/cmh/` 直达。若将来需要在官方界面放一个链接，走 RFC（预计是官方 `about` 页或系统公告的 HTML 配置项，不改组件）。
  3. 品牌客户、团队、代理商从来不进官方前端，他们的入口是品牌域名下的 `brandweb`。
- 登录态：复用官方 session cookie，`web-cmh` 直接调 `/api/user/self` 等官方接口；权限由 Casbin 策略判定（§2.5）。
- 组件：可以通过 workspace 依赖引用官方 `web/src/components/` 的导出组件，但不改它们。
- `brandweb/`：保持 Go 模板静态站，由 `ext/brand` 渲染，和主站前端无关。

### 2.5 授权与权限：用官方 Casbin，不自建 [官方]

OEM 的老板/主管/员工、推广人、运营、销售、业务运营等角色，全部定义为 Casbin 策略，放 `ext/brand/policy.csv` 之类由 `ext` 在启动时加载；品牌内的数据隔离仍靠 `TenantResolver` 写入 context 后在 ext 查询里过滤。

### 2.6 配置热更新与定时任务：用官方机制 [官方]

- 品牌配置、价格版本、白名单变更：`service.RegisterSyncTopic("cmh_brand", reload)`，多节点秒级同步。
- 域名验证/证书续期、统一钱包自动化、未知结果免单扫描、对账：注册为官方系统任务，自动获得 DB 租约去重与运行历史。

---

## 第三部分：实施分期

每期结束必须做一次"同步官方演练"（`merge upstream/main`，冲突只许在 `ext/docs/HOOKS.md` 登记文件），这是所有阶段共同的最后一条验收。

| 期 | 目标 | 主要产出 | 退出标准 |
|---|---|---|---|
| **P0 基座**（1–2 周） | 仓库立住、规则落地、两个关键假设验证 | fork + upstream remote；CI 越界检查；`web-cmh` 独立应用能用官方 session 登录并调 `/api/user/self`；`brandweb` 能按 Host 渲染 | 官方 CI 绿；越界 PR 被拒；`web-cmh` 登录与 Host 渲染两项原型通过 |
| **P1 挂载点**（3 周） | 7 个挂载点落地：观察型空实现，接管型先做"品牌租户接管、非品牌透传"的最小实现并过守恒测试 | `ext/core`、`ext/docs/HOOKS.md`、7 个 `hook(Hn)` commit、H3/H7 守恒测试 | 去掉 ext 后官方测试全绿；官方区 diff ≤ 80 行；H3/H7 在 `BATCH_UPDATE_ENABLED` 开/关两种模式下守恒测试全绿；同步演练零越界冲突 |
| **P2 白名单与文本渠道**（2 周） | 主站文本业务可跑 | `ext/domestic`、H1 生效；28 个文本渠道配置；`cmh-param-normalize` 插件 | kimi-k3 / glm / deepseek / qwen 验收矩阵通过；非白名单模型被三处入口拒绝 |
| **P3 定价与钱包**（4 周） | 资金链 | `ext/pricing`、`ext/wallet`、H3/H4/H7 生效；守恒测试 | 守恒测试全绿；三库验证；独立复核签字 |
| **P4 自研视频渠道**（3 周） | 视频业务可跑 | `cmh-lingjing`、`cmh-skywork-h3`、Seedance 协议+状态拆分、`ext/publication` | 每个渠道×型号连续 3 次真实任务账本闭环；H3/灵境不再占渠道类型编号 |
| **P5 OEM 与分销**（8–10 周） | 品牌站全功能 | `ext/brand` 全部、`brandweb`、团队/代理商/收款/券/收益/发票/报表/文档链 | 见 `OEM-MIGRATION-TASKS.md` T2–T4 全部验收项 |
| **P6 推广/发票/结算/企业**（4 周） | 其余业务 | `ext/promotion`、`ext/invoice`、`ext/settlement`、`ext/enterprise` | 各自资金守恒与权限测试 |
| **P7 上线**（1 周） | 新系统对外开放 | 生产环境、域名、构建发布链、监控告警；固定验收账号跑完全部验收矩阵后禁用 | 发布门禁全部通过；600 秒健康观察；第一个真实客户的首笔付费调用账本闭环 |

总计约 **25–29 周**（以 2–3 人全职估）。估算依据：旧系统 OEM 约 2.2 万行、56 张表，按"重建而非搬运"折算为 8–10 周；资金链 4 周含独立复核；其余按模块规模比例。P2 结束即可用固定验收账号开始真实调用；P3 结束后即可对外接文本业务客户，不必等全部阶段完成。

**最早可对外的时间点是 P3 结束（约第 10 周）**，此时主站文本业务可接真实客户；OEM 要到 P5 结束。

### 3.1 分期的含义变化

原方案的 P7"数据迁移与并行"、P8"切流与退役"已删除。新系统没有存量数据要搬，也不替换旧系统，所以：
- 没有"灰度"：新系统上线就是新系统自己的上线，不影响旧系统。
- 没有"回滚到旧系统"：新系统的回滚是回滚自己的上一个版本。
- 旧系统的功能移植是**按需、逐项、持续**的，不是一个阶段；由负责人下指令触发，每项登记到 `ext/docs/transfer-log.md`。P5、P6 列出的 OEM、推广、发票等，是预期会移植的功能，实际先后顺序以指令为准。

---

## 第四部分：验收体系

### 4.1 每期固定检查（G 项）

| 编号 | 检查 | 通过标准 |
|---|---|---|
| G1 | 官方区 diff | 命中文件 ⊆ `ext/docs/HOOKS.md`；总行数 ≤ 150（硬上限），设计目标 ≤ 80 |
| G2 | 依赖方向 | 官方区无 `import ".../ext/"` |
| G3 | 表名 | 扩展区新表全部 `cmh_` 前缀 |
| G4 | 同步演练 | `merge upstream/main` 冲突只在挂载点文件 |
| G5 | 品牌隔离 | 品牌页/文档/错误信息不含平台主体、平台域名、上游名称 |
| G6 | 资金守恒（P3 起） | 守恒测试全绿；对账零差异 |
| G7 | 文档同步 | 主站文档与 OEM 生成物一致 |
| G8 | 三库验证（涉及表时） | SQLite/MySQL/PostgreSQL 各跑一遍迁移与核心用例 |
| G9 | 官方计费规则（涉及计费时） | `.agents/rules/billing.md` 的不变量逐条对照：多重因子有上界、量化走 `quota_math`、无裸 `int()` |

### 4.2 资金守恒的定义（G6 的具体形式）

对任意请求序列（成功、失败、部分成功、未知结果、超时免单、退款）：

```
客户钱包 + 品牌可用额度 + 品牌在途预留 + 平台应收 + 平台已收 = 初始总额
```

且每笔请求：客户只扣一次、平台只结一次、退款只退一次；重放同一事件不改变任何余额。

### 4.3 验收账号与证据

- 固定验收账号 `cmh_acceptance_test`（沿用现规则）：验收期启用，验收后禁用，发布门禁检查其状态。
- 每期证据落盘到 `ext/docs/acceptance/<期>/`，含命令、输出、数据库版本、对账结果。不落盘不算完成。

---

## 第五部分：风险与未决项

### 5.1 已识别风险

| 风险 | 影响 | 应对 |
|---|---|---|
| 官方预扣/结算没有事务（已核实），"同事务"不可行；接管型 H3/H7 若漏掉某条资金路径，会出现官方扣一次、ext 又扣一次 | 资金不守恒 | H3/H7 落在 `BillingSession` 三个方法与 `taskAdjustFunding` 这四个唯一出口；P1 用 `grep DecreaseUserQuota\|IncreaseUserQuota` 全仓核对没有其他出口；守恒测试覆盖 `BATCH_UPDATE_ENABLED` 开/关两种模式 |
| 接管型挂载点改变官方控制流，是最容易在同步时冲突的地方 | 同步成本上升 | 两个接管点都只在函数入口加 `if handled { return }` 一段，不改函数体；同步时若官方重构 `BillingSession`，按 RFC 重新落点 |
| 官方前端没有自定义导航入口（已核实） | 运营需记住 `/cmh/` 地址 | `web-cmh` 自带完整导航；需要时走 RFC 在官方 about 页或公告放链接 |
| 两套系统各自演进，同一业务规则（如定价优先级、OEM 资金规则）两边逐渐不一致 | 运营口径混乱 | 移植时以旧系统当时的规则文档为依据并记录版本；之后新系统独立迭代，不强求两边一致；`ext/docs/transfer-log.md` 记录每项移植的依据版本 |
| 新系统初期无真实流量，验收只能靠合成数据 | 真实场景的问题发现晚 | 固定验收账号跑完整矩阵；P3 后尽早接第一个真实客户的文本业务，用真实流量验证资金链 |
| 官方插件沙箱无法承载 Seedance 素材库/回跳 | 功能缺失 | 已明确拆分：插件管协议，`ext` 管状态；P4 用灵境先验证拆分可行 |
| 官方 `billing.md` 规则严格，`ext/wallet` 重写时容易违反 | 被官方规则判为不合格 | 把 billing.md 不变量做成 G9 检查清单，代码评审逐条过 |
| 官方 release 节奏快（约 1–2 周一次），同步跟不上 | 冲突复合 | 宪法 §7.2 强制每 release 跟一次；P1 起就开始跟，不等业务做完 |
| 官方未来新增渠道类型或改插件契约 v2 | 插件失效 | 插件 fixture 纳入同步演练；契约变更时按官方迁移指南升级 |
| 团队同时维护两套系统 | 人力分散 | 新仓库为主迭代；旧系统按需维护；由负责人决定旧系统的维护强度 |

### 5.2 已拍板的决定（2026-11-13，负责人确认）

| # | 事项 | 决定 | 对方案的影响 |
|---|---|---|---|
| 1 | H3/H7 资金一致性 | 负责人原意是**同事务**（品牌扣款与官方扣款不可分离）。核对官方源码后发现官方预扣/结算没有事务可加入，因此实现形式改为**接管**：品牌客户的资金操作由 ext 在自己的事务内完整完成，官方对该请求跳过 `users.quota` 读写。守恒目标不变 | H3/H7 为接管型接口（返回 `handled`）；T1.6c 改为验证四个资金出口全部被接管且两种批量模式下守恒；恢复器只处理上游未知结果 |
| 2 | `web-cmh` 技术栈 | **与官方同栈**：React 19 + TanStack Router/Query + Rsbuild + Tailwind + Base UI | 可直接复用官方 `web/src/components/` 导出组件；构建配置以官方 `web/rsbuild.config.ts` 为模板 |
| 3 | 新旧系统关系 | **新系统是全新系统，不替换旧系统**。旧系统独立运行，不迁移数据；旧系统的有用功能由负责人逐项下指令移植；后续以新仓库为主迭代 | 删除数据迁移、并行对账、灰度、切流、回滚到旧系统、旧仓库退役等全部内容；移植指令须指明功能范围与依据的旧系统版本 |
| 4 | 灰度 | **不做** | 已从方案中删除 |
| 5 | 人力与周期 | 接受。v0.5 按旧系统 OEM 实际规模重估为 25–29 周（2–3 人全职） | 分期见第三部分；若超出接受范围需负责人再确认 |

### 5.3 待确定项

- 功能移植的先后顺序与每项依据的旧系统版本（随负责人指令逐项确定）。
- 新系统的域名、生产环境与数据库（P7 前确定）。

### 5.4 本方案不做的事

- 不在现有仓库上做任何"渐进式对齐官方"的尝试。
- 不改渠道类型编号来让出 58/59（插件化后不再需要）。
- 不把 `ext/` 发布为独立 Go module（保持单仓库，避免跨模块版本管理）。
- **不迁移旧系统任何数据**：不迁用户、Key、余额、渠道、价格、品牌、日志、任务。新系统的渠道和价格在新系统后台重新配置。
- 不做新旧系统之间的对账、同步或互通。

---

## 第六部分：立即可做的事

1. 按 `AGENTS.md`（当前版本）建新仓库，接 upstream，上 CI 越界检查（P0.1、P0.4）。
2. 用两天验证两个关键假设：`web-cmh` 能否复用官方 session 登录；`brandweb` 按 Host 渲染是否只靠 H2+H5 两个挂载点就够。
3. 写 H3/H7 接管型原型（≤200 行）：在 `BillingSession.preConsume/Settle/Refund` 与 `taskAdjustFunding` 四处加 `handled` 判断，ext 侧用一张最小的 `cmh_test_wallet` 表接管，跑守恒测试（含 `BATCH_UPDATE_ENABLED` 开/关）。

这三件事做完，方案里所有"待验证"的部分都变成事实，即可全面投入。

## 修订记录

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-11-13 | v0.2 | 基于官方 main 一手文件重写：挂载点精确落点、前端改独立应用、docs 位置改 ext/docs、插件沙箱约束、Casbin/同步主题/系统任务复用、G8/G9 检查、分期与周期 |
| 2026-11-13 | v0.3 | 负责人拍板：H3/H7 同事务；web-cmh 与官方同栈；旧仓库不冻结、功能逐项按指令转移；灰度品牌 P7 前定；周期接受。风险表与第六部分相应更新 |
| 2026-11-13 | v0.4 | 负责人明确定位：新仓库是全新系统，不替换旧系统、不迁移数据、不灰度、不切流。删除 P7/P8 迁移与切流，改为 P7 上线；周期 22–26 周；风险与决定表相应重写 |
| 2026-11-13 | v0.5 | 审查修正：(1) 官方预扣/结算无事务，H3/H7 由"同事务"改为"接管型"并落到 `BillingSession` 与 `taskAdjustFunding`；(2) `HeaderNavModules` 不能加自定义入口，前端入口改为 `web-cmh` 自带导航；(3) docs 目录限制改述为我们的选择；(4) diff 硬上限 150 行；(5) P5 重估为 8–10 周，总计 25–29 周 |
