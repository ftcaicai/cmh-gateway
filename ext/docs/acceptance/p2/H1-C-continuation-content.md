# H1-C 公共artifact内容读取按档案恢复（2026-11-14）

本批接实际TaskArtifactContent/VideoProxy读取侧，保留原owner/signedURL认证。全局业务Gate与TaskContinuations仍未安装；不是task-scoped旧source/target grant全部完成。

## 实现

- controller/task.go的projectTaskArtifacts原有独立currentRegistry路径也改为ref任务content用途解析，列表与builder来自存档源。initTaskArtifactAdaptor按requestctx恢复旧源，当前type61+pluginbinding匹配渠道只提供现有credential key，不用Init替换冻结runtime。
- TaskAdaptor.AttachContinuationCredentials(ctx,task,currentChannel)先校验boundtask/contentpurpose及currentID/type/pluginbinding，沿原account选取，只有key注入；ApiKey仍不在档案。privateTask.Key存在时保留原优先级，但bound实例会核对其fingerprint不能被reuse替换。
- proxyTaskMedia对ref任务独立取ContinuationChannel frozen transport，不再借当前base/proxy/settings。当前sourceB/configB/baseB不能替换原A内容目标/签名。
- ref任务缺archive/列表/descriptor失败，TaskArtifactContent固定503；VideoProxy不再偷偷回退持久rawResultURL。无ref legacy fallback行为不变。Range/HEAD/SSRF/redirect/auth原控制保留，inline data本地解码不虚构网络审批。

## 真实公共端点测试

既有ext/domestic/host_admission_test的真实加密Save→TaskPrivate ref→owner TaskArtifactContent：源A/config approved/baseA存档后，当前Registry同key/version换sourceB（不同artifactkey），DBchannel换configB/baseB及live statickey。owner请求仍206/data、X-Revision A、X-Tier approved、Authorization live fixturekey，B目标服务counter0；Range保持。

foreignowner返回404且media次数不变；Store=nil时Artifact和VideoProxy均503，storedResultURL指向fallbackservice但network0。pre-canceled owner请求503/no media；已boundcontent任务PrivateData.Key被替换则Attach拒绝。

## 独立复核与修复

只读复核d88f6334-ef6d-4ac9-9c7a-8df3af6446fe（未开发/未执行测试/无生产收费调用）发现archive/credential准备原Background取消脱离、boundprivateKey复用可变+失败reattach旧key残留；主代理已传播handlerctx、Attach先清key并拒绝预取消、使用ctx刷新及绑定privateKeySHA，补上述具体回归。复核最终重读确认关闭，未报告本限定范围其他阻断。

共享credential refresh singleflight一旦启动仍按官方设计后台继续；requestctx限制调用方与pre-start检查，不宣称取消能中止所有共享网络。ListArtifacts接口本身仍无context；编译top-level执行与后续Bind时机沿前批限制，不叫所有JS完整取消认证。

## 验证

最终 `ops/.cache/db-test/run-admission-matrix.ps1`实际 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 1.83s、MySQL8.0.44 0.61s、PostgreSQL17.12 1.30s，包4.025s。真实owner lookup/TaskPrivate/archive及媒体读取均在三引擎集中case，不用纯policy替代。此前14.733s为修复前矩阵，不作为最终源码证据。专属引擎已优雅退出，无旧/生产或收费模型调用。

定向controller artifact/media tests6.030s；相关relay5.844s/task11.733s/service18.401s/domestic2.616s及其余ext tests全部过；对应vet与完整build均exit0。累计46/48官方文件、3599/6000增删行，新增精确controller/task.go登记，剩2文件/2401行，不作全部收口可容纳保证。

## 未完成

旧source/config/target授权仍沿当前installed gates，不会因为ref而自动豁免，完整task-scoped continuationgrant及credential refresh旧源/配置未认证；报告原源冻结、candidate generation/affinity、Save目录/type60、启动keycustody/备份/GCM预算/完整failclosed activation仍缺。无资金算价/结算代码修改，不认证BATCH金融守恒，H3/H7同事务RFC保持独立。
