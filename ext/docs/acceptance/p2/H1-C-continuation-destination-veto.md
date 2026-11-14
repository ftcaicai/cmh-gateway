# H1-C 已存档任务目的地策略拒绝分类（2026-11-14）

状态：限定分类修复已实施并通过真实三库验证，不增加任何目的地许可，不启用Gate/Store。

## 缺口

有限scope首批已经把目标Policy veto转为Unavailable，但没有独立grant的旧archived任务仍返回普通request rejected。该任务同样拥有冻结source/runtime，operator撤回目标关系时应当禁止本次网络，而不把host政策决定当作上游任务失败，累加PollFailures并触发普通失败阈值。

本批精确2增删行：TaskAdaptor.checkQueryDestination仅在真实core.Destinations.CheckDestination拒绝分支，用a.continuation!=nil代替a.continuationScope!=nil判断。任何archived runtime目标Policy veto返回ContinuationUnavailable；非冻结普通调用仍相同generic request rejected。source/ref/scope不因此获得目标授权。共用helper覆盖实际最终签名URL/Host/method及redirect每跳；没有把非法URL/SSRF/TLS/网络错误泛化成跳过，原错误合同保持。

service已有Unavailable THIS-pass跳过复用，不变timeout/CAS/refund、不延长deadline，也不是无限挂起不退款保证；只能说本次政策拒绝不会误计普通失败。

## 最终真实回归

在现有host三库集中fixture使用真实档案认证/绑定/resolver/Dispatch，特意nil ContinuationGrants确保不是scope分支：初始目标拒绝0query；初始query获批但303redirect新目标未批，只有1initial/0deniedtarget。两者PollFailures保持2/状态InProgress/Quota/FinishTime/failreason/data/完整privatefunding不变。普通非冻结control明确返回request rejected且不是Unavailable、0query；其fixture先InitChannelCache以免更早的故意陈旧cache导致wrong signer config抢先失败，不能拿其它错误冒充Policy控制。

首次matrix实际exit1只因普通redirectcontrol期望1query但更早signer陈旧config失败0query；逐项定位后普通control改为明确初始denyTargets并EqualError request rejected、刷新测试cache，冻结redirect严格1/0未放宽。最终 `run-admission-matrix.ps1` / `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 4.39s/MySQL8.0.44 3.84s/PG17.12 3.71s，package12.215s。专属引擎已优雅停止。

相关taskadaptor1.327s/service14.336s/relay0.394s完整tests及vet/fullbuild exit0；ext全套domestic5.596s及其余ext通过、ext vet exit0。独立只读25025774未参与实现、未执行测试，核对真实veto边界、redirectPolicyErr sentinel传播到service single/batch accounting前，未发现本限定2行修复阻断问题；不称ASVS或全H1认证。累计49/4365行保持，未用新官方文件。

没有第50官方文件、不改签名query严格匹配或重定向凭据剥离，不安装真实operator任务许可、不访问生产/旧系统/付费模型。金融同事务、其余协议旧任务grant与全局启用仍未完成。
