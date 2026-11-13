# RFC（已批准、实施待验）：H1 完整准入边界与登记预算

状态：**负责人于2026-11-13先批准32文件/2500增删行，达到文件硬限后又明确选择“批准累计48文件/6000行收口预算（推荐）”**。当前累计官方区硬上限48文件/6000增删行；仅限本RFC的H1准入型/多调用点、无业务ext/core接口导入，以及剩余执行链、最终目的地、后台延续和已发现兼容缺口。不授权资金H3/H7重构、费用或生产操作。A/B/C部分代码和测试已分批提交，当前仍未安装全局策略；预算批准不代表完整安全覆盖验收通过。
依据：本地官方 `v1.0.0-rc.50`，`acceptance/p2/admission-surface-audit.md` 与独立只读设计复核。原“三处门禁、十个文件/150行即可”已被实际路径审查证伪，不再以此承诺工期或安全。

## 1. 不变的目标

- 新系统独立，不迁旧客户/Key/余额/配置，不访问旧生产。
- 官方基座优先复用，业务在扩展区；不增自有渠道类型编号，不复制官方协议转换/映射逻辑。
- 管理员预览上游目录不等于公开发布；拒绝调用必须在本地预扣与对应上游派发前。后台既有任务的结算/退款不得因新调用门禁自动放弃。
- 不用降低测试、删除断言、压缩代码、放行未知插件或伪造缓存满足预算。

## 2. 为什么现规则下不可直接实施

已登记官方区9文件/110行（包括README、workflow、2后端接线、4前端桥接与1工厂清单测试），剩1文件/40行。

H1最小的已确认分散边界包括渠道保存/部分补丁/批量、自动模型同步、凭据恢复启用、候选过滤、多套客户目录、价格投影、最终映射、预扣前校验与WS复用；这些并不位于同一个函数或文件。只补Distribute会漏掉Responses WS和重试，只补Controller会漏掉copy/tag/auto-sync/status，只过滤pricing会漏掉用户模型列表与单模型retrieve。

另有规则表述冲突需显式修正，而非暗中按宽解释执行：
- §2.2说官方禁止import扩展区，而现有H5已import `ext/core`，CI也明确允许。应将例外限定为**只允许已登记挂载点导入无业务依赖的ext/core，以及main唯一空白导入ext**；不允许官方业务包import ext/brand/wallet等。
- H1是拒绝/过滤门禁，当然会改变控制流，不能再称“观察型不改变控制流”。应增加**准入型**合同，error必须沿官方错误格式传播，不能被调用方忽略。
- 安全边界可能有多个调用位置，不能硬性“一接口只有一行调用”。用编号登记用途与函数/阶段并逐项回归；挂载点数量不因增加调用位置偷算为新增业务逻辑。
- “删除ext目录还能编译”与已有静态import不相容；正确基线验收为**保留core接口及空实现、禁用业务注册后官方行为一致**。

上述H1专项形态/依赖/预算调整已由负责人明确批准并写入宪法v0.8；所有实现仍须登记并逐批复核，不扩大为资金重构授权。

## 3. 推荐接线合同（候选）

`ext/core`只定义值DTO和接口，不import model/service/relay，防止循环依赖。公开名Public、插件声明Declared、最终上游Upstream、实际执行PluginKey不可混用。

```go
// 草案，实施前根据已核实官方类型调整；不是现有API。
type ModelIdentity struct { Public, Declared, Upstream, PluginKey string }
type ChannelIdentity struct { Known bool; ID, Type int; ConfigFingerprint string }
type ExecutorIdentity struct { Kind, Key, SourceSHA256 string; Generation int64 }
// Gate.Check(ctx, phase, modelIdentity, candidate, executor) error
// Gate.ValidateChannels(ctx, completeEffectiveSnapshots) error
// Gate.Visible(ctx, modelAndCandidateProjection) bool
```

- nil/未知渠道或尚未解析上游不等于授权。候选阶段与最终执行阶段有不同必填字段，不能用type0/空串自动通过。
- 沿用官方公开名规范化语义，之后对**规范化的完整标识**精确匹配；不声称这等同原始字节精确匹配。原始大小写严格拒绝是另一政策，未经批准不暗中改变。
- 不透明部署ID必须有operator审核的渠道/模型/上游目标关系，不凭model包含某词推断国产。允许的公开别名不能洗白未批准上游，反之也不能自动放行未批准公开名。
- 插件按实际执行器、源哈希、配置指纹审核，不把同名meta owner当作执行者。纯逻辑Gate只能证明逻辑身份，不能识破任意JS在内部网络调用里偷偷变模型；因此需要已审核、固定版本的operator可信制品。缺该证明的候选拒绝新调用，而不是假称任意插件都已隔离。

## 4. 分批实施与冻结检查点

### A. 基础准入与客户目录

在下述官方既有位置增加小型DTO桥接与veto/filter：

