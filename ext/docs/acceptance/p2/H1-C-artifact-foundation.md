# H1-C 源制品与审批基础（2026-11-13）

**本批是SOURCE-ONLY运行时拒绝点与审批基础，不是完整插件准入；未安装core.Models或core.PluginHooks。**

## 已交付

- LoadedPlugin私有编译源摘要与SourceSHA256 getter：精确原始源字节，包括注释/空白，不信任Meta.Version或Options.Key标签。factory、override均由同一编译构造；旧generation保留旧对象摘要。
- Engine单独保存它实际编译代码的摘要，所有Call/CallInto/CallJSONInto/CallMember/CallPath*收敛Engine.call，在slot/runtime/JS参数转换及hook执行之前检查core.PluginHooks；取消先于策略，策略错误固定为plugin execution rejected。
- core值DTO只含SourceSHA256/Export/Members，不含Args/请求正文/配置/Key/凭据；Members复制，策略不能改调用者路径。默认noop，启动期才可替换，不自动批准已安装插件。
- ext.Gate同时实现source-only CheckHook：只有显式审批集合出现的源摘要允许hook源边界；它不代替完整Check对config/key/model/channel tuple校验，当前也未注册进core.PluginHooks。
- ext.ParseExecutorApprovals显式operator记录：每个非空行精确key、source SHA256、config SHA256三列TAB；64KiB/256记录上界，错误不返回半成品，空文档批准零插件。
- EffectiveConfigFingerprint对**host schema/default处理后真正交给hook的冻结JSON对象字节**求摘要；不重复默认展开、不自动排序或删字段。空白/键序变化有意改变摘要；这是fail-closed exact-byte约定，不是canonical JSON或schema/唯一键验证。

## 实际行为测试

集中扩展既有policy_test.go与plugin_test.go，没有新增多层测试：
- 同key/version但代码、注释、空白不同，源摘要均不同且等于独立sha256计算；factory/override/旧generation对象身份保持。
- 九种真实JS公共调用包装都被共享入口拒绝；先拒绝再允许时module计数仍1，证明被拒hook没有执行；不是仅日志断言。
- 策略改Members不能改变实际调用路径；取消context先于策略；JS容量已占用时veto在容量timeout前返回（同步channel控制，无sleep）。
- 元数据Export与编译仍可执行（有意不属于运行时hook授权）。默认策略恢复后原调用可用。
- 审批格式错误、重复tuple、wildcard、大小写摘要、额外字段、无效UTF8、超大小/记录上界失败且无部分结果；显式zero/false/null保留；config变化撤销匹配；不注册任何全局策略。
- host JSON decoder接受重复键，fingerprint保留并审批原字节；不声称schema/唯一键已检查，host producer必须先验证其有效配置。

## 独立审查与限制

复核者 `db40743d-22e0-4278-bf4b-e5782804ce16` 只读审查，未写代码/未重复测试：在文档exact-byte合同下未发现解析或SOURCE-only scaffold阻断性fail-open。

明确保留风险：LoadedPlugin.Engine是既有公开可替换字段，因此LoadedPlugin getter只证明该对象编译时源，不证明可信Go代码未替换Engine；运行时veto使用**Engine自己的摘要**，不被这类标签替换绕过。未来实际executor/config投影必须绑定实际Engine，而非单靠LoadedPlugin getter。

source批准不证明有效config、model、channel或wire目标；top-level模块求值、Export/HasCallable检查未被hook gate拦截。optional插件吞掉错误时仍阻止hook执行，但不保证整request拒绝，未来plan级host veto还需不可选。既有任务后台延续能力未接线，不以当前撤销方式偷放弃结算/退款。

## 80%预算复核

已触发文件数80%警告，独立只读复核 `7317a93a-ac0b-4064-843a-746242899cbc`已检查剩余权威链，见 `H1-C-budget-review.md`。当前仅提交可证明子集，不承诺剩余5文件/529行能完成全部pipeline/config/destination/poll；不扩预算、不压缩漏路径。

## 真正执行的验证

```powershell
$env:GOSUMDB='sum.golang.google.cn'
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOMAXPROCS='2'
go test -p 2 ./pkg/jsplugin ./plugins ./ext/... -count=1
go vet ./pkg/jsplugin ./plugins ./ext/...
go build -p 2 ./...
```

最终全部exit0：pkg/jsplugin2.532s、plugins0.927s、brand0.101s、console0.158s、domestic7.684s。纯基础不新增数据库行为；既有host SQLite回归随ext执行，MySQL/PG未为本批纯函数专门启动，不声称又做三库。此前数据库接线矩阵证据保留。

门禁27/32官方文件、1971/2500增删行，语法/空白检查通过，并按规则输出80%警告。编译/测试工具环境仅进程内，未改全局Go或源码依赖。无生产/旧系统/付费上游/凭据读取，无新后台服务。
