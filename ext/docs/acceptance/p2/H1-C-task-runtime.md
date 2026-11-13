# H1-C Task提交前后运行检查（2026-11-13）

本批限定type61新提交，未安装全局ModelGate/PluginHooks；不是完整H1、后台任务延续或金融守恒验收。

## 实现与顺序

- RelayTaskSubmit在late TaskAttempt之前、validation前及validation后价格/预扣前调用原共享检查，拒绝固定403/local/NoRetry。
- TaskAdaptor实际检查绑定a.info、context ID/type/public/group、exact pinned plugin/generation、actual Engine源、有效配置与当前DB投影。发生credential刷新时保守要求current源仍为exact pinned，避免service内部替换源。
- 检查在最终native/protocol decoder前、buildSubmit的credential/auth准备前、submitContext可能触发auth网络前、buildSubmitRequest前、descriptor.Model/RewriteModel变化后和cached复用时执行。实际提交DoRequest在签名/发送前再验。
- 当前type61提交hook使用已检查的RelayInfo有效配置，不再让CacheGetChannel的另一份配置替换它。signTaskRequest独立通道也改用同一已准提交配置。
- descriptor缓存保存构建时的Executor/Model；即使DB和actual info一起改成另一个已批准配置，也不能复用旧descriptor继承新权限。
- 默认noop立即透传；新建/Init后的poll/content适配器仍走既有配置路径，不把新调用门禁套到已付费任务退款/结算。

## 独立复核

复核者 `05b72387-fd5b-40e3-bbbe-2c46aa147d6a` 未参与开发、只读，未重复测试/DB/生产/收费调用，指出并提交前修复：

1. DoRequest直接发送缓存descriptor、不经过buildSubmit再验；已增加发送前检查与DB撤销测试。
2. credential.go signTaskRequest仍独立读cache配置；已冻结提交签名配置，真实upstream验证X-Tier。
3. 最终decoder在build检查之前执行；已在Validate入口加实际运行检查，保留后续build/descriptor复核。

复核最终重读确认上述落点存在，未发现限定形状新增阻断缺陷。admittedSubmit是适配器生命周期flag，同一已提交对象若被可信Go调用者混用Fetch/Batch/Content且不Init，可能改变其配置来源；实际生产poll/content路径会新建/Init并重置flag。**不承诺任意同一对象跨submit/query或多Init复用合同已验证**。descriptor拒绝后info保留其变更并继续failclosed，不缓存拒绝的descriptor。

## 真实本地行为

集中扩展既有host_admission_test.go，不散布新测试文件。实际导出TaskAdaptor.New/Init/Validate/BuildRequestBody/DoRequest，运行真实JavaScript和loopback HTTP：

- 未批准actual config在hook前拒绝：lookup=0、submit=0、reserve=0，403/local/NoRetry。
- 故意把channel cache改成stale配置，再恢复fresh DB但不刷新cache；允许build hook读取已准配置并lookup一次，实际signed submit一次，upstream X-Tier=approved。
- DB禁用后cached BuildRequestBody与直接DoRequest均拒绝，submit仍1。
- DB和actual配置一起改成新tuple且两个tuple都批准：旧descriptor仍拒绝，submit仍1。
- build hook返回unapproved RewriteModel：已批准read-only lookup增加到2，但拒绝后submit仍1；reserve/settle/refund probes均0。此处**不能说所有被拒descriptor网络0**：批准hook可先读取再返回不准目标，断言的是不提交/不预扣。

该fixture直接验证适配器边界，不声称完整controller→钱包扣费E2E；Billing probe未被动调用只说明该测试的拒绝准备不触资金，真正资金守恒未验。Root流程检查通过现有relay回归及代码顺序核对，不拿它冒称全收费链通过。

## 实测

最终含缓存权限/签名/dispatch修复三库：`powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1`，内部 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`，exit0：SQLite3.50.4 1.65s、MySQL8.0.44 0.38s、PostgreSQL17.12 1.14s，包8.851s。随后只把Validate入口拒绝文案统一成现有fixed403/local/NoRetry，新增错误契约assert；相关全部测试再次通过，未更改DB读取/投影/SQL。

最终 `go test -p 2 ./model ./relay/helper ./relay ./relay/channel/task/jsplugin ./relay/channel/providerplugin ./pkg/jsplugin ./plugins ./ext/... -count=1` exit0：model9.493s/helper2.826s/relay5.262s/task8.482s/provider4.185s/jsplugin2.148s/plugins6.101s/domestic8.635s，其余ext通过；随后相关vet与 `go build -p 2 ./...`均exit0。全仓Windows历史失败不据此抹除。

`GOMAXPROCS=2`、进程内GOSUMDB=sum.golang.google.cn/GOPROXY=https://goproxy.cn,direct；没有全局配置/依赖改动。专属MySQL/PG已优雅关闭；没有旧/生产DSN、客户Key或付费模型调用。

开发时一次import半写态unusedcore及一次fixture用不存在BillingSession字段导致编译失败；稳定源码后使用既有Billing接口，完整重跑。没有删除断言或修改usage/math来掩盖失败。

## 范围与剩余

累计31/32官方文件、2433/2500增删行（门禁通过、80%警告正常），剩1个新文件/67行。未闭合：requestplugin完整有序chain和不可选hostveto、最终opaque模型/URL/redirect/fetch、provider chain/WS、背景credential和旧任务冻结延续、candidate pinnedgeneration、affinityeffective关系、无请求规格的Save/目录与type60歧义。全局策略保持默认不安装，不能以拒绝所有未知插件长期冒充功能完成。后续实际必要补丁触及硬上限前必须负责人决策，不静默增加或压缩安全检查。