| 文件 | 已观察到的职责 | 落点要求 |
|---|---|---|
| controller/channel.go | Add/Update/Copy/batch status | 完整有效快照；PATCH第一次Key写入前预验；整批错误不能部分成功 |
| controller/channel_upstream_update.go | 自动/单项/全部模型同步 | 共享更新函数写Models之前 |
| model/channel.go | Insert/Batch/Update/tag/status | 最终持久化和重新启用前复核；关闭操作始终允许；并发/事务语义三库验 |
| model/channel_plugin_credential.go | 登录/定时刷新三种locked更新恢复启用 | callback后、保存前验最终启用快照 |
| model/channel_constraint.go | pins/affinity/cache候选 | 过滤不合格候选再排序/加权，不选一个后错误封死全模型 |
| model/ability.go | enabled model投影与DB候选 | 失败关闭，不将DB错误回退为未过滤集合 |
| controller/model.go | ListModels/Dashboard/Retrieve | 静态表与插件模型也过滤；拒绝单模型保持notfound错误契约 |
| model/pricing.go | 模型/厂商/分组/端点索引 | 投影前过滤，不能只末尾删模型留下供应商关系 |

### B. 派发前、追加预留前与长连接

| 文件 | 已观察到的职责 | 落点要求 |
|---|---|---|
| middleware/distributor.go | 实际plugin reconciliation、key选择 | public gate覆盖pins；最终执行者明确后、取Key/网络前再验；不能忽略SetupContext错误 |
| relay/helper/model_mapped.go | 多跳映射/base fallback/self-map | 复用或导出官方解析，不复制进ext；自映射也不能提前跳过 |
| relay/request_billing.go | handler映射前就预扣 | 准入必须在估价/免费模型跳过/预扣前完成 |
| controller/relay.go | 重试换组与额外预留 | 实际重试身份复核在PrepareTieredBilling之前 |
| relay/responses_websocket.go | 多次response.create复用连接 | 每个事件/恢复渠道/当前政策均复核；首次候选和复用都验 |
| relay/relay_task.go | task attempt/validation/price | 插件可网络动作前验；解析后再次验身份 |
| relay/channel/openai/relay_realtime.go | locked model事件直接转发 | 握手和每个发送事件复核，不能只验初始HTTP |

### C. 插件执行、请求处理与最终目的地

| 文件 | 候选职责 | 验证状态 |
|---|---|---|
| relay/channel/task/jsplugin/adaptor.go | 凭据刷新/driver hook前，descriptor改model后 | 次序已读；最终运行钩子可先联网，需审核制品身份而不是只后置检查 |
| relay/channel/providerplugin/adaptor.go | refresh/exchange/convert前后 | 需在已审插件配置关系下复核实际目的地，不承诺任意opaque正文可通用识别 |
| relay/channel/api_request.go | 最终物理destination | 这是host transport防线，不是utils.fetch全部网络的唯一出口 |
| pkg/jsplugin/registry.go | 发布允许的固定源版本 | **保守占位，精确generation commit路径待实施前完整核实**，不能称已验证具体插入点 |
| relay/requestplugin/plan.go | 活跃实例源版本/配置指纹 | ingress可联网前有可信执行计划；只允许operator授权，不新开客户激活权限 |

此候选20个生产文件不是数学最小值，也不是直接批准这20个文件任意改写。估计800–1500新增生产差异行（含import/格式余量），加现有110约910–1610；实际以可读补丁numstat为准，估计不能代替验收。

## 5. 已批准专项预算与形态

负责人先批准32文件/2500增删行，达到32文件硬限时再次明确批准**累计官方区48文件/6000增删行收口预算**（不是额外再加48/6000）。包括此前R1–R4、H2/H5、所有H1生产与既有测试文件。新增范围仅H1剩余执行链、最终目的地、后台延续和已发现兼容缺口；不含费用、生产操作或资金H3/H7重构。每个文件仍需在HOOKS/RFC列出具体函数和职责，不能给整个目录通行证；80%复核、真实本地/三库行为测试、独立审查仍保留。额度不等于完成保证。

这是硬预算候选，不是越多越好；实现分A/B/C独立提交、逐批实测。到预算80%先复核，达到上限停止该部分改动并提差异说明，不自动续增。不同时授权资金H3/H7重构；金融桥仍单独设计复核。

若不接受预算，应选择另一架构（例如独立外层网关或明确缩小产品协议范围），不能在原预算内宣称全协议防绕过已完成。方向改变需用户明确，不由实现者静默禁掉整个既有协议。

## 6. 必需验收（不是当前已完成）

- Admin Add/批量/PATCH nil与空/copy/tag/auto-sync/apply_all/credential-enable；拒绝时无部分写入，三库运行证明。
- 全部客户models/list/retrieve/dashboard/pricing/self；vendor/group/endpoint投影无残留，缓存切换不泄漏。
- pins、正常候选、affinity、DB/cache、重试；被拒候选零预扣零对应上游网络，合格备用候选仍可选。
- Responses WS第二次create、Realtime每事件、插件native/shared、映射变更与自映射/循环。
- plugin源/配置变更使新调用拒绝；后台已有任务的query/结算/退款按冻结能力完成，不因发布禁用偷退或漏结。
- 确定性本地stub计数，`network=0,reserve=0`对拒绝场景必须直接观察；纯Policy单测和插件CLI通过不够。
- 独立复核、真实SQLite/MySQL/PostgreSQL验证、现有官方测试与新增扩展回归；上游新release出现才做真实同步演练。
