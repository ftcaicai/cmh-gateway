# H1-C 授权轮询重定向验证等待跨到期（2026-11-14）

状态：动态边界已补齐并完成真实三库验证；无生产源码改动、不安装全局Gate/Store/authorizer。

有限taskgrant首批已在Engine排队跨到期与既有adaptor到期后拒绝真实验收；当时文档明确redirect跨到期只有代码复核。本批补真实网络边界：初始query返回303、初始与后继目标均明确批准，取得与TaskAdaptor相同cachedHTTPClient，在共享CheckRedirect验证回调内用可观测barrier与显式ExpiresAt计时器跨到期，再返回nil。此时TaskAdaptor必须在共享验证后再次检查scope、拒绝后继派发。

测试必须观察：callback实际进入且进入时grant仍有效、initialQuery=1、approvedRedirectTarget=0、返回errors.Is(ContinuationUnavailable)。后继目标有独立真实handler counter，避免只检查未使用counter而假阳性；target关系预先通过独立DestinationPolicy.CheckDestination正控，确保拒绝原因是到期而非未批准URL。

只对本地fixture exactURL临时替换实际client.CheckRedirect，委托原共享检查后才阻塞、defer恢复，Go上下文deadline防挂；不改生产clock或全局可注入时间、不用固定sleep。已发起网络动作可完成、不强杀/不回滚；原tasktimeout不变。最终 `run-admission-matrix.ps1` / `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 7.62s/MySQL8.0.44 6.77s/PostgreSQL17.12 6.73s，package26.865s；本子场景分别2.39/2.77/2.35s。实际callback恰好1次、进入仍未到期，两个target均CheckDestination正控通过，initial1/后继0/Unavailable全部断言通过。专属引擎已优雅退出。相关pkg2.318s/task1.382s/relay0.439s/service14.461s完整tests、相关vet/fullbuild exit0；ext全套domestic7.740s及其余ext通过、ext vet exit0。

独立只读412a4368未参与开发/未执行测试，发现初版fetchCollected未置真会重复join、owncontext未覆盖等待、后继counter尚无handler、失败清理HTTP无timeout等fixture问题；开发owner/主代理补状态、own10scontext、真实handler及实际client.Timeout=10s且join后恢复，最终重跑。初次用例被默认SSRF端口限制在barrier前拒，按现有官方规则仅给本地fixture privateIP和exact临时端口许可并恢复，不放宽生产policy或断言。scope真实到期后已启动操作可完成、不强杀/不回滚，原tasktimeout不变；这些测试不是新生产grant、全H1或ASVS认证。
