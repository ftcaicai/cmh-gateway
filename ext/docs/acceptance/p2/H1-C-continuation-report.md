# H1-C 持久Task报告原源与配置冻结（2026-11-14）

本批给best-effort task report接入可选加密archive，不改变计费屏障、任务终态或报告丢弃契约；TaskContinuations仍默认nil。

## 实现

- TaskReport从原submission Plan取真正scope的report插件与原Instance，保存v1{Version,Instance}+exact source。source/Engine摘要及大小UTF8界限校验，按TaskID/User/ChannelBinding保存，safe InstanceID→Ref写入既有TaskPrivate.RequestPlugins JSON，不新增官方列或明文源/config。
- Frozen先置true，只有全目标Save成功才挂全部refs；失败保留Frozen空集合，不回当前settings。非JSON/过大request等早返回也已在前面freeze。
- ReportTask先检查Frozen/refs完整有序、ID/数量/重复、archiveBinding/sourcehash/runtime编码，再未发布编译原源，整批还原后才能入队。缺档/不完整/空Frozen不回当前Registry。旧非Frozen任务维持原lookup。
- 仍尊重operator当前instance enabled及plugin绑定：删除/禁用/改绑会整批drop，不用当前config/source替代原值。原worker全目标executor/source/目的地准入仍运行，不提供存档绕过权限。

## 独立复核与资源修复

只读复核a3db71e0-58ce-4067-95dc-87c3b8aeea9f未参与开发/未运行测试，指出不完整target集合和新编译制品未计队列预算：完整数量/顺序ID校验已在最终源中；主代理新增**restore前**保守4MiB source/runtime allowance预留，所有早失败释放或转移给原queue释放路径；每report最多16 frozen targets、合计source+runtime<=4MiB，超限best-effort整批drop。

此预算界定的是新增保留制品输入，不等于精确Go/JS engine堆上限；compiler/runtime仍有固有开销与其自身限制，未声称64MiB为总进程内存硬限。旧报告最多256队列记录/64MiB输入规则保留，frozen allowance会更早触发drop。capture可存较多目标但超过恢复上限则不执行，保持best-effort而不影响已接受任务。

## 真实测试

既有ext/domestic/host_admission_test.go集中fixture：真实SubmitIngress report-only plan→TaskReport原sourceA/config original-A→加密Save→TaskPrivate insert/reload，随后Registry同key/version变sourceB、settings变current-B，actual reportworker utils.fetch POST依旧sourceA/original-A。有效网络目标审批绑定原source/config，未关闭门禁作正控。

Save失败Frozen空refs；partial声明第二目标缺ref、operator禁用、17目标超限通过counted真实Store wrapper验证Load0；正常报告以channel接收payload，无任意sleep。缺Store/emptyFrozen调用后即时counter不增只是补充断言，不能独立证明异步无执行；主要无fallback依据是源码整批前置返回与partial/disabled/cap Load0断言。未构造多目标后段坏源的完整异步case，不冒称该组合E2E已验。

## 验证

最终 `ops/.cache/db-test/run-admission-matrix.ps1`运行 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 2.00s、MySQL8.0.44 0.71s、PostgreSQL17.12 1.41s，包11.737s。包含最终资源/禁用校验，safe refs真实serializer与worker在三库通过；专属引擎已优雅停止，无生产/旧数据/付费模型。

`go test -p 2 ./relay/requestplugin ./model ./service ./ext/... -count=1` exit0：requestplugin6.530s/model17.513s/service17.894s/domestic2.215s，其余ext通过；相关vet及完整build均exit0。

累计47/48官方文件、3797/6000增删行。新增精确task_report.go登记，剩1文件/2203行，不作完整收口保证。

## 尚未完成

报告仍best-effort，没有可靠outbox/重放/恰好一次，重启可丢队列；archive丢失不会改变已结算任务/金额。只冻结source/config，当前source/target审批仍可拒旧task，未认证task-scoped continuationgrant、credential refresh旧源和全局启用。candidate generation/affinity、Save目录/type60、启动keycustody/备份/GCM预算/完整failclosed验收未收口。H3/H7同事务RFC独立，未实施钱包接管或BATCH守恒认证。
