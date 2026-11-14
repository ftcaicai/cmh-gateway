# H1-C 冻结任务source veto与普通hook故障区分（2026-11-14）

状态：限定source veto分类已实现并完成三库验证。只修复拒绝分类，不放行任何旧源/目标、不安装Gate或Store。

## 缺口与最小修改

Engine.call原先把source PluginHooks veto转成普通errors.New("plugin execution rejected")。冻结任务已能在缺档/绑定错误时跳本次poll，但这个源策略拒绝被误作普通插件失败，可能增加PollFailures，最终误走普通阈值终态/退款。

ext/core增加同安全文案的typed ErrPluginAdmissionDenied；Engine仅在真实host Gate返回拒绝时返回该sentinel，JS抛相同字符串不能伪造errors.Is。TaskAdaptor仅在冻结runtime遇该typed错误，将requestDescriptor与单/批parse的错误映射ErrContinuationUnavailable；真实JS错误/网络失败仍原poll失败规则。普通未冻结请求仍原错误文案与错误路径，无wholemethod financialhandled返回。

本次复用既有service DispatchPlatformUpdate/updateSingle/pollBatch对Unavailable的THIS-pass跳过，不改timeout/CAS/refund规则，不承诺无限挂起或不会未来超时退款。source gate仍真的拒绝执行，绝不是旧task grant绕过。

## 任务级授权仍待明确设计

只读66faf08c直接核实：archive.Save仅保存source/runtime字节，发生在buildSubmit之前，因此未来query/content实际签名目的地尚不知；Load目的枚举不是operator权限授予，Restore+Bind与非secret ref不能当授权bearer。真正旧源独立grant必须明确operator授予、精确task/user/channel/ref/purpose/source/runtime/finitehook、撤销/期限与目标关系，不能自动从档案存在推出source全export或任意futureURL准入。

本批不为绕过该设计问题再增加unusedparser，也不贸然mint权限。后续若选择parse-only独立grant，应保持fetch/target globalchecks、拒newsubmit/其他task/purpose，最终完整poll网络仍另验，不能假称全旧任务可完成。

## 最终验收

真实冻结TaskAdaptor经service DispatchPlatformUpdate：single build/query、sign拒绝0对应query网络；parse与extractUsageOnComplete拒绝允许已发生1次query，但PollFailures保持2/status InProgress/Quota13/FinishTime/failreason/data/完整privateFunding/provenance不变。batch build/sign/parse/usage四分支同样验证，不只推断。真实JS抛**完全相同文案**仍ordinary failure2→3，避免字符串比较伪造Unavailable。reserve/settle/refund tripwire0仅是本次跳过，不是整个资金生命周期守恒证明。

`run-admission-matrix.ps1`实际 `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`最终exit0：SQLite3.50.4 3.41s/MySQL8.0.44 1.45s/PostgreSQL17.12 1.84s，package12.596s；专属引擎均优雅停止。相关pkg/jsplugin2.520s/taskadaptor1.418s/service20.403s全套通过，相关vet/完整build exit0；ext全套通过domestic2.942s及其余ext，ext vet exit0。

独立只读069450b8未参与实现，指出optional completionusage错误本来忽略会发布不完整终态，已补仅typedveto映射并重读闭环；无本限定范围阻断。其review未执行测试，specialized security route不可用时只作手工独立复核，不宣称全ASVS/长期hold或完整H1认证。命名sharedendpoint仍14/40行；累计49/4240，未增第50文件。
