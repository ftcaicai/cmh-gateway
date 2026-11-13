# H1-C 80%预算范围复核（2026-11-13）

## 触发与当前结论

已批准累计上限32官方文件/2500增删行。本轮从25/1946进入27文件约1970行，**文件数超过80%**，所以在继续实际运行时放行前做本次复核。不自动扩大预算，不压缩代码，不将剩余未知执行者全部拒绝称作完整支持。

当前可安全提交的实际子集：插件原始源摘要、显式operator审批记录/有效配置字节指纹、Engine.call共用SOURCE-ONLY执行前veto。全部默认noop/不自动安装，无客户/生产激活。

## 独立只读范围复核

复核者 `7317a93a-ac0b-4064-843a-746242899cbc` 读取完整Plugin API与实际执行链，未写代码/未测试/未做收费或生产请求。结论：剩余约530行/5文件不能被诚实证明足以完整H1-C，需按权威边界拆开做真实可读补丁后测量；当前不承诺全部可容纳。

### 已核实必须留住的事实

- 全部Call/CallInto/CallJSONInto/CallMember/CallPath*最终走 `Engine.call`；只放在CallPath会漏平面driver/provider调用。Options callback仅某loader传入也不完整，factory/override/temp编译会遗漏。
- effective config必须是**真正交给该hook的冻结值**，不能拿DB/cache当前值代替；task applyChannelContext读cache，而provider从RelayInfo冻结值读取，语义不同。
- requestplugin是有序pipeline，不能伪装成单个Executor；同源插件不同instance配置/role/path需分别绑定。keyword hookConfig是有损summary，不能拿它当原配置指纹。
- candidate只能根据本渠道绑定/映射和实际固定generation投影；GetByModel是元数据owner，不是执行者。cached candidate持channelSyncLock时不能调用可能rebuild aggregate alias cache的ResolveTaskPluginModel（锁/DB死锁风险）。
- hook source veto被optional实例吞掉错误时，仍能阻止该hook执行，但不保证整个request拒绝；plan级host veto必须不可选。
- ingress/login/refresh/query/report/poll可能无model/channel；需要独立执行器合同，不能编造public/upstream/channelID让ModelGate通过。
- 旧任务query/退款/结算须冻结能力延续，而不是发布撤销就放弃已有钱账。
- artifact批准不证明wire model或URL目标，transport共享DoRequest不是utils.fetch/凭据交换的全部网络出口。

## 后续具体步骤（尚未实现、非验收）

1. 在已登记model/channel_constraint.go实现本渠道type61精确插件源/角色/有效配置投影；type60歧义和requestplugin chain仍保守unknown，不能因此全局激活。
2. middleware实际pin/provider执行者、CheckRequestAdmission与task/provider hook前使用同一冻结LoadedPlugin/Engine身份、role/path+有效config envelope；禁止用新registry候选授权旧pinned源。
3. core值DTO增加有序executor chain或每model完整plan，再在requestplugin实际plan/reply/report/task report路径证明每个实例；无证据不认证。
4. task pre-validation、descriptor.Model/RewriteModel变更后、provider credential refresh前及最终网络发送前分别观察；本地stub对拒绝source/config/target的network/reserve计数必须0。
5. 最后增加operator配置加载（严格显式manifest，空默认拒绝），完成三库/独立复核/paid-free protocol矩阵后才考虑安装Gate。当前manifest parser不读环境/磁盘、不注册任何实例。

如真实实现触及硬上限，停止该部分并给实际diff与剩余调用位置请求一次性决策；**本记录不是新预算授权**。资金H3/H7同事务仍独立RFC，不借H1专项变更。
