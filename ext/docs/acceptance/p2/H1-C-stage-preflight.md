# H1-C request stage整批制品预验（2026-11-13）

**只接Plan.Run stages，不安装core.Executors/Models/PluginHooks，不等于完整pipeline/reply/report或目标准入。**

- 新增core.ExecutorGate值接口，模型/渠道未选择时不编造假ID；Gate仅允许operator明确审批的plugin key+actual source+configFingerprint完整tuple。builtin/unknown/malformed拒绝，取消优先。
- Plan.Run先解析原scope顺序，再对本stage所有将执行的实例preflight，成功后才开始任何hook/effect。检查实际Engine摘要与Loaded编译摘要/instance key一致；版本化envelope包含request_stage角色、Path、Stage及完整冻结Instance，原始Config（含私有关键词列表）只进入hash，不外泄。
- 任一host veto直接返回固定Outcome.Failed，绕过plugin optional错误跳过路径；它不是可选hook失败。nil Body原本不执行hook，仍走unchecked，不声称该类请求整请求已拒绝。late已运行实例跳过沿用原语义，预验失败不会写ingressRan。

## 实际行为

扩展既有ext/domestic/plugin_test.go，真实NewPlan/Run及公共Ingress、真实JS optional=true插件、两个实例、本地HTTPfetch计数：

1. 临时probe记录实际tuple，两实例允许fetch=2（正控），不同instance/config摘要不同。
2. 只批准first，second未批：整stage拒绝，first也没先联网，fetch=0，原Body无变更。sameplan再late调用仍拒绝fetch0，不能借ingressRan洗白。
3. 公共Ingress同样返回API error，原官方ShouldRetryRelayError确认不重试，optional不能吞host拒绝。
4. 全部tuple批准fetch2；Path改变或第二实例原Config改变均fetch0拒绝。
5. Body=nil保持unchecked、无fetch，不把没读的内容说成已审。

只读独立复核 `635d8919-a3bb-4da5-8dff-da905c04ced7`（未开发/未测试/无DB或生产收费调用）核对preflight先于ingressRan/JS/network；settle对Failed固定SkipRetry，不受optional；没有发现本stage限定范围阻断缺陷。未证明后续report不会联网，不得把“本stage所有hook为0”写成“整个失败请求所有网络0”。

## 验证

进程GOMAXPROCS2、GOSUMDB=sum.golang.google.cn、GOPROXY=https://goproxy.cn,direct。

`go test -p 2 ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/... -count=1`全部通过：requestplugin1.033s/jsplugin2.117s/plugins0.760s/domestic7.392s，其余ext通过。`go vet ./relay/requestplugin ./pkg/jsplugin ./plugins ./ext/...`及 `go build -p 2 ./...`均exit0。无数据库行为变更，新边界是冻结内存plan纯投影；既有SQLite hostfixture随ext同跑，未为本批纯tuple启动MySQL/PG，不声称额外三库验证。

最初并行写入半态unusedcore/sha256编译失败，源码稳定后完整重跑，没有放宽断言。

## 已达硬限制

累计**32/32官方文件、2463/2500增删行**，最后新增官方文件为relay/requestplugin/plan.go，均精确登记；门禁输出正常80%警告。余37行，且不能再新增官方文件。此批没有把多实例pipeline伪装成一个Executor。

未完：reply/report/taskreport独立路径、最终destination/opaque正文模型/redirect/fetch、provider chain/WS、后台credential与既有任务冻结延续、candidate generation/affinity支持、无请求规格Save/目录/type60关系。当前预算不能继续向新官方文件接线；需要负责人真实规则决策，不自动续增，不删除检验、压缩代码或限制协议冒充完成。
