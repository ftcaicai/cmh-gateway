# H1-B Realtime 规范身份字段验收

日期：2026-11-14。范围仅现有 Realtime 准入及中央回归；不启用生产策略，不扩大 type61 支持，不修改 usage、资金或 H4/H6。

## 缺口与修复

原 guard 将事件解码到大小写不敏感的 Go struct，但转发原始字节。规范 `type:session.update` 可被后续 `TYPE:response.create` 遮蔽，规范 `session.model` 因而未被检查；`SESSION`、transcription 的 `MODEL` 也存在相同差异。此为源码可达的审批视图差异，不是供应商 live 接受证明。

现在按解码后大小写敏感的规范 key 提取身份；仅在 type/session/response/model 及两条 transcription 身份路径拒绝重复成员，包括 Unicode 转义同名键。不 blanket 拒绝未知字段或无关重复键；可选容器、transcription null/空 model 与 noop 保持既有合同。固定显式 model 仍须精确等于权威 upstream。plugin 前与 patch 后的原有复核保留，不重写事件。

## 验证

- 修复前完整 Host fixture 红测：exit 1，15.563s，五处真实 Expected-not-nil 断言失败；不是过滤缺依赖或超时红测。
- 最终中央回归含 17 个拒绝、6 个兼容正例：TYPE/SESSION/MODEL 遮蔽、普通/转义重复身份路径、唯一转义规范 key、可选 null、未知 alias/无关重复字段、noop。
- 最终 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` 真实三库：SQLite 3.50.4 9.22s；MySQL 8.0.44 7.76s；PostgreSQL 17.12 7.32s；包 24.621s，exit 0。使用既有隔离 loopback fixture，所有引擎正常停止并收齐 jobs。
- `go test -p 2 ./relay ./relay/channel/openai ./ext/... -count=1`：relay 5.934s、openai 5.366s、domestic 8.821s，全部通过；相关 vet 与 `go build -p 2 ./...` exit 0。
- 独立两次人工只读复核无阻断；未运行专用安全子工具，不宣称完整 ASVS 认证。owner 的 `-run Realtime` 显示 no tests to run，不算额外覆盖。
- 官方 diff gate：rc51 基线，49/49 文件、4591/6000 增删行，余 1409 行；named exception 14/40，80% scope review继续有效。

第63轮仅新增实际 codec 的中央 guard 回归；第64轮补充如下 raw socket 证明。两轮均不证明供应商解析行为、任意正文安全、完整 TOCTOU 消除或资金守恒。资金、H4/H6 实施拒绝保持有效。

## 第64轮：真实 handler 原始帧

仅扩展中央测试，未修改生产代码。loopback WebSocket gateway 调用真实 `OpenaiRealtimeHandler`，连接本地 upstream stub：两条 TYPE/SESSION 遮蔽原始帧均收到 `error/request_rejected`，上游计数保持 0；随后同一连接的合法原始帧逐字节回显，上游计数恰好 1。不是只调用 guard 的模拟断言，也不是供应商 live 测试。

局部复制 dialer，两次握手 timeout 5 秒；stub、gateway 双端及客户端读写 deadline 5 秒。客户端关闭后分别有界等待 gateway/upstream handler 6 秒，再关闭 server；生产 handler 内部 reader 没有导出的 join，本测试不宣称 race/所有内部 goroutine 认证。

最终 owner 完整 SQLite Host 回归 13.531s、vet/diff-check exit 0；独立人工只读最终复核无阻断。父会话 `go test -p 2 ./ext/... -count=1` 全通过（domestic 7.975s），相关 vet 与官方 gate exit 0，仍为49文件/4591行。父会话生产包完整回归 openai 0.196s、relay 0.420s，exit 0。本轮未改变数据库行为，未将本轮 SQLite 测试宣称为新一轮三库认证；上一轮真实三库证据仍保留。
