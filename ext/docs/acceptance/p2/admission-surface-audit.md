# P2 白名单入口与别名边界复核（rc.50，2026-11-13）

状态：**纯策略已实现并测试；H1 未接线，不宣称任何生产入口已受保护。**
独立复核：agent `f995ba5d-1b38-48a9-ae11-a098ac8e34a6`，只读官方源码；基线 `f2aa91232ed3e5878a614762ace6736a2b74257f`。无收费请求、无旧系统访问。

## 被证伪的方案假设

“AddChannel/UpdateChannel + Distribute + updatePricing 三处即可完整实施白名单”不成立。拒绝型门禁会改变控制流，不能称为“不改变控制流”的观察钩子。150行预算的可行性未证明，不增白名单文件让未核实实现混过去。

## 官方入口清单

### 1. 渠道写入与再次发布

| 路径 | 官方源码 | 必须防住的点 |
|---|---|---|
| Add/批量Add | controller/channel.go:817-938 | 规范化之后、任何持久化前，整批原子预检 |
| Update部分补丁 | 同文件:1214-1439 | 合并旧Type/plugin/settings和新补丁后再验；必须在1422的首个Key写入之前，而不只是1439 Update之前 |
| 复制渠道 | 同文件:1844-1863 | 不经过AddController，clone.Insert前需验 |
| 按标签改模型/映射 | 同文件:1047-1144 → model/channel.go:1015-1064 | 对全部受影响渠道的最终快照预验，保留nil无改动/空清除语义 |
| 自动发现/手动apply/apply_all | controller/channel_upstream_update.go:583-636,1078-1101,1160 | 共享更新函数583-591在写Models之前预验；不是模型元数据sync接口 |
| 启用已有/标签渠道 | model/channel.go:861-989 | 已存记录与政策变化下再次上架同样不能绕过；只验新增不够 |
| Ability重建 | model/channel.go:610/615,657/663 | 不是唯一安全入口：渠道可能先写库，再更新Ability；此时拒绝已太晚 |

### 2. 调用与WebSocket

- 普通文本、playground、Gemini、native/task/plugin大多有Distribute，但固定渠道分支也必须验，不能把钩子放在 `!pinned` 内（middleware/distributor.go:48-134）。
- shared task协议在Distribute前已经选候选（middleware/task_plugin.go:546），所以不能把Distribute称为“任何选路之前”。
- Responses WS的GET注册不经过Distribute（router/relay-router.go:76-80）。首次create/重试经service.SelectChannelForRequest（service/channel_select.go:308-401，relay/responses_websocket.go:1095）。
- **WS第二次及之后response.create复用连接，不重新选路**：relay/responses_websocket.go:297-310。必须在checkResponsesWSModelAccess的token空限制提前返回前（1067-1069）检查公开名，并在restoreConnectionContext加载渠道后、计费前（655-658）复核候选资格/政策变更。
- 候选筛除与最终拒绝不同：拒绝一个候选不应错误地封死仍有合格候选的模型；pins、affinity、DB/内存路由、重试都需要覆盖。

### 3. 客户可见目录

| 路由 | 官方源码 | 绕过点 |
|---|---|---|
| /v1/models及Anthropic/Gemini变体 | controller/model.go:251-263 | 直接取enabled models，不经pricing缓存 |
| /v1/models/:model | 同文件:348-360 | 静态map直接取单模型，甚至不依赖enabled abilities |
| /api/user/self/models | controller/user.go:645-649 → service/group.go:111-122 | 不经pricing |
| /api/models | controller/model.go:325-338，router/api-router.go:30 | **普通UserAuth而非admin-only**；内置表+插件Meta.Models都需过滤 |
| /api/pricing | model/pricing.go:217-320,373,476 | 应在Ability集合进入vendor/group/endpoint索引前过滤；末尾只删模型会残留供应商/分组投影 |

管理员上游发现列表、ChannelListModels、EnabledListModels、FetchModels等与对客目录分开设计。发现不等于公开；不得凭上游列表自动扩充本站白名单。

## 模型身份与最小接口候选（未批准实现）

公开名 Public、插件声明名 Declared、最终上游名 Upstream、实际执行 PluginKey 是四个不同事实。

- relay/helper/model_mapped.go:29-66 跟随多跳映射，带base-model fallback、自映射提前返回与循环检测；上游检查必须覆盖自映射，且在请求SetModelName或任何插件网络动作之前。不能只检查公开别名。
- 公开允许的别名映射到未批准上游不能放行；未批准公开别名映射到合格上游也不能自动公开。上游不透明部署ID需单独批准映射，不按子串、前缀或大小写猜国别。
- type61必须带实际task_plugin_key；type60有多个绑定；legacy类型也有插件。GetByModel取第一个元数据owner，不代表执行器（pkg/jsplugin/routing.go:515-552）。
- 插件会做大小写canonicalization（middleware/task_plugin.go:572-636）；如果政策要原始输入精确匹配，必须保留raw identity，不得在折叠后宣称做过原始精确检查。

`ext/core`候选仅含值类型，不import model/service，防循环：

```go
type ModelIdentity struct { Public, Declared, Upstream, PluginKey string }
type ChannelIdentity struct { ID, Type int; PluginKeys []string }
type ChannelSpec struct { ChannelIdentity; Models []string; Mapping map[string]string }
// Gate候选：ValidateChannel(ctx, spec) error
// CheckRequest(ctx, identity, optionalCandidate) error
// VisibleModel(ctx, identity, optionalCandidate) bool
```

nil candidate表示未解析，不能把type0当授权；上游空表示未解析，不等于已通过。以上还不是通过验收的API。

## 本轮已交付的纯策略

`ext/domestic/policy.go`：默认空、nil拒绝；build+env并集；完整解析失败不安装半成品；精确公开名与类型；不做alias/namespace/case自动拓展；过滤保序、去重、不改缓存切片。未注册于ext.Register，无运行时副作用。

`go test -p 2 ./ext/... -count=1`、`go vet ./ext/...`通过。仅证明纯解析与授权集合规则，不证明上游目标/插件身份或上述入口接线。尚未用ldflags实际注入制品验收。

## 接线退出标准

必须有与本清单逐项对应的拒绝/允许集成测试：批量新增、部分补丁、copy/tag/auto-sync/apply_all/status；所有客户目录及单模型retrieve；pins/retries与WS第二create；公开别名/上游目标/实际插件不同维度。不能用一个Policy单测代替。若预算不足，提交RFC，不减少测试、不绕过规则。
