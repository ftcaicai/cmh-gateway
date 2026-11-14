# H1-C 共享端点提前预选generation（2026-11-14）

状态：实现、独立复核与最终三库验证完成；不是完整H1验收。全局Gate/Store未装。

## 批准与规则先行

负责人明确选择“批准单文件例外（推荐）”。独立rules PR #2 (https://github.com/ftcaicai/cmh-gateway/pull/2)仅修改ext宪法/RFC与ops门禁，在实际initial CI43535688066后端4m36/前端6m59及差异门禁43535688230全success后普通merge3348e9be8。未用auto/admin绕检查；分支保护自身未认证。合入后完整UbuntuCI43536668813也success。主分支/develop同步规则后才改production。

门禁不是任意49文件预算：只有已登记且基线已有的middleware/task_plugin.go发生变化时允许49，其自身≤40行，累计6000行不变。真实Gitfixture验证任意49拒、指定49过/50拒、40过41拒、binary无法计数拒、无对应基线的untracked命名文件拒、空未跟踪文件仍计数、6000/6001及精确登记拒。独立review34ffa7f7指出untracked与binary计数漏洞，已修复；一次fixture git rm移除末目录导致文件创建失败，补mkdir后完整重跑exit0，未放宽断言。

## 精确实现

middleware/task_plugin.go相对rc50 +12/-2=14增删行：RouteTaskPluginEndpoint捕获的generation在parse与预选前存context，预选ChannelFilter携model.NewPluginCandidateView(localgeneration)，后面localfilter改kind仍同view；PinTaskPluginEndpoint优先selected.Generation，其次capturedcontext，最后currentRegistry，再保存选择。原Prepare仍用pinnedcandidate制品；不复制decoder或改变协议/候选优先级/权重/鉴权/计费。

## 验证设计

真实public Route→Prepare→Distribute并在abilities查询前用确定性callback替换同key/version源A→预编译B。GateA只批准A；必须观察**第一次Candidate准入证据就是A**。只看最终handler成功不足以证明修好，因为旧版可能预选误看B失败后fallbackPinA/DistributeA而最后仍成功。另用Go `-overlay`替换为HEAD未修复的同一官方文件、保持工作树不变，运行同SQLite firstcandidate子场景：实际exit1，首次CandidateSHA错误为B，directPin也选到B/markerB；修后最终三库exit0。overlay首次PowerShell路径解析和BOM输入错误已分别定位修正，红测结果不是这两种工具配置错误。

首次三库命令在SQLite负控暴露fixture未调用官方i18n.Init导致noAvailableChannel错误分支nilbundle panic（正控已过），不是调整错误码或放宽断言的理由；需补官方初始化后完整重跑，最终结果另绑定。最终修复仅调用官方i18n.Init()初始化fixture，`run-admission-matrix.ps1` / `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 1.89s/MySQL8.0.44 0.62s/PostgreSQL17.12 1.67s，package4.466s；所有引擎两A/B子场景及directPin断言通过。专属引擎优雅停止。

相关model8.602s/relay5.487s/middleware定向2.908s通过，相关vet及完整build exit0；扩展全套通过（domestic2.305s，其他ext全过）及ext vet exit0。全middleware历史Windows占用日志cleanup失败未因此消失，不宣称全仓全绿。累计门禁49/49文件、4077/6000增删行，命名文件14/40行。

测试终点是非真实relay的本地handler与reserve tripwire，不是资金生命周期守恒证明；不假称每个拒绝都在A decoder前，当前官方顺序仍可先PrepareA后Distribute拒。网络counter为本地stub，不访问付费上游。

## 独立复核

只读f430a274-7289-49e7-824c-f596b333d935未参与该实现，重读源及路由链未发现本14行限定范围阻断问题；强调firstcandidate断言而非finalsuccess。未把其只读复核当执行测试或全ASVS认证。

## 尚未结束的主线

本次预选收口不等于task-scoped旧source/config/targets grant、credential refresh冻结、affinity/type60/legacy、keycustody备份/GCM预算或完整startupactivation通过；资金同事务RFC独立未实施。完整H1仍未激活。后续任何第50文件或本命名文件超过40行必须另决策，不自行加额度。
