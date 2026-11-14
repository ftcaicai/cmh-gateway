# 范围决策请求：共享端点提前预选的精确generation（2026-11-14）

状态：**负责人于2026-11-14明确选择“批准单文件例外（推荐）”；当前只落实独立rules PR，生产实现未改、Gate未启用**。

## 当前硬边界

真实门禁为累计48/48官方文件、4063/6000增删行，最后已推a758574ae；不是额外6000行。新的官方文件即使只改几行，也会成为49，当前授权不允许。不能删测试/恢复有意义挂载点来腾名额。

## 已直接核实的剩余位置

middleware/task_plugin.go RouteTaskPluginEndpoint在523取generation、529用该generation选协议候选，536–544构造ChannelFilter但没有PluginView，546先调用SelectChannelForRequest；这个调用发生在Distribute之前。现有distributor.bindPluginCandidateView于Distribute/Setup才执行，不能逆时修正已经做出的预选。

同文件PinTaskPluginEndpoint在598默认取当前generation，599–602已能沿预选结果复用原generation；因此不需要复制decoder/route逻辑到ext，不需要改plugin编号或策略默认放行。初步最小补丁是在已有generation边界，将实际generation视图带入预选filter及context，确保后续filter改kind及fallback仍同源；具体以实现审查后可读diff为准。

## 为什么不是再加一个ext parser

JS插件无权改变host候选选择；ext.RouteRegistrar不能插进既有函数内部；外层middleware在入口取currentgeneration与函数523之间仍有替换窗口，若仅从名称/编号重建就不是同一artifact证明。新typed PluginCandidateView已经给出正确host接口，但必须在拥有该generation局部变量的权威点接它。不会为了凑48预算复制整条official route或使用any/反射/未知executor放行。

## 请求的最窄授权

**只给已有middleware/task_plugin.go一个额外官方文件例外**，允许累计49文件，**6000行累计硬限不变**；仅RouteTaskPluginEndpoint的generation视图传递，以及必要时PinTaskPluginEndpoint对已捕获context generation的复用（已有selected.Generation保持最高优先级，不改decoder/候选排序）。预估生产增删不超过40行，测试继续复用ext/domestic/host_admission_test.go而非新增official测试文件；超过此函数范围或预估须再说明，不据此任意改整个middleware。

保持：core无业务边界、逐文件HOOKS登记、80%复核、独立只读审查、真实SQLite/MySQL/PostgreSQL、原候选优先级/权重/鉴权/计费不变、失败0对应网络/预扣断言。授权不含资金H3/H7、生产/收费调用、密钥、全局Gate激活，也不保证全部H1能在49文件内完成。

若不批准：保留已验代码和全局未启用状态；不能以关闭整个原协议或长期堆纯parser伪称目标完成。另有task-scoped旧grant、credential refresh、affinity/type60/legacy、启动key管理/全验收仍需收口，金融同事务RFC仍独立。
