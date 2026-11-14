# H1-C 冻结任务凭据refresh身份护栏（2026-11-14）

状态：限定refresh身份护栏已实现并完成三库验证；不是task-scoped旧grant完成，不安装Gate或Store。

## 已确认的缺口

TaskAdaptor恢复冻结source/config/base/proxy，但taskCredentialLine之前调用普通FreshPluginCredentialLine；该函数可能进入RefreshPluginCredential，在service层重新查当前发布插件和当前配置。旧任务可能因此执行不同源/配置的refresh，再把当前结果供旧driver使用。仅冻结query/signer不解决它。

本批限定为漂移veto：已有service/provider_plugin.go及task/jsplugin/credential.go追加显式continuation期望。身份由原始sourceSHA与EngineSHA配对、绑定plugin、effectiveconfig exactbytes/base/proxy组成；不是version/generation编号授权，不是新operator grant。普通非continuation refresh合同不改。

在API入口即用当前DB身份预验，覆盖未到刷新margin的fresh快路径；现有refresh lock取得当前行后、hook前复核；运行原冻结artifact；短事务UpdateChannelLocked回写前再次对当前行/源复核，防网络期间配置改变而落盘。Registry发布不与DB同锁，因此是边界检查，不承诺检查后的所有并发发布都被原子冻结。singleflight隔离期望digest域，不能加入普通current刷新；跨节点仍复用原credentialrefresh锁/line CAS，不新增私有钱包或财务改动。取消只在启动前挡；共享refresh开始后仍按官方背景单飞合同，不承诺caller取消能中止已在网请求。

## 控制与边界

- 错配返回固定ErrContinuationUnavailable，不回原始配置/凭据；即使旧credential尚未过期但已到刷新margin，也不能因一般refresh失败fallback把身份错配吞掉。
- 原账号绑定仍需存在，不替换其他账号；source/config/base/proxy漂移在本次刷新0hook/0对应网络；网络期间漂移不能写新credential。
- 这是保守拒绝漂移，不是“新源替换后旧任务已经自动可完成”。task-scoped source/config/target grant仍未完，旧任务可能Unavailable跳本次；原timeout/CAS/refund不变。
- 没有改任何login/用户cookie/MFA/token验证；没有生产凭据、收费调用、档案key落地或新第50官方文件。

## OWASP参考（非整站合规声明）

已读[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)与[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)，核对server-side身份绑定、凭据替换与错误/日志保密。

参考[ASVS5.0.0 V6](https://raw.githubusercontent.com/OWASP/ASVS/v5.0.0/5.0/en/0x15-V6-Authentication.md)6.1.3/6.3.4关于多路径一致性作为检查方向：本批只覆盖冻结task refresh桥，不宣称整个gateway或第三方OAuth端到端满足该要求。无登录/恢复强度、TLS全矩阵、MFA/replay全面验收结论；剩余旧grant与全activation缺口保留。

## 最终验收

`run-admission-matrix.ps1`实际 `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 2.34s/MySQL8.0.44 1.18s/PostgreSQL17.12 1.89s，package9.363s。包括matchingA refresh持久化与同原usedline第二次调用复用新credential仅一次网络、Restore/Capture/BindContent/Attach实际桥接正控；source/config/base/proxy漂移在margin与fresh1小时快路径均拒0hook/network/Key不变；空account替换/expired/needslogin已存新行拒；refresh返回其他account或空→有account拒写；预取消0；网络期间settings漂移后正常返回以及throw两种路径均Unavailable不落新Key。普通非continuation仍保留官方暂时失败时未过期line回退，**冻结continuation任一refresh错误则Unavailable**，更保守而非静默复用。

独立只读eaa31c2d与b907dbb9指出初版freshfastpath、generic账号通配重用、DB证明失败、hook返回account变化及hook错误后普通回退缺口；主代理补前置DB预验、strictreuse、fixederror及冻结错误一律拒，并逐项新增负控。最终独立microreview adca91ab重读错误/账号修复确认所报缺口关闭，无本次限定范围新阻断；只读review未执行测试，不冒充全ASVS或Registry/DB原子冻结认证。

一次测试编译使用不存在Hook字段失败，修为实际PluginHookIdentity.Export后重跑全部通过，未放宽断言。最终 `go test -p2 ./service ./relay/channel/task/jsplugin ./ext/... -count=1`通过（service25.128s/task11.968s/domestic4.192s，其余ext通过），相同包vet及完整build均exit0；专属三库已优雅退出，无生产凭据/收费模型。累计49文件/4217行、命名sharedendpoint仍14/40行。
