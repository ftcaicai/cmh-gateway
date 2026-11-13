# H1-C 提交前耐久档案与Task安全引用（2026-11-14）

本批接**提交侧**可选archive边界；core.TaskContinuations默认nil，未在启动安装，也未接poll resolver，因此不认证完整重启延续。

## 接线

- TaskAdaptor实际checkSubmitAdmission在每个store-active guard先清空Gin旧ref；要求已安装权威ModelGate、host生成PublicTaskID及正User/Channel。单装archive而ModelGate noop会拒，不把cache配置误当actual。
- 通过actual source/config/model审批后，将admittedSubmit建立为true，确保driver实际读取与capture相同的info配置，不再次从cache替换。Capture+Save在credential准备、decoder/build hook及其utils.fetch之前完成；只有Save成功且ref非空才存context。
- cached body、descriptor模型变化、final DoRequest的既有guard仍重复审核/Save，最终成功ref跟随该投影。未改钱包/预扣/结算代码，也未跳过官方计费检查。
- model.TaskExecutionSnapshot只增加Continuation safe ref/digest到**既有private JSON**；不新增官方表列，不放源/config/credential。service.TaskExecutionSnapshotFromContext复制成功ref，既有controller插入task时自然持久化，无新整方法接管。

## 实测

既有ext/domestic/host_admission_test.go实际taskdriver集中fixture安装测试only加密archive：

1. 未批准配置拒绝且旧contextref清空，archive行0、lookup0、submit0、billingprobe reserve0。
2. archive安装但ModelGate noop拒绝，lookup/submit0。
3. 确定性Save失败注入：Validate失败、lookup/submit0、ref清空；positive路径仍是真SQL加密Store，不用mock替代成功存储。
4. 权威配置通过，hook开始前已Save成功；从context→TaskExecutionSnapshot取得ref，按task/user/channelBinding Load得精确实际源。
5. 真model.Task插入和重新查询后PrivateData.Execution.Continuation完整保留，序列化私有JSON不含源或RuntimeJSON；之后task最终目标未批submit0、批准后原signed配置正常提交的既有case仍过。

## 验证

`ops/.cache/db-test/run-admission-matrix.ps1`实际 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` 最终独立审查修复后的三库exit0：SQLite3.50.4 1.84s、MySQL8.0.44 0.83s、PostgreSQL17.12 1.54s，包4.498s（之前修复前pass13.237s保留，不当最终）。涉及既有TaskPrivate serializer变动，三库均实测，不以纯JSON单测替代。专属引擎已优雅关闭；无旧/生产数据或收费请求。

`go test -p 2 ./model ./service ./relay/channel/task/jsplugin ./relay ./ext/... -count=1`全部通过：model13.777s/service24.279s/task2.362s/relay5.596s/domestic2.073s，其余ext过；相关vet及完整build均exit0。曾因fixture缺bytes import编译失败，修正后重跑，不删断言。

累计43/48官方文件、3201/6000增删行，新增两个精确登记文件model/task.go和service/task_plugin_audit.go。safe ref字段本身不代表grant或bearer，不提供跨task恢复授权。

## 独立复核与修复

只读复核55f4c350-13ae-4fb2-abaf-ecd33b303840未参与开发/未执行测试，发现activearchive guard会在TaskRelayInfo==nil时读取promoted PublicTaskID导致panic；主代理在读取前增加embeddednil拒绝，补NotPanics+0lookup断言，最终SQLite/三库、vet/build复验通过。其余已审范围未报告阻断；后续pollresolver/资金恢复不在该结论内。

## 未完成与边界

archive失败目前在提交前拒绝，已有任务缺档的hold/修复/timeout语义还没有接，也未改资金归属。初次Save可产生最终未提交的孤立加密档案，暂不自动删除，避免错误回收仍被任务引用的源；留存/治理另验。save→network及官方task插入不是跨资源原子事务，本批不称exactly-once上游提交或crashfinancial恢复。

后续需要task-bound Load→未发布恢复、poll分组与content凭据注入、授权区分旧任务延续与newsubmit，并明确缺档不会落普通PollFailures自动退款。所有全局Gate和archive均未启用；生产key保管/备份/GCM预算仍未落。用户H3/H7同事务要求保持，不能用此档案逻辑代替钱账方案。
