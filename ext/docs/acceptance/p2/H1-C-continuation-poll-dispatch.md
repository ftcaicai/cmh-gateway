# H1-C 档案任务接入真实poll派发（2026-11-14）

本批接实际DispatchPlatformUpdate，不再只显式resolver；**不改原timeout资金deadline**，不安装core.TaskContinuations/Models等业务Gate。

## 实现

- main注入service.GetTaskContinuationFunc(ctx,task)：relaytaskboundLoadRestore→ContinuationChannel fresh冻结transport，只从当前type61且pluginbinding匹配的渠道取现有Key。不会以currentconfig/source/status/发布状态替代frozen controls，也不复制ApiKey进档案。
- TaskAdaptor.ContinuationChannel返回独立BaseURL/settings/OtherSettings/HeaderOverride/ParamOverride快照，无Key；现有account选取继续运行。
- service.DispatchPlatformUpdate在currentplatformfactory之前将ref任务独立派发，不mutatecaller map；不合并不同ref/account。per_task走原updateVideoSingleTask；batch-only走原pollTaskBatch的**singleton batch**，保留builder/parse合同而不是强行调用不存在的buildQueryRequest。无ref remainder仍原batch/video逻辑。
- 缺resolver/archive/冻结channel不可用只跳过**本次poll pass**并固定publictask/channel日志；Fetch/Parse的ErrContinuationUnavailable（含batch）不记普通PollFailures。其他transport/HTTP/hook分类、CAS、终态结算与退款保持原链。
- Archivedtask有persisted CredentialAccount时，读取凭据前先检查该plugin/account仍在当前传入credential lines；静态Key不能替换原账户，缺account固定Unavailable，不能问另账户“任务是否存在”。仅boundarchived路径新增，不改变普通task兼容行为。

## 明确deadline边界

sweepTimedOutTasks及其CAS/refund未修改。缺档不增失败计数仅限本次派发，**不是无限hold、不是承诺缺档永不退款**；到原TaskTimeoutMinutes仍按官方timeout流程。完整人工修复、紧急撤销、延长deadline及金融同事务方案还未验，不擅改财务决定。不能提前激活并把该边界说成全部财务恢复完成。

## 真实测试与独立发现修复

扩展既有host_admission_test实际encryptedSave→Taskinsert/reload→真实service.DispatchPlatformUpdate→loopback query+原CAS：SUBMITTED推进IN_PROGRESS，quota13保持，currentplatformfactory调用0。Store=nil后再派发，状态/failures/quota13保持，不fallback。测试调用实际Dispatch而非完整定时runner/sweep，未认证deadline终态资金行为。

只读独立复核e0cb10c7-e93d-4fbd-9e22-7f434e6f5779（未开发/未执行测试/无生产收费调用）发现：

1. 初版所有archive任务强行per_task，会使合法batch-only源missinghook计失败甚至退款；主代理改batch-mode singleton原batch链，补仅有batchbuilder的真实插件正控，IN_PROGRESS/failures0/quota13。
2. 原credentialhelper静态Key捷径可能替换archivedtask原account；主代理补boundaccount明确存在检查，fixture原account→staticKey返回Unavailable/no dispatch。

复核其余已定范围未报告阻断，不能改写成“审查全无问题”。fixture初次status断言string与TaskStatus类型不符，两次修正类型后原状态值断言保留。

## 验证

最终 `ops/.cache/db-test/run-admission-matrix.ps1`执行实际 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 1.89s、MySQL8.0.44 0.70s、PostgreSQL17.12 1.56s，包4.460s。包含最终single/batch/account修复，不拿之前6.366s矩阵冒充最终。引擎已优雅停止，无旧/生产数据或付费请求。

最终 `go test -p 2 ./service ./relay ./relay/channel/task/jsplugin ./model ./ext/... -count=1`全部过：service24.048s/relay9.053s/task6.347s/model8.624s/domestic2.061s，其余ext通过；相关vet与完整build均exit0。

累计45/48官方文件、3470/6000增删行。新增精确service/task_polling.go登记，main原已登记但新增factory用途单独登记，仍保留80%范围复核。

## 尚未完成

source/target准入仍照当前operator规则，可拒旧任务，尚无完整task-scoped旧grant延续；static/credential刷新旧源和contentkey注入需进一步验。report冻结、candidategeneration/affinity、Save目录/type60与启动failclosedconfig/keycustody备份/GCM预算未收口。非终态quota保持不等于BATCH开关金融守恒，未实施H3/H7接管。
