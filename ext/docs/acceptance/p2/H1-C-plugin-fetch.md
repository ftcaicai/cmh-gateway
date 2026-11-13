# H1-C Plugin utils.fetch目的地能力（2026-11-13）

本批不是完整H1。全局Models/Executors/PluginHooks/Destinations仍默认noop，不增加启动安装。资金同事务RFC独立。

## 独立网络能力合同

- DestinationAdmission增加Executor与Hook。plugin_fetch类型**必须**Model/Channel全零，不为未选渠道的ingress/report或credential login/refresh伪造模型/渠道。relay_http/websocket反而禁止携带plugin Executor/Hook，避免关系串用。
- RequestPluginFetchExecutor对实际编译源+原始实例Config形成request_fetch v1摘要（不拿keyword summary代替原配置）。
- PluginFetchExecutor对实际编译源+BaseURL/Proxy/ChannelSettings形成channel_fetch v1网络能力摘要；**该API没有ctx.config/channelID，所以不是完整hook配置或某个渠道授权证明**。每个具体hook/目标/方法仍需单独operator审批；源非空且必须与实际Engine一致。
- 两个fetcher每次调用重建actual能力身份，保留已有allowedHosts/configURLhosts校验；sendPluginFetch构造最终HTTP请求后，按实际URL/req.Host/Method和runtime填的Hook+Executor审批，在创建client、读取BodyStream或任何网络前拒绝。
- 原TLS/proxy、JS fetch次数/时间/body限制、禁止自动跟随redirect均保留，explicitapproval不能覆盖这些官方限制。错误不包含地址、配置、Key或摘要原文。

## 实际行为

扩展既有ext/domestic/plugin_test.go，无新增官方测试文件。真实JavaScript CallTransform(ingress)及CallRefresh内utils.fetch→loopback HTTP：

1. 空目的地审批拒绝，网络0；精确request_fetch+ingress+URL关系批准后成功1。
2. 同URL原Config marker改变、实际Hook改report、同key/version源替换分别拒绝，网络不增加。
3. 未批准POST BodyStream读计数0；不是先读取/联网再审。
4. request_fetch关系不能授权credential channel_fetch；明确channel_fetch+refresh+POST+目标关系批准后成功。没有假model/channel。
5. 只批准redirect URL，HTTP3xx到另一个服务不跟随，第二服务网络0。

现有policy_test补充plugin_fetch假channel/model拒绝、hook变更拒绝、relay/plugin身份不得混入。构造政策不自动激活，空集合deny。

## 独立复核

只读复核33dc8568-caad-4d96-9aa7-278c8ce02351（未开发/未测试、无DB/生产收费调用）检查实际send callsites、bodyStream不读前置、HTTP Host语义、原fetch限制和凭据调用路径；未发现本限定能力边界的阻断性fail-open。HTTP req.Header里的Host被Go client忽略，guard hash的实际req.Host仍是URLauthority；不宣称支持手工Host header绕转。

channel_fetch不足完整ctx.config/持久任务归属是明确未完，不把网络能力摘要叫完整配置身份。该gate控制任何实际fetch目标，但后台hook本身可执行的权限/冻结归属仍须另接；不是授权整个后台driver。检查与网络之间不是跨节点原子撤销。

## 验证

最终 `go test -p 2 ./service ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/... -count=1`通过：service18.251s/requestplugin4.770s/jsplugin2.034s/plugins2.062s/domestic7.571s，其余ext通过；`go vet ./service ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/...`及 `go build -p 2 ./...`均exit0。环境仅进程GOMAXPROCS2/SUMDBsum.golang.google.cn/PROXYhttps://goproxy.cn,direct。

本批无DB读写行为改动：真实JS+内存tuple+HTTP；既有SQLite hostfixture随ext同跑，未启动MySQL/PG，不声称又做真三库。没有生产DSN/旧系统数据/付费模型或session协议修改。一次并行重构半态私有send签名与调用方不一致编译失败，稳定后重跑，不削弱断言。

累计39/48官方文件、2841/6000增删行。达到新专项**文件数80%**，按规则先记录范围复核：剩余9文件/3159行，需覆盖task普通提交/后台query实际发送、背景credential完整冻结、candidate generation/affinity、Save目录/type60及最终激活验收。可复用已登记文件和现有core值合同，不因此放行未知执行者；如实际实现超预算，仍停并提真实差异，不再凭估计保证全部可容纳。
