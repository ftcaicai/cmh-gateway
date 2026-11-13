# H1-C 按精确持久Task解析延续档案（2026-11-14）

本批新增显式GetTaskAdaptorForTask与operation绑定；**没有替换service后台poll factory/调度或缺档hold流程**。core.TaskContinuations仍nil，所有全局Gate仍noop。

## 解析与隔离

- relay.GetTaskAdaptorForTask(ctx,task,purpose)仅poll/content，submit/report先拒。ref存在时必须按真实taskID/userID/channelID及ref Load，缺Store/Load失败不回退currentRegistry；platform和审计TaskPlugin.key须匹配artifact。Load返回后重查取消，避免已取消请求继续Compile。
- Restore仍未发布编译，随后BindContinuationTask验证runtimechannel/public/upstreamfallback与持久task一致，copy Binding/ref/purpose/CredentialAccount及**有效vendor upstreamTaskID**，不持有可变task指针。后续host改变taskID/user/channel/model/account/ref/upstreamID均不能借用resolved实例。
- bound FetchTask/Batch、ParseTask/Batch按poll；ListArtifacts/BuildContent/ConvertVideo按content，在driverhook前检查。boundbatch每member需同任务绑定，emptybatch拒绝；不是已实现多task归并分组。newsubmit冻结拒绝不变。
- completion usage入口也在hook前核对同任务，旧int接口返回0而不运行不匹配task的hook；不改计价/状态/结算代码，不将该值当缺档hold处理。
- 无ref且未安装Store保留原GetTaskAdaptor行为；Store安装且既有TaskPlugin审计但缺ref拒绝，不自动编造历史快照。仍需真实调度侧区分旧task/未存档和安全hold语义。

## 实际三库行为

扩展既有ext/domestic/host_admission_test.go：真实preSubmit加密Save→model.Task privateRef insert/reload→GetTaskAdaptorForTask→实际query本地HTTP成功。Wrong taskID/user/channel/model分别在resolver与已resolved实例再用时拒；account/vendor jobID变更的reuse拒；poll实例不能ListArtifacts，content实例不能FetchTask，submit目的拒；ref存在Store突然nil拒绝不fallback。注入成功Load后取消，返回context.Canceled，未进入恢复编译。

独立只读复核5c6751f9-fc8f-44df-972e-2a6c0bcaaa6b（未开发/未执行测试/无生产收费调用）发现两个防御缺口：Load成功后缺取消重查、绑定遗漏有效upstreamID；主代理已补校验并加入以上具体回归。复核其余已限定task/purpose/no-fallback路径无新增阻断报告。

**注意时机**：runtime/task模型绑定检查位于RestoreCompile之后，driverhook之前；不宣称所有JS都在taskmetadata核对后，compile会执行top-level模块但此时无utils.fetch。Sourcehash仍在compile前验证，archiveBinding先经真实Load验证。

## 验证

最终 `ops/.cache/db-test/run-admission-matrix.ps1`运行真实 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` 全过：SQLite3.50.4 1.84s、MySQL8.0.44 0.76s、PostgreSQL17.12 1.35s，包4.737s。纯resolver不改schema，但actualTaskPrivate+archive读写与旧集中矩阵均真实三库执行；引擎已优雅关闭，无生产或旧数据/付费调用。

`go test -p 2 ./relay ./relay/channel/task/jsplugin ./model ./service ./ext/... -count=1` 全过：relay5.833s/task11.623s/model13.530s/service14.455s/domestic2.107s，其余ext通过；同范围vet及 `go build -p 2 ./...`均exit0。一次fixture漏relaychannel别名import导致编译失败，补齐后稳定重跑，不删目的隔离断言。

累计44/48官方文件、3344/6000增删行，新增唯一精确relay/relay_adaptor.go登记；仍留4文件/2656行收口空间，不承诺预算保证全部完成。

## 未完成

后台service.TaskPollingAdaptor由main注入当前platform factory，本批没有替换；需要task-aware factory注入、ref/account分组与冻结transport投影/当前凭据安全注入。现存缺档不会自动进入新安全hold，因为尚未接调度；不能贸然激活将Unavailable送入普通PollFailures/timeout财务终态。content凭据/targetgrant延续、report原源、currentsource撤销后task-scoped授权仍缺。所有active source/target gate照旧执行，拿ref不自动绕过任何operator政策。金融同事务RFC保持独立，未认证BATCH资金守恒或crash exactly-once。
