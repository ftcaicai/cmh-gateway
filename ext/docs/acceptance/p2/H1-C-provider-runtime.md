# H1-C 普通provider运行前检查（2026-11-13）

本批只验证type61普通provider请求的实际运行配置，**不安装全局ModelGate/PluginHooks，不代表完整H1或资金验收**。

## 接线

- model.CheckPluginExecution将实际运行快照与freshDB精确制品/模型/配置投影比较，复核当前启用状态、模型及所选group仍属于渠道，实际UpstreamModelName必须与映射目标一致。
- provider从实际RelayInfo与已绑定a.config重建检查快照，不从DB伪造selected值。验证context ID/type/public/group、exact pinned source、有效配置相同，并拒绝未知retained request plan。
- 在bind选配置后、初次credential刷新/key exchange之前检查；prepared复用、DoRequest准备后conversion前、renew与直接send前再次检查。default noop不增加DB依赖。
- 准入激活时，只有全部bind/refresh/token准备成功才设prepared。失败不能靠同adaptor第二次调用跳过key准备。默认noop仍保持原路径。
- credential刷新服务实际按当前registry重新解析plugin，不能用old pinned批准新源刷新。因此credential非nil时保守要求current plugin指针等于actual a.plugin，否则先拒绝。包括未过期旧credential也会拒绝，是明确兼容限制，不暗称完整旧generation延续已实现。

## 独立发现与修复

只读复核 `939da1c6-e84b-41ea-9985-905107db1480`（未开发、未运行测试）指出三项：runtime漏group撤销、refresh service会替换成current新源、prepared过早导致失败后绕过准备。均按上述修复。只读复核 `de0af28c-3faa-4f18-a04d-cfc310a55dde`核对三项代码级闭合、active nil前置条件与context一致性，未发现本次限定修复新增阻断问题；未替代主代理运行验证。

## 真正运行的行为回归

扩展既有host_admission_test.go同一三库fixture，actual provider.Adaptor.DoRequest→真实loopback HTTP server，不用helper返回值代替网络观测：

1. 实际RelayInfo改成未批准config：exchange=0、dispatch=0。
2. 允许路径：exchange=1、dispatch=1（正控）。
3. 不同fixture key的exchange首次返回502并抛错：累计exchange=2、dispatch仍1。恢复后同一adaptor重试必须再exchange：累计exchange=3、dispatch=2。
4. prepared adaptor主target改成未批准值，freshDB删除using group，或fresh配置改变：均拒绝，dispatch仍2、无新增exchange。
5. 已选旧plugin发布同key/version新源，构造过期plugin credential调用实际adaptor：refresh=0、dispatch仍2；拒绝发生在service解析新源之前。

手工构造RelayInfo最初未设UsingGroup，新检查正确拒绝了“允许”fixture；显式初始化default后通过。不是删group检查放宽生产。测试源码带模拟凭据但均为合成fixture，不是生产Key。测试不调用DoResponse/钱包结算，不把此计数叫收费守恒。

## 最终验证

`powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1` 加载ignored专属DSN后执行 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`：exit0。

- SQLite3.50.4：1.80s；MySQL8.0.44：0.56s；PostgreSQL17.12：1.21s；包4.018s。
- 同跑之前的渠道事务、候选/目录/价格/预扣前、真实Responses连接复用等回归。故意关闭DB的错误日志是既有fail-closed断言，exchange失败日志是本批明确失败fixture。
- `go test -p 2 ./model ./relay/helper ./relay ./relay/channel/providerplugin ./pkg/jsplugin ./plugins ./ext/... -count=1` 全部通过：model7.734s/helper2.990s/relay6.216s/provider4.473s/jsplugin2.228s/plugins0.870s/domestic2.113s，其余ext通过。随后 `go vet ./model ./middleware ./relay/... ./pkg/jsplugin ./plugins ./ext/... ./controller` 与 `go build -p 2 ./...`均exit0。
- 专属MySQL与PG已优雅停止；没有旧/生产连接、付费模型调用、数据库结构或金融逻辑更改。

## 安全验证边界

参考[OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)及[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)：本批只在既有credential准备前追加server-side veto，不新增登录/会话/令牌协议；错误不包含key、原配置或上游地址。未做ASVS整体合规认证，不声称TLS/credential存储/全部OAuth路径已验。

明确未覆盖：检查与后续service/network之间的并发TOCTOU、背景refresh/login/scheduled/revoke，chain.go sendStep与provider WebSocket派发，opaque最终body模型/URL/redirect，任务driver提交与descriptor变化、requestplugin有序chain，旧任务冻结延续。当前source-only核心策略也未安装。refresh新源的确定性替换问题已保守阻断，不代表跨节点原子network授权。

累计28/32官方文件、2328/2500增删行，逐文件登记门禁通过并输出80%警告；剩4文件/172行，后续不压缩漏逻辑凑上限。所有未闭合边界必须留在下一批，不提前激活。
