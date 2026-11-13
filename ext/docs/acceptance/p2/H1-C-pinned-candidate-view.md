# H1-C Cycle-free pinned候选typed view（2026-11-14）

本批实际修复请求pin旧源A、Registry变B后candidate误用B的type61兼容，不以pin代替operator批准，不开启全局Gate。

## 结构

- dto/channel_constraints.go仅依赖ext/core，加入key-free ChannelCandidateSnapshot与typed PluginCandidateView{Admission,Matches,SameGeneration}，ChannelFilter.PluginView不序列化JSON；没有dtoimportjsplugin，也没有any/反射或按generation编号/名称反查源。
- model私有wrapper保留真正RoutingGeneration指针，Admission复用原ChannelPluginAdmission exactsource/config/role/path，Matches复用hostfilterhelper对capturedprovider route/ResponsesWS判断。BaseURLPresent保留nil/空pointer的默认URL差别，其他原始control字符串完整，不传Key。
- 任何显式view缺插件不得fallbackcurrent；所有非nilview需samegeneration双向一致，混合generation拒type61；nilkindfilter继承共同view，cache路径在按kind分组前传播，避免policy用A而hostfilter又看B。无viewlegacy调用维持current，nofilterpublication仍current。
- middleware在Distribute过滤器准备后/选择前绑定view；directSetup亦绑定，已捕获generation跨重试保留。Provider实际Setup使用同capturedgeneration，不重新选currentsource。

## 已验

既有ext/domestic/host_admission_test.go：actualplugin A生成taskruntime批准，保留generation后currentRegistry替B。无view当前B拒；viewA可候选；mixedA/B和capturedemptygeneration拒；真实GetRandomSatisfiedChannel DB/cache两路径都选择A批准渠道；实际SetupContextForSelectedChannel保留原generation。另provider A有chat路由、B改embedding，capturedview仍按A接受旧chat，defaultcurrent拒。

最终 `ops/.cache/db-test/run-admission-matrix.ps1`实际 `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 1.99s/MySQL8.0.44 0.66s/PostgreSQL17.12 1.36s，包7.543s。真实三库包含原集中runtime/存档回归；专属引擎已优雅停止，无旧/生产/付费模型。两次并行源码半态分别缺hosthelper编译失败，稳定后重跑，未弱化断言。

相关model/relay/ext全套通过（model10.450s/relay5.453s/domestic2.584s，其余ext通过）；middleware候选/插件/模型相关定向8.713s通过；dto/model/middleware/relay/ext vet与完整build exit0。新增provider-onlyfixture随后最终真三库通过，不伪称全middleware历史cleanup失败消失。

独立只读复核cc90989a-f61c-46ce-9694-dd14a779bf48未参与开发/未执行测试，重读最终snapshot/控制/hash复用和typedcoherency后未发现本限定范围阻断问题。其早期directSetup观察已补真实接线/断言并重读确认。

## 边界与预算

**Shared endpoint在Distribute之前的预选入口尚未单独绑定/覆盖**，不能称所有候选入口收口；本批不解决affinity有效参数覆写、type60/legacy、task-scoped continuationgrant或credential refresh旧配置。operator批准SHA/config仍需显式match，source/target检查不跳过，opaque正文模型和完整activation未认证。

累计**48/48官方文件、4063/6000增删行**，用完最后dto文件slot；剩1937行并不意味着可新增第49文件。任何后续新官方文件必须先得到范围决策，优先已登记范围/扩展区，不删除有意义测试或压缩校验绕预算。H3/H7金融同事务独立未批准，全局Gate/Store保持默认未安装。
