# RFC：旧任务独立轮询授权最小合同（2026-11-14）

状态：**负责人于2026-11-14明确选择“批准有限首批合同（推荐）”；允许按本文实现和验收，不是实际task许可数据或全局启用授权**。批准前最后基线03bc32218，全局Gate/Store仍未启用。

## 1. 为什么现在要拍板

已有档案证明原source/runtime字节完整性，ref绑定task/user/channel，但**数据完整性不等于执行授权**。Save发生在buildSubmit之前，未来query/content的真实签名URL尚不可知；Load接受poll/content/report枚举，也不是operator为这些用途授予权限。若在global source撤销后仅凭Restore/Bind/ref放行，等于把非secret审计引用变成bearer绕过。

源策略拒绝现已能让冻结poll跳本次、不增加普通失败计数；这保护账务流程，却没有让合法旧任务继续完成。需要明确：谁为哪一个旧任务的哪一段旧代码继续执行负责、何时到期、怎么撤销。不能用泛化“白名单已批准”代替这组新权限。

## 2. 推荐首批权限（不是永久产品缩范围）

仅实现**显式operator逐任务、per_task、静态Key轮询**，在已有精确global目的地批准下完成真实query→parse。

批准键必须同时包含：
- authoritative TaskID/UserID/ChannelID、Archive Ref.ID/Ref.Digest；
- Purpose固定poll，精确PluginKey/SourceSHA256/RuntimeSHA256；
- 有限Hook集合，仅buildQueryRequest、parseTaskResult、可选signRequest/extractUsageOnComplete，无Members路径通配；
- operator显式ExpiresAt（UTC Unix秒，必须未来时间，无默认永久、无续期）；构造时拒重复/未知/畸形/过大清单；
- 清单通过既有批准制品的明确digest锚定，不从registry发现或ref存在自动生成许可，不把source某一config批准扩大成全export许可。

## 3. 期限、撤销与控制

- 每次被授权hook真正进入JS前（包括slot/runtime排队之后）检查用途/源/精确export和期限，不只resolver创建或Engine入口时检查；附带scope失效/过期必须拒绝，不回退currentGate以绕到期，既有adaptor也必须拒绝。已经开始的hook/网络操作可按原timeout完成，不承诺到期瞬间中断或回滚；过期返回ContinuationUnavailable，不能newsubmit。
- scope期限还要在descriptor真正派发及每次redirect前核对（now Unix秒>=ExpiresAt即拒）；到期后不启动下一hook/新网络，即使上一hook到期前已开始。已经启动操作可完成且不能回滚，不提供到期秒强杀保证；系统时钟属于TCB，不声称解决恶意时钟回拨。
- 第一批**启动时不可变批准快照**；撤销通过部署新的不含该grant清单并重启所有承载节点生效，不承诺热撤销或全节点瞬时原子撤销。需要强制即时撤销则另立版本化同步/存储合同，不能假装已有RegisterSyncTopic接好。
- 原task timeout sweep/CAS/refund规则保持，授权不自动延长资金deadline、不承诺永不退款。Grant期限与任务deadline必须由operator配合配置，运行时原timeout仍生效。
- 当前源策略撤销只阻止新调用；精确旧task grant可独立允许上述有限poll hooks。其它task/ref/purpose/source/runtime和未列hook仍按当前Gate拒绝。没有operator grant时维持当前保守拒绝/跳本次。
- 本首批不提供新的客户/API激活入口，不绕官方身份、ownership、持久task绑定或原credential-account守卫。

## 4. 权限传播与网络硬限制

- 在GetTaskAdaptorForTask对应的**可信ContinuationStore.Load完成认证后**，由独立authorizer对精确binding/ref/purpose/artifact核验，core签发private-field scope。调用方不能通过零值scope或公开Restore/Bind自行mint。启动注册的Store/authorizer与Go宿主代码属于TCB；私有字段防客户/JS/普通ref复用，不声称防恶意替换可信Go实现或进程攻陷。
- scope只绑定认证过的task及purpose；TaskAdaptor每次先做现有checkContinuationTask，再给相应单次hook挂scope；Engine仍检查当前sourceGate，只有精确scope可作为这一source-only边界的受限替代证明。
- 携带scope的hook始终**不提供utils.fetch**，即使currentglobalGate也批准也不恢复fetcher；不能通过buildQuery内部读请求获得未声明网络；刷新/账号交换也不授予。只接受静态Key且meta.auth.type仅空/none/api_key，遇pluginCredential、oauth2_jwt或其它认证模式在queryContext/resolveAuth和任何token网络前Unavailable；不能把服务账号JSON当安全staticKey。
- scope启用必须同时安装权威global DestinationPolicy（DestinationAdmissionEnabled=true），noop时拒scope，不将原nopolicy快速允许当exact批准。实际query descriptor/sign后的物理请求仍须该policy精确(method/完整URLhash/Hosthash/source-capability/hook)批准，所有redirect每跳仍检查；绝不弱化动态query/签名URL匹配为通配。
- scope不免除SSRF、TLS、proxy/host边界、body大小、执行timeout、archivedbinding和禁止newsubmit等现有检查。源编译/顶层metadata执行当前合同不因本批改变，不宣称编译可被hookscope中断。

## 5. 明确不在首批

batch/content/report、pluginCredential refresh、utils.fetch、task-scoped动态URL/完整target授权、凭据/档案key管理/备份GCM寿命、全局Gate/Store启用。它们仍是后续目标，不能因首批只支持static per_task就宣称原全协议产品验收完成。

## 6. 文件预算与验收

预计只复用已登记pkg/jsplugin/engine.go、relay/relay_adaptor.go、relay/channel/task/jsplugin/adaptor.go约150–250官方增删行，core/policy在ext，测试继续既有host_admission_test.go。当前49/4240、余1760行，无第50file授权，预算非完成保证；实际超界先停该部分。

验收必须真实三库：global sourceGate拒A的新提交，精确grant只让旧taskA经真实resolver/DBpoll/网络stub/parse成功；错task/user/channel/ref/purpose/source/runtime、过期与未列hook拒；零值scope/RestoreBindalone不可绕；已创建adaptor过期拒；utils.fetch尝试0对应网络；query目的是未批时0对应网络，staticKeyonly；JS同文案普通失败分类保持；完整私有funding状态不受拒绝改变。独立未参与实现复核、相关tests/vet/build、差异CI、main/develop推送。不得把纯policy parser/CLI fixture当host执行通过。

## 7. 独立设计复核

只读093219da（未参与实现，未运行测试）核对现有resolver/Engine/TaskAdaptor/auth路径，提出必须enabled目的地policy、排队后/派发与redirect前过期检查、scope限制不因global重批而放松、拒隐式oauth2_jwt token网络、TCB与全进程/节点重启撤销六项；均已写入本合同。复核认为在接受重启撤销前提下无需为了首批引入更大持久化架构，不代表实现已通过或完整ASVS认证。

## 8. 请求决定

负责人已批准上述**显式逐task、有限hook、显式到期、重启撤销、static per_task首批**合同；接续直接实施并独立验收，不再重复询问同一许可。批准不含任何真实grant数据/生产凭据/收费调用、不含资金重构或全局启用。未来若要求立即热撤销或动态target授权，仍须先选更大的持久化/多节点控制架构，不暗中更改本合同。
