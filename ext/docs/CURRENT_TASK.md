# 连续推进检查点

更新时间：2026-11-14；范围 P0→P1→P2。用户授权阶段级连续推进，2026-11-14明确将自动轮数上限改为80并恢复active/armed；遇到规则/架构/费用/生产凭据决策才停。

## 当前事实

- 静态resolver owner修复已发布 `2c880f9e6e765b04cf2a62393d3ae0ea27cfd9e3`，main/develop一致，门禁43586768786/43586763330 success：共享原始platform映射/优先级，sora无channelTypes和numeric fallback不得投影builtin。真实三库22.005s及集成相关全tests/vet/fullbuild通过，数据库均已停止。累计49官方文件4475行/剩1525，named exception14/40不变。第60轮只读核查未发现旧pinned legacy owner消失场景的执行旁路：真实非空task witness在current owner查询前即unknown，Setup与预扣前仍拒绝非type61；普通无task路径不应blanket deny。不为此增加重复生产守卫，不代表legacy正向执行支持。
- 最近已推 `0851f7369adc17dbc0adaa5d7d6d8a6a31f26822`，main/develop一致，门禁43585172419/43585168004 success。第56轮实际loader同路径合法替换回归：旧SHA拒绝、新SHA采用新审批、旧返回快照独立，定向/extension全tests及vet通过（domestic8.245s），独立只读复核无阻断；测试提交5f596065a。第57轮该源码实际根二进制ldflags注入验收：build-only正确文件到故意DB失败点、冲突env SHA与追加LF文件均在DB前固定拒绝，三个子进程退出、无SQLite/监听/上游；证据 `acceptance/p2/H1-C-policy-only-startup.md`。不代表热更新、全局撤销、来源签名或生产启用。H4/H6负责人明确选择“均保持设计，不批准实施”；不创建对应规则PR、不扩大预算、不试做接线、不重复申请同一授权。资金负责人亦明确选择“继续设计，不批准实施”：不扩官方区、不创建实施rules PR、不试做资金代码；不重复申请同一授权。有限资金方向及经济/恢复/memory-only设计批准保留。只读候选18必需＋6条件＝3既有＋15新增必需，组合64；全条件最多69官方路径，测试拟集中扩展区，这不是获批范围或行数预测。生产模型可靠计费上界尚未证明、资格为空；后续仅收敛接口/状态转换/故障验证规格，见 `acceptance/p1/funding-engineering-proof-gaps.md`。rc51同步、policy-only合成Serving与目录对照已验；整体P1/P2未完成。
- 第60轮request-plan只读调查未发现可直接删除的冗余拒绝：单体source/config/stage审批不等于基础executor与完整有序计划的组合授权；非nil plan可在后续channel/group阶段进入scope，有限billing字段守卫也不是opaque body身份证明。暂不改生产守卫。空执行计划兼容需冻结私有plan、覆盖全部阶段和最终group/channel、重试重新验证，并解决publication/runtime区别；尚未设计批准，不借此启用normalize实例。
- P0：20前端测试 +23 Go回归子场景 +13HTTP +7真实Chrome会话项通过。
- 用户已批准R3最小官方前端回跳例外（ext/AGENTS v0.7）；真实官方生产表单复验5项PASS：匿名自动回CMH、已登录beforeLoad回CMH、重载、退出、普通dashboard仍SPA。证据 `acceptance/p0/T0.2-bridge-pass.md`。旧失败证据保留。
- 官方相关前端99+10测试、类型1488文件/2项目、4文件lint与production构建均通过；独立只读R3安全复核无阻断问题。门禁8文件/97行。
- 全量Go回归exit1：controller/middleware Windows占用文件cleanup；relay/channel HTTP2错误文案；plugins固定清单17vs18由本次cmh插件引入，不能忽略。详见 `acceptance/p1/full-backend-windows-regression.md`。
- P1：完整生命周期独立只读复核推翻“四个唯一出口”。追加预留、Rollback、直接PostConsumeQuota、Realtime/违规/MJ、token更新、后台任务重启身份都不在旧假设覆盖内。未实施任何资金钩子；用户同事务选择保持，不自动改成替代钱包或双扣最终一致。证据 `acceptance/p1/funding-lifecycle-audit.md`；设计RFC `rfc/2026-11-13-funding-transaction-boundary-review.md`未批准。
- P2：已完成纯Policy、定向插件和H1-A候选/目录首批实际接线，未激活core.Models。完整旁路审查见 `acceptance/p2/admission-surface-audit.md`；当前已验范围和未完边界见 `acceptance/p2/H1-A-candidate-catalog.md`，不是完整H1通过。
- 官方rc51 c9c6da278已真实无冲突merge并经完整LinuxCI后合main/develop，sync-log当前基线rc51；Windowscleanup及全lint失败保留。证据 `acceptance/p1/upstream-rc51-sync.md`。

