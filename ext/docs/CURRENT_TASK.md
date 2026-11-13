# 连续推进检查点

更新时间：2026-11-13；范围 P0→P1→P2。用户授权阶段级连续推进，遇到规则/架构/费用/生产凭据决策才停。

## 当前事实

- 最近已推 `87b03f896`：main/develop一致，差异门禁两次success（43486881526 / 43486873065），本轮extension operator manifest待提交。48/6000规则已落实；完整Ubuntu CI证据见 `acceptance/p2/H1-closeout-approval.md`。分支保护未认证。所有Gate仍noop，TaskContinuations默认nil。
- P0：20前端测试 +23 Go回归子场景 +13HTTP +7真实Chrome会话项通过。
- 用户已批准R3最小官方前端回跳例外（ext/AGENTS v0.7）；真实官方生产表单复验5项PASS：匿名自动回CMH、已登录beforeLoad回CMH、重载、退出、普通dashboard仍SPA。证据 `acceptance/p0/T0.2-bridge-pass.md`。旧失败证据保留。
- 官方相关前端99+10测试、类型1488文件/2项目、4文件lint与production构建均通过；独立只读R3安全复核无阻断问题。门禁8文件/97行。
- 全量Go回归exit1：controller/middleware Windows占用文件cleanup；relay/channel HTTP2错误文案；plugins固定清单17vs18由本次cmh插件引入，不能忽略。详见 `acceptance/p1/full-backend-windows-regression.md`。
- P1：完整生命周期独立只读复核推翻“四个唯一出口”。追加预留、Rollback、直接PostConsumeQuota、Realtime/违规/MJ、token更新、后台任务重启身份都不在旧假设覆盖内。未实施任何资金钩子；用户同事务选择保持，不自动改成替代钱包或双扣最终一致。证据 `acceptance/p1/funding-lifecycle-audit.md`；设计RFC `rfc/2026-11-13-funding-transaction-boundary-review.md`未批准。
- P2：已完成纯Policy、定向插件和H1-A候选/目录首批实际接线，未激活core.Models。完整旁路审查见 `acceptance/p2/admission-surface-audit.md`；当前已验范围和未完边界见 `acceptance/p2/H1-A-candidate-catalog.md`，不是完整H1通过。
- 最新fetch官方release仍rc.50，同tag检查只是基线祖先验证，**不是新release升级演练**。

## 当前需要的用户决策（不是逐小节确认）

R3/R4及H1专项多调用点/core导入均已批准。32文件硬限到达后，负责人明确选择“批准累计48文件/6000行收口预算（推荐）”，v0.9与CI已经独立rules PR #1落实；只限H1剩余执行链、目的地、后台延续和兼容缺口，逐文件登记/80%复核不变。直接继续，不再重复问同一预算；不含费用/生产/资金重构，额度不等于完成保证。

资金同事务方案仍单独设计复核；H1审批不等于H3/H7审批或三库测试通过。

## 下一动作

1. `cmh-param-normalize`官方CLI lint与43/43 golden、真实CallTransform+patch通过。它会随官方整目录embed自动工厂注册，但未创建启用的请求实例；旧“不注册”描述已纠正。
2. R3五项Chrome表单通过、R4工厂测试完整通过。纯官方rc50字节精确隔离archive定向复验已完成：controller同名SQLite矩阵和middleware日志文件cleanup失败复现；relay HTTP2单次通过、未复现，不能笼统归环境。证据 `acceptance/p1/windows-baseline-reproduction.md`。relaykit/tokenkit各 `GOWORK=off go build -p 2 ./...` exit0。
3. 本轮extension OperatorManifest：显式public/upstream/type、source/config Executor及destinationhash批准canonicalwire v1，Parse拒unknown/duplicate/ambiguous/size等，返回immutableGate+targetPolicy/fingerprint不安装全局。review发现Encode先Marshal会invalidUTF8替换U+FFFD，主代理补原始全部identity字符串UTF8/长度及counts/version前置检查+models/hook断言；exttests/vet/build过，证据 `acceptance/p2/H1-C-operator-manifest.md`。拟pinnedGeneration放dto因dto→jsplugin→relay/common→dto实环停该方法，无源码改动/any逃逸，缺口保守deny未假称完成。官方本轮0改动/allGateStore未装。
4. P1先验证FundingSource工厂+tx-aware契约可行性；不用函数提前return跳过token/会话内部状态，不伪造缓存余额。
5. 已从发布方取得便携MySQL8.0.44/PG17.12，独立loopback实例和SCRAM/随机测试账户用于本轮三库验证，均已优雅退出。引擎与专属数据在忽略 `ops/.cache/db-test`，启动脚本与env只本地保留，不提交凭据。今后可直接复用该独立环境；不借生产DSN。三库通过仅本批读查询/候选，不等于资金/事务升级全部通过。

## 命令与环境

- Go1.26.10已缓存，调用使用临时 `GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct`。
- `go test -p 2 ./ext/... -count=1` exit0，`go vet ./ext/...`此前exit0。race未通过：CGO/C编译器缺失，不宣称通过。
- 控制台pnpm.cmd（历史bun缺失）；官方完整web使用portable exact Bun `ops/.cache/web-build-p0/bun-1.4.4/bun-windows-x64/bun.exe`，未全局安装。
- 官方原始rc50构建旧SHA在失败复现证据；当前R3桥接资源/测试二进制的完整SHA绑定在 `acceptance/p0/T0.2-bridge-pass.md`。两者都是实际production模式构建，不是占位页。后续正式发布必须重新冻结源码重建，不把本地测试exe当上线制品。
- 门禁目前47官方文件/3833行，H1硬限48/6000，剩1文件/2167行；文件数80%范围复核保留，实施/官方测试均精确登记，不能凭估计保证全收口。分支保护未认证。

## 临时资源与作业

- 全部本地gateway与独立Chrome已退出，后台build输出已收；本轮官方build由独立代理完成且其jobs已收。
- `%TEMP%/cmh-gateway-ui-acceptance.exe`和旧proto.exe为本任务测试二进制，后续验收仍需，不删除。
- `%TEMP%/cmh-p0-*.db`及独立Chrome profile为合成资料，禁止提交；清理前核对精确绝对路径与进程，不做广泛匹配删除。
- 官方Web依赖/构建缓存 `web/node_modules`、`web/dist`、`ops/.cache` 为本轮所建，后续UI验收仍需，保留。
- 接续pinned候选typedhostoverload/coreview不能直接dtoimportjsplugin（真实环），无any/反射/按name-number找旧制品；OperatorManifest仅purecompiler还无env/ldflagsstartuploader或原子安装，不能当enable。type61publication已修。task-scoped旧grant/credentialrefresh、affinity/type60/legacy、keycustody备份GCM预算/fullactivation未完；只余1officialslot2167行，复用已有file，不删测试凑。H3/H7金融同事务另RFC未批准，缺档skip本次原timeout仍退款。
- 本轮完整 `go build -p 2 ./...`、model/helper/plugins/ext tests和对应vet均exit0；三库最终修复后pass。全仓Windows失败历史仍未宣称消失。
- 只提交精确源码和脱敏验收路径，不 `git add -A`扫数据库/缓存。

## 禁止误报

不访问旧生产/不迁客户数据/不做付费上游；不把纯Policy单测称为live白名单；不把CLI插件fixture称为已激活；不把同tagAlready-up-to-date称同步验收；不把API登录或手动刷新称为官方表单自动跨应用成功。
