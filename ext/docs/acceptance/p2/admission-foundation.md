# H1 基础合同检查点（2026-11-13）

状态：**基础值DTO/纯策略，不是live入口验收。**
负责人已批准专项累计32官方文件/2500增删行；规则v0.8已单独提交。当前仍只改既有9官方文件，准入接线尚未开始，不用扩大预算的成功当安全覆盖完成。

## 本轮新增

- core的ModelGate只依赖context和值DTO：区分public、channel_save、candidate、attempt、dispatch。
- ChannelSave允许ID0的新渠道预验；已选/执行/可见候选必须Known且ID>0，不伪造ID。
- domestic Gate为不可变集合：公开名、最终上游与非空声明名分别精确授权；类型单独允许。
- builtin执行器只接受空Key/摘要/配置，不用插件伪装builtin；plugin必须Kind/Key/64位小写SHA256源摘要/配置指纹完整且operator审批集合精确一致。Generation只是快照标记，不把它当信任。
- 构造不安装core.Models，默认空实现保持官方行为；可见性不能拿public-only检查替代候选检查。禁用渠道始终可做清理，启用批量必须全部预验后才能由host写入。

## 边界

该纯Gate不解析上游映射、不选择实际plugin、不证明最终网络目的地、不启动数据库事务。所有来源是host必须正确投影的事实，不能用该单测宣称已阻止绕过。插件哈希/配置归属仍需在宿主发布/执行流程接线。

测试里的“整批无写入”仅证明调用方遵守先全部预验再写的纯契约，**不是数据库并发原子性测试**。实际渠道写入接线将要求SQLite/MySQL/PostgreSQL真库验证。

## 验证与仍未验

原策略parser、43项插件golden/宿主patch、默认空实现与值准入表全部集中于已有domestic测试，未散落多层测试文件。第一轮因assert.Same要求指针而失败（core默认是值实现），改为值相等+可观察noop行为后重新运行；不改生产逻辑迎合断言。

- 最终父代理 `go test -p 2 ./plugins ./ext/... -count=1` exit0（plugins0.759s、brand0.096s、console0.151s、domestic0.106s），随后 `go vet ./plugins ./ext/...` exit0；bash语法检查和差异门禁exit0（9/32文件、110/2500行）。
- 本轮当前源码HTTP2定向测试 `go test -p 2 ./relay/channel -run '^TestUpstreamGetBody_HTTP2CannotRetryWithoutGetBody$' -count=1` exit0（0.139s）；纯rc50单次也exit0。两次单次通过不抹去全量回归中的失败，不给出已修复结论。
- relaykit/tokenkit各GOWORK=off独立build exit0。
- PATH与服务未发现Docker/Podman/MySQL/PostgreSQL本地引擎；没有借生产DB验收。后续准备独立测试环境，缺三库时不报DB接线完成。
