# H1-C Provider多步请求与WebSocket运行检查（2026-11-13）

全局策略仍未安装。本批只复核实际source/config/model/group运行权限，不把它称最终URL/opaque正文/重定向授权，也不涉及钱账/usage公式。

## 接线

- 每个sendStep在目标/headers/签名准备前和实际HTTP dispatch前检查；每次网络返回后、credentialRejected之前检查；readStep入口、全body读取后convertResponse之前及返回结果后复核，拒绝关闭response body。
- WebSocket每次dial（含renew重试）前、真正DialContext前、每个初始messages发送前检查。
- streamConverter保留实际Gin/RelayInfo与冻结hookCtx；BaseURL/Path/Config必须仍匹配actual info/a.config。每次读取pending输出前、source.next前后、convertStreamEvent后queue之前、各send回写前检查。拒绝关闭上游，不能借新批准runtime使用旧hook上下文。
- runtime拒绝为脱敏403/SkipRetry，经过官方Compatible NewOpenAIError包装仍保留，而不是中途已有work后变通用500重试。
- noop立即通过，不加默认DB/Gin依赖；已交付内容与原官方中断计费不改。

## 独立发现与实际失败

独立只读复核ab7c0334-e3e7-483d-9b1d-48d458745007（未开发/未运行测试、无DB或收费调用）指出：

1. stream拒绝Close→stopIdle把timer置nil，prime错误收尾又Stop导致新panic；实际WS撤销fixture已复现time.Timer.Stop panic，修复为同mutex下nil检查并添加NotPanics。
2. bare ErrChannelAdmissionDenied被CompatibleHelper包装成retryable500，部分chain已做work可能重复派发；checkExecution统一typed403/SkipRetry，冻结stream mismatch也同typed，实际错误包装+ShouldRetryRelayError断言不重试。

复核重读确认timer nilguard落点；没有发现生产scanner读取身份字段的新并发race，但race工具仍未验，不据此宣布原子授权。

测试最初StreamingTimeout未初始化（0），允许WS正控立即超时；显式设5并恢复。随后源码尚未接stream检查时撤销仍返回nil，作为真实红测保留。最后pending撤销主动关闭WS，调用者再次Close返回net.ErrClosed，测试仅接受这个精确已关闭错误，其他Close错误仍失败；没有放宽实际readErr或泄露字节断言。

## 真实行为测试

扩展既有host_admission_test.go，同一三库fixture，真实导出provider.Adaptor.DoRequest与response.Body.Read，loopback HTTP/WS，不用定时sleep/纯helper：

- 正常chain完成两个HTTP请求并返回转换body {ok:true}。
- first请求的上游handler先在DB禁用渠道再返回，运行检查阻止响应转换/第二请求，总calls=1，typed403且不重试。不能说整个请求network0，第一次本来被批准。
- 正常WS握手1、初始message1，返回转换body。
- 上游读到初始message后先禁用DB再发第一frame，首转换拒绝而非panic；握手/初始message均只有既有1次。
- streaming WS先批准并prime出pending转换事件，caller读取前禁用DB，Read返回拒绝且零pending字节交付，底层关闭。

直接测试适配器边界，不声称完整收费控制器E2E、所有WS callback/握手renew分支均有独立主动case；这些落点代码+既有默认包测试验证，不以数量替代证据。

## 验证

最终 `ops/.cache/db-test/run-admission-matrix.ps1` 加载ignored专属本地DSN执行 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 1.68s、MySQL8.0.44 0.53s、PostgreSQL17.12 1.33s，包9.248s。既有映射/事务/目录/价格/预扣前/真实Responses复用/taskdriver回归同跑。

最终相关模块 `go test -p 2 ./model ./relay/helper ./relay ./relay/channel/providerplugin ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/... -count=1` exit0：model11.634s/helper2.812s/relay0.410s/provider0.210s/requestplugin1.094s/jsplugin2.033s/plugins0.802s/domestic1.992s，其余ext通过；相关vet及完整 `go build -p 2 ./...` exit0。进程GOMAXPROCS2/SUMDB镜像不改全局。专属MySQL/PG已优雅退出，无生产/旧系统/付费上游。

## 未完成

目的地（含plugin url/query/bodyopaque model、redirect、utils.fetch）、后台credential/source冻结、既有持久任务延续能力、candidate generation/affinity关系、Save/目录/type60仍未闭合。check→JS/network之间并发TOCTOU不是本批原子保证。共享executor策略和model策略仍默认noop，不能提前启用。

累计37/48官方文件、2710/6000增删行，逐文件登记门禁通过。下一批实现最终目的地值DTO与host transport/plugin fetch权威检查，不用source审批替代目标关系。
