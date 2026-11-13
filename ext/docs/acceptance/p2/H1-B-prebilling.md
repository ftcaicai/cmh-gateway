# H1-B 公开名、选中渠道与预扣前边界（2026-11-13）

**本批已接线并专项验收，core.Models仍保持默认空实现；不是完整H1或资金守恒完成。**

## 实现范围

- Distribute成功解析模型后，在选路/pin新执行分支前检查公开模型；普通既有任务读取不套新发布门禁，保留原token/ownership政策。
- SetupContextForSelectedChannel在门禁激活时从主库读取新鲜行，检查启用、已解析pin、模型/选中group资格；先只读解析坏settings，不触发getter修库。实际plugin/provider/pinned执行者不确定时不能假称builtin。在Key取得/上下文凭据发布之前检查，真正的Setup错误现在传播，不再忽略。
- relay.CheckRequestAdmission为纯检查：严格匹配本次context ID/type/original，查询当前DB身份，不取缓存；fresh映射必须与**实际context映射最终目标**相同，不能用修复后的DB授权仍会发送旧目标的context。
- PrepareRequestBilling检查先于token metadata/estimate、价格、免费跳过与PreConsume。初始RelayInfo尚无ChannelMeta时仅建立浅的检查快照，不初始化或修改真实请求/计费状态。
- controller重试循环在本次选路后、PrepareTieredBillingForSelectedGroup之前重新检查选择快照，避免沿用上次ChannelMeta。保留原有失败退款/违规费用处理，未改金融生命周期。

## 独立复核发现与修复

复核者 `803412be-c8b6-45fd-9110-2579c8cd220d` 仅只读，未参与实现/未重复测试。
1. P1实际context映射与freshDB目标不同仍能通过。已补context最终映射一致检查；不改Request DTO或映射状态来掩盖。
2. P2既有task读取是否被拦取决于token是否开启模型限制（只有开启才解析旧模型名）。已将public gate限定新选择/pin执行，集中测试两种token模式都能到原读handler。
3. P2新鲜channel已不再Models列出该请求、或Group移除，但旧cache选择仍可用。已用原路由exact/normalized匹配语义在host投影检查model，Setup/prebilling核对selected group；集中测试model/group撤销都在Key发布前拒绝。

第二位只读复核 `358ad9a7-eeba-4966-b9f9-c49dfc2373f2`核对这些限定修复；其结论以任务消息为准，不把代码审查等同运行证据。

## 观察到的行为与真三库

相同集中测试文件 `ext/domestic/host_admission_test.go`复用此前候选/目录/写入/价格回归，新增请求边界：
- 允许Setup成功发布其实际测试Key；坏高优先候选Prepare返回403。
- 拒绝请求的计费probe Reserve/Settle/Refund全部为0，estimatePromptTokens/FinalPreConsumedQuota为0，Request.Model未变；初始nilMeta路径拒绝且真实info.ChannelMeta仍nil。
- DB repaired allowed与context stale denied映射目标冲突拒绝。
- 当前channel移除Model/Group或禁用时，旧selected snapshot不能通过Setup或CheckRequestAdmission；拒绝Setup没有发布channel Key。
- 未上架public模型在Distribute停止，下游handler计数为0。
- 既有task旧模型撤销后的GET不属于新提交，token模型限制开/关（且原token允许旧模型）两者均到204原读handler。
- default noop `CheckRequestAdmission(nil,nil)`不增加上下文/DB依赖。

```
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1
# 子进程本地测试DSN + GOSUMDB/GOPROXY
go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v
```

最终包含允许Setup的三库完整run exit0：SQLite3.50.4（2.56s）、MySQL8.0.44（0.46s）、PostgreSQL17.12（1.37s）；包10.302s。专属loopback引擎已优雅退出，数据/凭据均仅ignored cache，无生产/旧系统/收费上游。

这证明**被拒路径不进入预扣/后续handler**，不是实际钱包扣款/账本守恒证明，也不是全HTTP请求从认证到真实上游的端到端付费验收。计费probe用于观察接口是否被调用；DB路径是真三库。重试检查位置已审查，当前新增测试没有模拟一次完整真实失败后自动重试链，不能把代码位置审查称retry E2E。

## 回归/构建限制

相关model/helper/relay/plugins/ext全测试全部exit0：model20.238s、helper13.319s、relay5.762s、plugins6.260s、domestic6.775s；middleware实际路由/task/plugin/模型限制等筛选测试exit0（5.814s）。随后 `go vet ./model ./middleware ./relay/... ./plugins ./ext/... ./controller` 与 `go build -p 2 ./...`均exit0。门禁22/32官方文件、1656/2500行，git diff --check通过。controller子代理 `-run '^$'`只编译，未算行为测试。工具链高并发曾internal compiler0xc0000005，使用进程GOMAXPROCS=2和 `-p 2`重试成功，不更改go.mod/全局环境，不称race通过。

未完成：Responses WS每create与Realtime每事件、任务/Provider plugin实际不可变制品+配置指纹、最终映射/目的地一致性与所有opaque覆写。主库检查与最终网络发送之间仍需完整freeze/transport边界，不能凭本批freshDB说全TOCTOU闭合。Gate未激活，H3/H7同事务方案仍单独设计。

父级真实计费规则/ASVS全面认证与最低数据库版本/并发事务/账本守恒没有新增通过结论；全仓Windows历史测试失败仍保留。
