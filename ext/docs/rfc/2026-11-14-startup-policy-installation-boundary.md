# RFC（未批准）：H1 启动准备与原子安装边界

## 本轮决定：不增加同义 loader，不安装全局策略

现有 `LoadOperatorPolicies(environment map[string]string)` 已完成显式环境快照、build/env 锚一致性、原始文件摘要、大小/普通文件约束及严格 manifest 编译。全部缺省返回 nil；部分或错误配置返回固定错误。`ParseOperatorManifest` 总是产生具体 Gate、DestinationPolicy、ContinuationPolicy；Gate 同时实现 ModelGate/PluginHookGate/ExecutorGate。因此再封装“Enabled + 四个非 nil 接口”的对象只能证明构造器运行，不能证明启动失败关闭，不增加另一层可被误认为上线证明的 wrapper。

空授权集合是有效的 deny-all 策略，不应因无 executor 或无 destination 条目而被称为 noop。目的地授权也不能强制属于同一 executor approval 集合：fetch capability 与 task-scoped 延续有独立配置身份，简单集合包含会错误拒绝合法有限授权。反过来，存在 grant 不能证明档案 store、密钥或迁移已可用。

## 当前必须区分的三种状态

1. Disabled：显式 manifest path、env SHA 与 build SHA 全缺省，保持官方兼容；不是故障回退。
2. Prepared：现有 loader 认证的具体不可变 policies；尚未安装，不能开放请求/worker。
3. Serving：受控启动已验证全部必要资源并在任何入口执行前安装一个一致策略集合。当前未实现，也未授权本 RFC 实施或生产激活。

配置存在但失效必须退出启动，不能退回 Disabled、旧配置或仅启用部分 gate。安装失败也不能继续官方默认允许路径。

## 建议的有限实施合同（需负责人批准）

- 使用启动期显式传入的环境快照；复用现有 loader，不另读进程环境、不重复 JSON 解析。
- 启动负责方须在 router 接受请求、poll/report/system workers 启动前完成准备。现有多个可变 core globals 并非原子发布容器；仅串行赋值不等于并发原子安装。必须核对真实启动顺序，并通过屏障保证发布前无消费者，或另行批准 core 单快照容器改造，不能在 ext 里假装原子。
- 同一 Gate 必须同时绑定 Models、PluginHooks、Executors，具体 DestinationPolicy 同批绑定。禁止允许调用者替换其中一个后仍自称完整 prepared。
- 初版可将 continuation 明确设为 disabled，且拒绝任何非空 Continuations；不能静默忽略 manifest grant。若需要开启 archive/grants，则必须在同一次准备中验证独立 store/migration/keyring 方案，不能仅凭非 nil store 指针作为 proof。
- archive 密钥来源、key ID、保留旧解密 key、离线备份恢复、随机 GCM nonce 的每 key 写入寿命上限与轮换、损坏/缺档修复责任必须单独批准。不得自动生成 key、读取测试或生产 key、以重启替代持久密钥。
- 全部承载节点使用同一制品/策略摘要，grant 撤销按既有全节点替换重启合同；不新增热更新或即时撤销承诺。
- 只输出不含路径/原始配置的状态与 canonical fingerprint；不打印文档、密钥、URL 或 archive runtime。

## 需要拍板的具体选择

A（建议先做）：批准 policy-only 启动合同，显式禁用 archive/grants，存在 grant 就拒绝启动；在既登记启动文件内证明发布发生在全部 consumer 之前。该选择仍不等于实际启用 gate 或生产上线。

B：一次加入 archive/grants，则先批准完整密钥运维与迁移准备合同；不是给 loader 添加一个 bool。

选择 A 还须接受当前严格 gate 的兼容限制：legacy 当前 owner 所在普通渠道也会保守拒绝、type60 插件绑定与启用 request plan 等仍未完整支持。不能以启动完整为名取消这些拒绝。

## 后续实现与验收范围

1. 主代理已实读 `main.go`：main第63行调用InitResources；其内部dotenv347、InitEnv355、logger357、DB365、options398、log DB404、Redis410、monitor418及auth cleanup440，返回后cache106/pricing115/pluginSync119、go/workers109/111/143、systemRunner218/listen272。因此“InitResources返回后、workers之前”已晚于monitor/auth cleanup，虽尚未证明其读取gate，也不能称无消费者。policy-only候选位置是InitEnv355/logger357之后、DB365之前；实施前仍需核对package init、InitEnv副作用及所有首次gate读取依赖，不能据此声称完整屏障。若需要新官方文件或超过现有登记范围，先独立 RFC，仍受累计49文件/6000行及命名例外限制。
2. 在现有 domestic 测试中验证 all-absent disabled、partial/invalid fatal、deny-all 有效、不能混合四个 policy 实例、含 grant 的 policy-only 启动拒绝；不引入新的 parser 格式。
3. 在合成启动进程中证明非法配置时没有监听端口/worker/hook/network；合法准备完成前没有消费者；启动错误不恢复默认允许。该项必须测试真实顺序，纯结构体断言不够。
4. 若选择 archive，按官方规则真实 SQLite/MySQL/PostgreSQL 新库/升级/重复启动与备份恢复故障验证，再独立复核；本轮不访问 DB 或 archive key。
5. 保持 ext-only 准备和最终安装分离，负责人明确批准实施合同后才动启动接线；实际生产激活需另行明确授权。

## 本轮代码事实与验证边界

已实际阅读 root/ext 规则、operator loader/manifest、Gate、core destination/continuation、ContinuationPolicy 及现有 policy 测试。无官方源码、global、plugin host、资金或 DB 修改。本 RFC 不宣称完整启动 ready。实际运行 `go test -p 2 ./ext/... -count=1` PASS（domestic8.697s、continuation0.149s、fundingcontract0.097s）；`go vet -p 2 ./ext/...` exit0，job已收取，无遗留作业。测试没有证明新启动屏障，也不是生产配置/密钥/DB验收。
