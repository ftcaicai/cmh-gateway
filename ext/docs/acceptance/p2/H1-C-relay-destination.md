# H1-C Relay最终HTTP/WS目的地关系（2026-11-13）

本批限定relay transport URL/Host关系审批。全局Destinations/Models/Executors/PluginHooks仍默认noop；**不等于utils.fetch、后台网络或opaque正文model已审**。

## 合同与接线

- core.DestinationAdmission仅含Kind/Method、完整URL摘要、Host值摘要、实际public/upstream及渠道ID/type。URL的path/query可能带凭据，不传原文、不进错误。独立DestinationGate默认允许保持上游，不依赖业务model/service/relay。
- DestinationAddress由operator与host同用：strict absolute http/https/ws/wss，拒绝userinfo/fragment/opaque/空host/非ASCIIhost/backslash/控制空白/超64KiB/坏UTF8，scheme/host小写、空root补/，其余端口拼法、query顺序、escaping保持exact。没有wildcard/prefix/民族词推断。
- DestinationPolicy私有exacttuple关系集合，构造拒绝无效/重复/超4096项且不返回半成品，空集合拒绝；不自动信任channel BaseURL、plugin allowedHosts或源摘要。
- HTTP最终发送前取actual req.URL/Method/req.Host；WS sharedDoWss及provider privateDial在签名、scheme转换之后取最终URL与显式Host header（空表示URLauthority），URL摘要已单独绑定authority。不是两个transport统一的“effectiveHost”字段。
- 选择context ID/type/public与实际info必须一致，actual upstream非空；拒绝固定403/SkipRetry且隐藏地址。默认noop不加nil/context/DB依赖。
- 普通relay现有HTTPclient本就拒绝跟随redirect，本批保持3xx返回，不暗中开redirect。不能把这个称作所有redirect出口都逐跳审批。

## 真实行为与审查

既有policy_test覆盖exactrelation不串channel/model/method/target/Host、坏URL拒绝、querydeployment变化撤销匹配、构造不激活、错误/重复失败关闭。

既有plugin_test调用真实channel.DoRequest→本地HTTP：精确URL+query+Host批准成功1次；相同host但querydeployment不同、另一个目标、custom Host未准均固定403不重试，无新增请求。批准/redirect本身得到302但第二服务计数0，证明官方relay不跟随。

既有三库host fixture驱动真实provider converted WebSocket：空目标审批即使源与模型允许也**dial=0/messages=0**；精确signed wsURL+Host关系获准后dial1/message1并正常转换body。该fixture也同跑之前chain/stream撤销回归。sharedDoWss接线由源码复核及包默认回归验证，未单独为每种adaptor构造同样WS目标case。

只读独立复核5b80a8e2-fc80-463e-90a3-b17d3129c454（未开发/未测试/无生产收费调用）发现host全部小写会把IPv6 zone的大小写敏感interface标识合并；已严格拒绝zone-bearing host并保留大小写两个输入均拒绝测试。复核最终重读确认修复，未发现本transport-only范围新增阻断fail-open；不证明DNS/IP pinning或SSRF隔离。

## 验证

最终真三库 `ops/.cache/db-test/run-admission-matrix.ps1`（加载ignored专属本地DSN后go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v）exit0：SQLite3.50.4 1.85s、MySQL8.0.44 0.74s、PostgreSQL17.12 1.47s，包4.355s。

进程GOMAXPROCS2/SUMDBsum.golang.google.cn/PROXYhttps://goproxy.cn,direct。`go test -p 2 ./model ./relay/channel ./relay/channel/providerplugin ./relay ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/... -count=1` exit0：model13.107s/channel8.574s/provider0.202s/relay5.385s/requestplugin4.242s/jsplugin2.218s/plugins6.240s/domestic2.001s，其余ext通过；随后相关vet和 `go build -p 2 ./...`均exit0。专属MySQL/PG已优雅关闭，无旧/生产DSN、客户Key或收费模型调用。

## 不能提前认证的部分

DestinationPolicy只证明explicit URL关系，actualinfo仍要另过模型/source/config等门禁；exact查询审批可能需要operator预先审核固定目标关系，不能擅自放wildcard为动态deployment填坑。opaque正文中的模型需可信固定源/配置+host语义证明，不用通用字符串搜索冒充。plugin utils.fetch/credentials/login/scheduled/task queryredirect等独立出口仍待接，DNS解析不在该URL摘要中。没有全局安装/生产发布/钱账守恒或ASVS完整认证。

累计38/48官方文件、2754/6000增删行，精确登记通过。下一批接plugin fetch/后台网络的实际目标，不把本批relay共用发送器叫唯一网络出口。