## 当前需要的用户决策（不是逐小节确认）

R3/R4及H1专项多调用点/core导入均已批准。32文件硬限到达后，负责人明确选择“批准累计48文件/6000行收口预算（推荐）”，v0.9与CI已经独立rules PR #1落实；只限H1剩余执行链、目的地、后台延续和兼容缺口，逐文件登记/80%复核不变。直接继续，不再重复问同一预算；不含费用/生产/资金重构，额度不等于完成保证。

负责人于2026-11-14另明确批准唯一middleware/task_plugin.go第49文件例外，限RouteTaskPluginEndpoint/必要Pin capturedgeneration且该文件≤40增删行；累计6000不变。v0.10与门禁已rules PR #2落实；不再重复问同一许可，不授权任意第49文件或第50文件/全局启用。负责人另已明确批准 `rfc/2026-11-14-task-continuation-authority.md` 有限首批合同：显式逐task有限hook/到期/重启全承载节点撤销/static per_task，无utils.fetch/refresh/oauth令牌交换，必须enabled exact目的地policy；仅合成验收与实现，不是实际grant或全局启用授权。资金同事务方案仍单独设计复核；H1审批不等于H3/H7审批或三库测试通过。

## 下一动作

1. `cmh-param-normalize`官方CLI lint与43/43 golden、真实CallTransform+patch通过。它会随官方整目录embed自动工厂注册，但未创建启用的请求实例；旧“不注册”描述已纠正。
2. R3五项Chrome表单通过、R4工厂测试完整通过。纯官方rc50字节精确隔离archive定向复验已完成：controller同名SQLite矩阵和middleware日志文件cleanup失败复现；relay HTTP2单次通过、未复现，不能笼统归环境。证据 `acceptance/p1/windows-baseline-reproduction.md`。relaykit/tokenkit各 `GOWORK=off go build -p 2 ./...` exit0。
3. grant redirectexpiry已于cc5898a62发布，initial1/后继0/Unavailable真实三库通过。本轮继续affinity主线：stored base + final effective各独立精确审批，官方merge一次、key前批准同一map；runtime仅允许ParamOverride差异且其它权威配置一致。真实规则cache miss/provider wire、双撤销/context与RelayInfo篡改/未批fresh base通过三库21.946s及相关tests/vet/fullbuild；独立静态agent复核无阻断。候选首次fixture缺provider身份witness导致三库断言失败，改真实filter重跑通过，未改生产检查。证据 `acceptance/p2/H1-C-affinity-effective.md`。仅有限provider新增positive，非task-affinity全协议/来源证明，不启用全局。
4. P1已按资金RFC允许范围完成隔离纯值合成原型，12显式用例及独立复核通过，未导入任何生产资金路径；不是同事务/真实账本/Redis×BATCH证明。见 `acceptance/p1/funding-value-contract.md`。资金实施仍需独立批准，不用函数提前return跳过token/会话内部状态，不伪造缓存余额。
6. 启动RFC `rfc/2026-11-14-startup-policy-installation-boundary.md` 的A policy-only合同已经明确批准并于dcba0c5f7接线；InitEnv/logger后DB/worker前安装，错误早停、含grant拒绝、archive/grants禁用。真实六类子进程与扩展测试通过，预main factory编译例外保留；第45轮同一实际二进制已成功合成Serving：loopback/全新SQLite3.50.4、真实Bearer普通用户，models200空、chat403 access_denied、未认证401对照、upstream0，全部gateway/stub退出；见 `acceptance/p2/H1-C-policy-only-serving.md`。第46轮复用隔离合成库，同显示偏好下精确允许目录1项/deny-all目录0项，真实Bearer/401对照/上游0，未发chat，全部已退出；见 `acceptance/p2/H1-C-policy-only-serving.md`。不是三库启动认证/全协议可用或生产授权。
5. 已从发布方取得便携MySQL8.0.44/PG17.12，独立loopback实例和SCRAM/随机测试账户用于本轮三库验证，均已优雅退出。引擎与专属数据在忽略 `ops/.cache/db-test`，启动脚本与env只本地保留，不提交凭据。今后可直接复用该独立环境；不借生产DSN。三库通过仅本批读查询/候选，不等于资金/事务升级全部通过。

