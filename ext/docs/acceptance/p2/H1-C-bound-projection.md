# H1-C type61固定制品与选择配置投影（2026-11-13）

**完成限定切片：明确绑定type61、指定role/path的候选与已选identity投影。未安装ModelGate/PluginHooks；不代表完整插件执行、目的地、报告或资金守恒验收。**

## 实现

- Engine.SourceSHA256暴露实际编译代码身份。PluginExecutorIdentity要求LoadedPlugin的编译摘要与实际Engine摘要相同，拒绝wrapper.Engine被替换的身份错配。
- model.PluginExecutorIdentity值DTO只输出key/sourceSHA/configFingerprint/generation，原始配置不出桥接。版本化envelope绑定role、pathScope、BaseURL、channelOther、type、完整binding/OtherSettings、真实ChannelConfigFor有效值及mapping/param/header controls。provider AdvancedCustom由该源+effectiveconfig派生，不拿存储的旧派生值充当不同证据。JSON直接只读decode，null/坏config不能走silent {} fallback。
- model.ChannelPluginAdmission必须绑定本type61 key，exact指针仍属于对应固定generation；provider匹配RelayRoute(path,public)，task Declared使用同generation、本plugin自己的声明。无aggregate aliascache/GetByModel元数据owner代替执行者。
- candidate只有明确请求role/path约束时能投影type61；无请求上下文的Save/目录仍unknown，type60歧义/legacy/requestplugin chain未开放。
- Setup在实际pin/provider reconciliation后、发布Key前检查exact执行者；协议约束使用原host filter，不再用当前registry第二次替换已选pin的政策身份。
- CheckRequestAdmission读取freshDB，随后从冻结context重建实际选中Setting/OtherSetting/BaseURL/channelOther/mapping/param/header投影，与fresh模型和executor精确比较。两个配置都各自批准也不能用新DB配置授权旧hookcontext。实际task applyChannelContext后续仍读cache，此处不是“所有task运行配置已固定”的证明。
- 已留存的request_plugin_plan可能比当前setting更老；Setup与预扣前均保守拒绝非nil计划，直到完整chain有权威证据。当前global/bound请求实例也保守unknown。

## 测试

扩展既有plugin_test.go和host_admission_test.go，无新增散布测试文件：

1. 纯投影：允许source+effectiveconfig+role/path；改config/base/mapping摘要变化且失配拒绝；坏/nullconfig拒绝，错误绑定拒绝，替换Engine拒绝。
2. 实际Setup→CheckRequestAdmission允许；同key/version发布新源后，已选old pinned仍按旧源检查，不跟新registry变更。
3. DB改config拒绝；即使两个config tuple都批准，也因selected冻结值不同而拒绝。
4. 非nil retained plan在当前实例为空时仍拒绝；Setup未发布selected Key，预扣前检查也拒绝。使用sentinel验证存在性边界，不宣称真实整个requestplan阶段E2E。
5. runtime param override变化保守拒绝并恢复原context；不通过删除control检查来伪造支持。
6. candidate当前generation与alreadyselected pinned是两个合同：替换当前registry后，新候选因新源未批准拒绝，但已选旧pin投影仍允许。

## 独立审查与未闭合兼容项

复核者 `5d339c9b-82c0-4f6d-bab7-67d1ad69eb35`（未参与开发，只读、无测试/生产/付费调用）指出：

- retained plan不能被当前空settings洗白，当前代码已含拒绝，追加实际三库断言验证（包括Key未发布）。
- **旧generation在选路之前已被pin、此时registry切换**：candidate接口不带pinnedgeneration，当前按新registry保守过滤，因此可能错误拒绝已批准旧制品。当前不宣称跨切换请求保持可用；需要后续明确传递选择约束的固定generation，而非放宽全部候选。纯测试保留此差别，不把它记成已修。
- **affinity template改变ParamOverride**：runtime envelope与DB envelope不同，当前拒绝，即使operator审批两个tuple也不等价。未支持该动态覆盖配置的完整关系，后续应生成同一实际执行envelope，不删control比较。当前测试验证类似runtimecontext覆盖拒绝，不冒称真实affinity规则全E2E。
- 编译元数据假定可信Go代码不修改Meta.Relay/AllowedHosts；源摘要不是任意trusted Go mutation的防护。未观察实际生产变更，未据此声称漏洞已验证。

未完成：完整有序requestplugin chain、可选失败不可绕过的host veto、task/provider网络hook前+descriptor模型变化后、最终URL/body模型/redirect/fetch、旧任务冻结延续能力、type61发布/保存目录及不透明模型关系。故全局策略仍不安装。无法以此“长期所有插件unknown”交付完整产品。

## 实际验证

最终三库：`powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1`（加载ignored专属本地DSN，内部go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v），exit0。

- SQLite3.50.4：2.27s；MySQL8.0.44：0.51s；PostgreSQL17.12：1.40s；整个包10.013s。
- 同次执行之前全部渠道事务/目录/价格/请求前门禁/真实Responses复用回归；error日志中的closedDB是故意失败关闭断言，不是漏查测试错误。
- 最终 `go test -p 2 ./model ./relay/helper ./relay ./pkg/jsplugin ./plugins ./ext/... -count=1` exit0：model8.208s、helper2.837s、relay0.464s、jsplugin2.306s、plugins0.805s、domestic2.016s；随后相关 `go vet ./model ./middleware ./relay/... ./pkg/jsplugin ./plugins ./ext/... ./controller`、`go build -p 2 ./...`均exit0。全仓Windows历史失败不据此抹除。
- 专属MySQL/PG已优雅关闭，未连接旧/生产DSN，未读客户Key，无收费上游请求。

开发中一次Go1.26.10编译器内部aeshashbody/ssagen崩溃，即使GOMAXPROCS2；采用临时GOMAXPROCS1、-p1、-gcflags=all=-c=1串行编译后有真实测试结果，未修改全局Go配置。一次并行规范化编辑中vet捕获map[string]RawMessage→map[string]any的半写态类型不一致；稳定源码重新跑通过。最初测试先于API完成有undefined编译失败，保留事实，不当作业务回归。首次candidate测试只有RequestPath、不含host的provider身份filter，修正fixture与实际constraint一致后通过，未放宽生产凭路径猜执行者。

## 范围与接续

累计27/32官方文件、2211/2500增删行，逐文件登记与门禁通过并输出80%警告。仅余5新文件/289行；后续不能凭估计宣称足够完整H1，达到硬限前先测实际diff。资金同事务RFC仍独立，不在本次H1批准内。

T1.6c同时补齐全仓三个quota符号的成功grep清单：24匹配行，3定义、1注释、5测试、15生产静态调用；TryReserveUserQuota和授信/批处理/原始迁移SQL等另外列出。此为词法清单，不是资金唯一出口或守恒证明，见 `../p1/quota-write-inventory.md`。
