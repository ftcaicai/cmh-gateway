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

本次新增的是实际 codec 的中央 guard 回归，未新增 raw socket 攻击帧验收；既有 WS 回归仍通过。不证明供应商解析行为、任意正文安全、完整 TOCTOU 消除或资金守恒。资金、H4/H6 实施拒绝保持有效。