## 命令与环境

- Go1.26.10已缓存，调用使用临时 `GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct`。
- 第54轮 `go test -p 2 ./ext/... -count=1` 与 `go vet -p 2 ./ext/...` exit0（domestic8.338s）；仅既有实现回归，不是资金/H4/H6验收。第55轮再次查询上游latest仍为v1.0.0-rc.51（2026-11-13发布）；未伪造新升级演练。race未通过：CGO/C编译器缺失，不宣称通过。
- 控制台pnpm.cmd（历史bun缺失）；官方完整web使用portable exact Bun `ops/.cache/web-build-p0/bun-1.4.4/bun-windows-x64/bun.exe`，未全局安装。
- 官方原始rc50构建旧SHA在失败复现证据；当前R3桥接资源/测试二进制的完整SHA绑定在 `acceptance/p0/T0.2-bridge-pass.md`。两者都是实际production模式构建，不是占位页。后续正式发布必须重新冻结源码重建，不把本地测试exe当上线制品。
- 门禁目前49官方文件/4475行，只有命名middleware/task_plugin.go第49例外（该file14/40行）才生效49上限，累计6000不变；剩0新文件/1525行，不含任意第50文件。既有范围登记/80%复核和独立review保留。分支保护未认证。

## 临时资源与作业

- 全部本地gateway与独立Chrome已退出，后台build输出已收；本轮官方build由独立代理完成且其jobs已收。
- `%TEMP%/cmh-gateway-ui-acceptance.exe`和旧proto.exe为本任务测试二进制，后续验收仍需，不删除。
- `%TEMP%/cmh-p0-*.db`及独立Chrome profile为合成资料，禁止提交；清理前核对精确绝对路径与进程，不做广泛匹配删除。
- 官方Web依赖/构建缓存 `web/node_modules`、`web/dist`、`ops/.cache` 为本轮所建，后续UI验收仍需，保留。
- 接续总体剩余：有限static per_task授权已实接/真三库，批准制品loader与policy-only早期安装已接线且合成早停通过，默认无配置仍noop；batch/content/report/refresh/dynamic目的地task权限未覆盖。affinity双精确envelope provider及task选中/捕获/实际描述符提交/撤销已验；task body不应用普通provider operations，未改官方语义。type60普通未绑定路径为builtin，绑定路径仍拒绝；legacy当前by-type owner已无条件unknown，未声明模型保存/目录/DB与memory候选三库拒绝；默认sora/type1普通chat也保守拒绝，owner缺失/旧pinned/静态platform整体身份仍未解决。证据 `acceptance/p2/H1-C-legacy-owner-projection.md`。更广重试覆盖未认证；新实例requestplan与最终startup failclosed/keycustody备份GCM预算/完整启用验收仍未完。金融同事务RFC独立未批，timeout不改；不能因有限首批成功就装全局或宣称P1/P2完成。49/1596硬限保留，无第50file或命名sharedendpoint>40授权。
- 本轮完整 `go build -p 2 ./...`、model/helper/plugins/ext tests和对应vet均exit0；三库最终修复后pass。全仓Windows失败历史仍未宣称消失。
- 只提交精确源码和脱敏验收路径，不 `git add -A`扫数据库/缓存。

## 禁止误报

不访问旧生产/不迁客户数据/不做付费上游；不把纯Policy单测称为live白名单；不把CLI插件fixture称为已激活；不把同tagAlready-up-to-date称同步验收；不把API登录或手动刷新称为官方表单自动跨应用成功。
