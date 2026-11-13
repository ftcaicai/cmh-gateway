# H1-B 长连接事件与握手检查点（2026-11-13）

**本批已接线，ModelGate仍未全局安装；不代表完整H1、插件最终目的地或资金守恒完成。**

## 实际实现

- Responses每个response.create在可能联网的SocketIngress之前检查公开名、实际选中身份。激活时先选实际候选并保留同一个RetryParam/候选进入原循环，不二次随机选路；未激活保持原顺序。
- 锁定连接每次复用读取新鲜DB行，检查状态/类型/模型/分组/Key/握手配置，恢复冻结context映射，不把新DB映射刷新到旧物理socket。复用时仍调用统一CheckRequestAdmission比较fresh与冻结目标。
- 每次首次/重试在预扣或追加预留之前检查；请求内准入403为event-local错误，允许后续事件重试；认证runner自身401/403仍保留原关闭逻辑。
- Realtime固定握手逻辑身份必须与当前渠道映射一致；OpenaiRealtimeHandler在plugin构造之前，以及每个client event进入plugin前、patch后计量/转发前复核。
- WssHelper初始化实际ChannelMeta后、DoRequest之前调用同一Realtime检查。当前映射别名若实际握手仍是public model，则**在任何dial之前拒绝**，不通过静默改URL或未批准模型映射掩盖。
- 原始JSON中的session/response.model不能选择不同固定主模型；legacy input_audio_transcription.model与GA audio.input.transcription.model是独立模型，使用单独的公开/上游target审批，不强迫等于主模型。未批准转写目标拒绝。

## 独立复核

复核者 `71c90d8f-9f26-4be6-8aa5-89c2c2464041` 未参与开发、未运行测试，指出并提交前修复：
1. Realtime检查只在handler，实际WssHelper先dial再拒绝映射目标不一致。已前移握手检查并真实计数upstream握手。
2. Responses复用restore调用GetSetting可能修复坏DB JSON，拒绝路径竟有写入。激活分支已只读decode并复用值；测试坏Setting原文保持。
3. 只检查直接model漏过独立转写模型字段。已独立授权两种协议形状，并测试普通instructions不误拒。

复核还提醒report hook可在拒绝后联网；本批没有证称全部插件网络均为0。当前未知plugin执行者本来failclosed，完整制品/配置/报告能力约束仍属后续阶段。第二位只读复核 `3ca2bad5-a952-4b6d-b3b2-65f832f64562` 核对最终限定修复，不等同重复测试。

## 真socket与真三库

集中扩展同一 `ext/domestic/host_admission_test.go`，本地真实Gorilla client↔gateway↔upstream连接，不用只看URL/helper返回值：

1. 第一次create允许，收到response.completed，上游create计数1。
2. DB Setting改成坏JSON，下一create错误，上游计数仍1，DB坏字符串完全未修。
3. 使用同时允许两个上游目标的策略，把DB映射改到另一个允许模型；旧socket下一create仍拒绝，上游计数1，不能在同一socket重指目标。
4. 恢复mapping后禁用channel，下一create拒绝，上游计数1。
5. 恢复channel，再create成功，计数2；证明拒绝event没有毒化整个可复用会话。
6. Realtime helper对允许主身份/普通instructions允许，对主模型篡改与两种未批准转写目标拒绝。
7. 实际WssHelper在映射别名握手不一致时返回错误，upstream握手计数不变，证明不是dial后拒绝。

fixture显式设置本地模型价格0、免费预扣开关关闭、token估计关闭并恢复，不构造假的钱包余额。此测试验证传输/门禁，而不是扣费路径；收费/退款守恒仍未验。fixture runner只提供本地选择context，不宣称真实认证全链路。Realtime尚未完整驱动一次允许的全双工client-event回路，本批实际网络证明主要是Responses复用与Realtime拒绝前dial计数。

```
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1
# 内部加载独立本地测试DSN后执行
go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v
```

最终含复核修复run exit0：SQLite3.50.4（1.68s）、MySQL8.0.44（0.41s）、PostgreSQL17.12（1.19s），包9.023s。所有既有候选/目录/价格/写入/预扣前回归也同跑；Setting singular和OtherSettings plural坏JSON均保持原值且不向上游转发。

## 失败与纠正保留

- 最初允许create被panic包装成500：缺省EnableFreeModelPreConsume=true，使零价也进入缺钱包fixture，i18n未init导致panic。仅为定位临时打印stack，最终已移除；fixture显式选择免费skip并关闭无关tokenizer，不改生产计费。
- 上游stub每次返回相同response ID，真实gateway把重复terminal当上次响应丢弃导致超时；改为确定性唯一ID后会话重用通过，不削弱去重。
- 发现普通setup会存在nil ProviderPlugin context key；Realtime只把非nil retained artifact当执行者未知，避免所有builtin误拒。
- 子代理Realtime `-run Realtime`显示no tests to run，只算编译，不算事件行为通过；实际行为以上述集中fixture为准。

## 最终相关回归与构建

`GOMAXPROCS=2 go test -p 2 ./model ./relay/helper ./relay ./relay/channel/openai ./plugins ./ext/... -count=1` exit0：model6.508s、helper2.799s、relay5.690s、openai4.432s、plugins0.768s、domestic1.956s，其余ext通过；随后 `go vet ./model ./middleware ./relay/... ./plugins ./ext/... ./controller`及 `go build -p 2 ./...`均exit0。最终门禁25/32官方文件、1946/2500增删行，git diff --check通过。

## 未完成与预算

未知任务/provider/request-plugin制品与有效配置指纹、最终URL/opaque正文模型目标、已有任务冻结能力和完整失败重试仍须接线；当前不激活Gate。Responses锁定mapping只是当前逻辑模型一致性，不证明任意header/config插件目的地不可变。未改钱包H3/H7同事务决策。

累计25/32官方文件、1946/2500增删行（约78%），下一批接近80%必须先复核范围，不压缩代码/跳测试满足预算。没有新release可演练时不伪造升级记录。所有独立DB已优雅关闭，便携引擎/数据/随机凭据留ignored cache，不提交。
