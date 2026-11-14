# Native Responses WS 最终模型身份

2026-11-14，H1既有登记范围；无资金/H4/H6实施，无生产调用。

## 修复

最终create event在PrepareResponsesRequest的转换、参数覆盖、finalizer和envelope assembly后，active ModelGate要求顶层解码后的model恰好一个，且非空字符串精确等于本次权威UpstreamModelName。遍历原始顶层成员而不是map，普通重复和Unicode转义同名均拒绝。固定403 AccessDenied/SkipRetry，不重写正文替代拒绝。首次与复用共用builder，分别在dial/write前拒绝；noop保持原override语义。仅原生Responses WS已知schema，不推断opaque vendor body、不修改HTTP/task或资金生命周期。

## 实际验证

中央host fixture实际wire捕获public-chat→approved-target，首次/复用均正确。错误、缺失、数字model的override各自拥有独立精确executor审批，仍403且0额外握手/create；条件override首次成功后第二次403，总计1握手/1create。noop实际发送unapproved-target。真实passthrough唯一转义键成功、普通和转义重复键拒绝且0额外发送。原目的地/渠道双握手撤销屏障保留。

- 实现者最终SQLite focused13.901s、相关WS5.751s、vet/diffcheck exit0，所有child jobs已收。
- 最终真实三库：SQLite3.50.4 8.76s、MySQL8.0.44 6.42s、PostgreSQL17.12 7.73s，package23.205s PASS。命令`powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1`，内部`go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`。专属loopback合成DB，已停止并收集启动jobs exit0。
- 集成`go test -p 2 ./relay ./relay/channel ./ext/... -count=1`全部PASS（relay0.438/channel0.188/domestic8.836s）；相关vet及完整`go build -p 2 ./...` exit0。
- 独立只读生产及host最终diff复核无阻断，核对实际moego alpha3 ForEach逐成员解码键；复核未运行测试。正常路径显式关闭/join；子测试fatal仅defer关闭peer的清理增强仍属非阻断建议。
- 官方累计49文件4564增删行，余1436；命名shared endpoint14/40，无新增官方文件。

## 失败与限制

初期fixture保留disabled渠道和handshake barrier导致错误/挂起，已仅修fixture状态并完整重跑；wire等待改为有界。红overlay第一次参数解析失败，修正后因子场景过滤跳过必要session、done信号溢出触发90s超时，不能算有效红测断言证明；未隐藏或计为通过。最终正常完整测试均通过。

不证明完整payload语义、任意vendor schema、全协议兼容、无TOCTOU、race、最低数据库版本、财务守恒或生产可用。缺省noop和生产archive/grants禁用保持。
