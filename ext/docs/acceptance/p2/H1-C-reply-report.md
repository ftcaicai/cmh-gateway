# H1-C Reply与Report执行组预验（2026-11-13）

限定范围：冻结request-plugin执行者tuple的响应检查与报告worker预验。全局Models/PluginHooks/Executors仍默认noop；不是完整H1、最终目的地或金融认证。

## 实现

- 从stage既有投影提取公开RequestExecutorIdentity：实际Engine源必须非空且等于Loaded编译摘要、key与实例一致；版本化envelope仍为Version/Role/Path/Stage/完整Instance，原Config（含关键词列表）只进入hash。既有request_stage摘要字节合同不变。
- 每个reply batch在消费pending/fed或执行任何hook前预验全部未跳过target。request_reply/reply与stage审批隔离；host拒绝直接形成failed stop，不进入optional错误跳过分支。后续batch重新查询当前审批，已经交付的旧输出及官方中断计费不改。
- reportJob.run在worker消费时、组装record或任何hook网络前预验全部target。角色request_report/report；任一个拒绝整组报告丢弃，不倒改已完成请求/资金。ordinary与ReportTask构造的任务报告使用同一worker，因此实际执行点共用检查，无需修改task_report.go。

## 实际行为与独立复核

既有ext/domestic/plugin_test.go扩展公共Ingress/Reply测试：两个真实optional=true reply插件、本地HTTPfetch。

- 只批准first、second未准：返回500请求插件失败，fetch0，成功正文不释放；全批准：原正文不变且fetch2。
- 真实SSE首batch允许交付first并fetch2，然后撤销second；下一batch无额外fetch、被撤销文本不释放、出现request_plugin_failed。不是只测私有helper。

既有relay/requestplugin/requestplugin_test.go新增唯一集中worker测试：同步调用实际reportJob.run，真实JS report→loopback POST计数，无sleep/后台job轮询。

- ordinary record与task record两分支：second未批first也不先fetch；全准依序fetch2并携带正确task payload。
- job已构造后撤销审批，消费时fetch0；registry同key新源发布后，旧job按实际旧制品批准执行，不偷偷替换current源。
- stage角色审批不能批准report；路径或原配置改变整组fetch0。
- task record证明共用worker分支，**不冒称ReportTask重建/排队全E2E、持久任务冻结能力已完成**。

只读独立复核13d5ffa5-13f4-4961-9b8b-4bea4c4d442b（未开发、未测试/DB/生产收费调用）核对HTTP/socket/dryrun call.Path初始化、optional不能吞host stop、worker消费时整组检查、原stage合同保持，未见限定范围新增阻断缺陷。生产scope构造target均非nil；不保证可信Go手工构造损坏niltarget列表不会panic。一个batch/job检查不是跨hook原子撤销；不声称任何失败请求所有后续report网络都为0。

## 验证与失败纠正

首次公共reply红测（实现未接线）真实fetch2且成功正文被释放；接线后拒绝变fetch0。允许fixture最初摘要失配，因为审批使用加载前Models=nil，而host加载规范为Models=[]；改为从实际冻结host实例投影，不放宽exacttuple。一次并行写入半态unusedimport编译失败，稳定后重跑。

最终进程GOMAXPROCS2、GOSUMDB=sum.golang.google.cn、GOPROXY=https://goproxy.cn,direct：

`go test -p 2 ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/... -count=1` exit0：requestplugin1.005s/jsplugin2.022s/plugins0.718s/domestic7.351s，其余ext通过；`go vet ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/...`及 `go build -p 2 ./...`均exit0。

没有数据库行为更改；本批内存执行组测试不启动MySQL/PG，不声称新增三库验证，既有SQLite hostfixture随ext同跑。没有生产/旧系统连接、付费上游调用、资金/usage公式修改。

累计35/48官方文件、2626/6000增删行，精确登记包括既有官方worker测试文件；主代理复核diff与门禁通过。

## 下一边界

executor tuple预验不是model/channel/目的地授权。接续provider多步chain/WebSocket、最终URL/body模型/redirect/fetch、背景凭据与既有任务冻结延续、candidate generation/affinity/Save目录/type60关系。全局策略在这些闭合前不启用；资金同事务仍独立RFC。
