# H1-C Task提交与查询最终目标（2026-11-14）

限定目标网络能力检查；不是持久任务source/config/model冻结完成。所有业务Gate仍默认不安装，资金同事务RFC不变。

## 接线与角色

- 任务新提交本已通过DoTaskApiRequest→共享doRequest，本轮只增加真实拒绝/批准证据，不重复新增发送器。
- 单查FetchTask与批查FetchBatchTasks向doFetchDescriptor携带真实buildQueryRequest/buildBatchQueryRequest角色。实际signed URL/HTTP Method/有效Host在client获取/Do之前审批。
- task_query tuple含实际plugin源与BaseURL/proxy/settings网络能力、hook和目标摘要；Model/Channel必须zero，不替一批任务伪造单个模型/渠道，也不以当前公开模型门禁阻断旧任务读取。
- 查询已有redirect-follow行为保留；每个新hop在原credential清理/恢复后重新审actualURL/Host/Method，随后仍过原SSRF、HTTPS降级/10hop等限制。Go303将POST改GET，审批取新Method而非借旧POST权限。
- 省略hook的旧内部测试/调用在noop仍兼容，active缺失或非queryhook拒绝。错误fixedrequestrejected，不含signedquery/密钥。

## 真实测试

既有ext/domestic/plugin_test.go真实New(plugin).FetchTask/FetchBatchTasks驱动JS builder+loopback HTTP：

1. 空目标集合单查network0；精确关系批准后单查1，任务的公开名故意不是当前发布模型，查询不靠新调用门禁。
2. 单查权限不能批准batch URL/method/hook；明确batch批准POST成功。
3. 初始redirect目标批准、第二target没批，第二服务network0；两hop分别批准后正常跟随。
4. POST303改GET：第二目标只批POST仍network0，改批实际GET后成功。

该测试显式允许loopback临时端口并恢复官方fetchpolicy；首次redirect正控失败“port not allowed”是原SSRF规则真实生效，修正fixture设置而非生产规则。

既有真三库host_admission_test.go的actualTaskAdaptor：只批准buildSubmit hook lookup能力，lookup成功但finalsubmit无审批时submissions=0；再批准exactrelay_http submit关系后成功并保留signedX-Tier。原hook/descriptor/cachedbody等集中回归同跑。

只读独立复核cfe0ba40-796a-4598-af9d-05ad87b0f7bf（未开发/未测试/无生产收费调用）核对signedURL时机、redirect Host fallback/303 effectiveMethod、逐hop源重新投影、credential原清理语义与原fetchpolicy保留，未发现限定能力检查阻断fail-open。建议的303不同方法case已补实测。普通check→network不是原子撤销认证。

## 验证

最终tasksubmit+既有集中实三库：`ops/.cache/db-test/run-admission-matrix.ps1`内部go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v，exit0：SQLite3.50.4 1.62s/MySQL8.0.44 0.54s/PostgreSQL17.12 1.38s，包3.928s。独立query/redirect本身不读DB，在相关ext全套同跑，不伪称持久任务重启/金额守恒三库通过。

`go test -p 2 ./relay/channel/task/jsplugin ./relay/channel ./relay ./service ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/... -count=1` exit0：task1.340s/channel5.529s/relay8.443s/service18.211s/requestplugin1.029s/jsplugin2.190s/plugins5.915s/domestic2.045s，其余ext通过；相关vet与完整go build -p2 ./...均exit0。进程GOMAXPROCS2/Go镜像设置，无全局改动。

一次并行半态doFetchDescriptor新参数与旧签名编译失败，源稳定后重跑，不放宽断言。专属MySQL/PG已优雅退出，无生产/旧数据/付费模型请求。

累计39/48官方文件、2887/6000增删行，沿用已登记task文件并更新函数范围，门禁通过。

## 未完成

task_query网络能力不证明hook前持久source/config归属、历史任务snapshot或content下载finaltarget。不得因目标获准就放弃当前钱账原子性，也不能拿拒绝所有旧任务冒充安全。后续要冻结提交时的执行与目标能力、重启后poll/content延续、源升级兼容和退款/结算维持，再解决candidate generation/affinity/Save目录及启动真实failclosed加载；完整H1之前不激活。
