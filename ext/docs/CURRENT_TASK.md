# 连续推进检查点

更新时间：2026-11-13；范围 P0→P1→P2。用户授权阶段级连续推进，遇到规则/架构/费用/生产凭据决策才停。

## 当前事实

- 最近已推 `fba46097b`（03bc32218后仅taskauthority RFC）：main/develop一致；sourceveto差异CI43542153488/43542147069 success。当前sync/20261114-rc51真实merge新release c9c6da278无冲突，后端/前端/三库验证进行中；未赋予新task权限。所有Gate仍noop，TaskContinuations默认nil。
- P0：20前端测试 +23 Go回归子场景 +13HTTP +7真实Chrome会话项通过。
- 用户已批准R3最小官方前端回跳例外（ext/AGENTS v0.7）；真实官方生产表单复验5项PASS：匿名自动回CMH、已登录beforeLoad回CMH、重载、退出、普通dashboard仍SPA。证据 `acceptance/p0/T0.2-bridge-pass.md`。旧失败证据保留。
- 官方相关前端99+10测试、类型1488文件/2项目、4文件lint与production构建均通过；独立只读R3安全复核无阻断问题。门禁8文件/97行。
- 全量Go回归exit1：controller/middleware Windows占用文件cleanup；relay/channel HTTP2错误文案；plugins固定清单17vs18由本次cmh插件引入，不能忽略。详见 `acceptance/p1/full-backend-windows-regression.md`。
- P1：完整生命周期独立只读复核推翻“四个唯一出口”。追加预留、Rollback、直接PostConsumeQuota、Realtime/违规/MJ、token更新、后台任务重启身份都不在旧假设覆盖内。未实施任何资金钩子；用户同事务选择保持，不自动改成替代钱包或双扣最终一致。证据 `acceptance/p1/funding-lifecycle-audit.md`；设计RFC `rfc/2026-11-13-funding-transaction-boundary-review.md`未批准。
- P2：已完成纯Policy、定向插件和H1-A候选/目录首批实际接线，未激活core.Models。完整旁路审查见 `acceptance/p2/admission-surface-audit.md`；当前已验范围和未完边界见 `acceptance/p2/H1-A-candidate-catalog.md`，不是完整H1通过。
- GitHub latest实际rc.51（2026-11-13发布），本轮已fetch并在sync分支真实merge；官方34文件原样、无冲突。证据 `acceptance/p1/upstream-rc51-sync.md`待最终绑定，不再只作同tag检查。

## 当前需要的用户决策（不是逐小节确认）

R3/R4及H1专项多调用点/core导入均已批准。32文件硬限到达后，负责人明确选择“批准累计48文件/6000行收口预算（推荐）”，v0.9与CI已经独立rules PR #1落实；只限H1剩余执行链、目的地、后台延续和兼容缺口，逐文件登记/80%复核不变。直接继续，不再重复问同一预算；不含费用/生产/资金重构，额度不等于完成保证。

负责人于2026-11-14另明确批准唯一middleware/task_plugin.go第49文件例外，限RouteTaskPluginEndpoint/必要Pin capturedgeneration且该文件≤40增删行；累计6000不变。v0.10与门禁已rules PR #2落实；不再重复问同一许可，不授权任意第49文件或第50文件/全局启用。负责人另已明确批准 `rfc/2026-11-14-task-continuation-authority.md` 有限首批合同：显式逐task有限hook/到期/重启全承载节点撤销/static per_task，无utils.fetch/refresh/oauth令牌交换，必须enabled exact目的地policy；仅合成验收与实现，不是实际grant或全局启用授权。资金同事务方案仍单独设计复核；H1审批不等于H3/H7审批或三库测试通过。

## 下一动作

1. `cmh-param-normalize`官方CLI lint与43/43 golden、真实CallTransform+patch通过。它会随官方整目录embed自动工厂注册，但未创建启用的请求实例；旧“不注册”描述已纠正。
2. R3五项Chrome表单通过、R4工厂测试完整通过。纯官方rc50字节精确隔离archive定向复验已完成：controller同名SQLite矩阵和middleware日志文件cleanup失败复现；relay HTTP2单次通过、未复现，不能笼统归环境。证据 `acceptance/p1/windows-baseline-reproduction.md`。relaykit/tokenkit各 `GOWORK=off go build -p 2 ./...` exit0。
3. 本轮rc51真实同步：sync/20261114-rc51无冲突原样merge官方34文件，moegoalpha3及前端依赖/密码反馈更新。rootvet/build/相关tests、独立relaykit/tokenkit、admission+archive真实三库、CLI43fixtures、官方235/2887前端fulltests/type/prodbuild与CMH20tests全过；Windowsfullroot仅controller/middleware cleanup失败并controller累计10mtimeout，单独末例0.768s过，optional全web lint148errors未修，不能全绿。证据 `acceptance/p1/upstream-rc51-sync.md`；接续远程完整UbuntuPR CI过后合main/develop。全GateStore未装。
4. P1先验证FundingSource工厂+tx-aware契约可行性；不用函数提前return跳过token/会话内部状态，不伪造缓存余额。
5. 已从发布方取得便携MySQL8.0.44/PG17.12，独立loopback实例和SCRAM/随机测试账户用于本轮三库验证，均已优雅退出。引擎与专属数据在忽略 `ops/.cache/db-test`，启动脚本与env只本地保留，不提交凭据。今后可直接复用该独立环境；不借生产DSN。三库通过仅本批读查询/候选，不等于资金/事务升级全部通过。

## 命令与环境

- Go1.26.10已缓存，调用使用临时 `GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct`。
- `go test -p 2 ./ext/... -count=1` exit0，`go vet ./ext/...`此前exit0。race未通过：CGO/C编译器缺失，不宣称通过。
- 控制台pnpm.cmd（历史bun缺失）；官方完整web使用portable exact Bun `ops/.cache/web-build-p0/bun-1.4.4/bun-windows-x64/bun.exe`，未全局安装。
- 官方原始rc50构建旧SHA在失败复现证据；当前R3桥接资源/测试二进制的完整SHA绑定在 `acceptance/p0/T0.2-bridge-pass.md`。两者都是实际production模式构建，不是占位页。后续正式发布必须重新冻结源码重建，不把本地测试exe当上线制品。
- 门禁目前49官方文件/4240行，只有命名middleware/task_plugin.go第49例外（该file14/40行）才生效49上限，累计6000不变；剩0新文件/1760行，不含任意第50文件。既有范围登记/80%复核和独立review保留。分支保护未认证。

## 临时资源与作业

- 全部本地gateway与独立Chrome已退出，后台build输出已收；本轮官方build由独立代理完成且其jobs已收。
- `%TEMP%/cmh-gateway-ui-acceptance.exe`和旧proto.exe为本任务测试二进制，后续验收仍需，不删除。
- `%TEMP%/cmh-p0-*.db`及独立Chrome profile为合成资料，禁止提交；清理前核对精确绝对路径与进程，不做广泛匹配删除。
- 官方Web依赖/构建缓存 `web/node_modules`、`web/dist`、`ops/.cache` 为本轮所建，后续UI验收仍需，保留。
- 接续task-scoped独立grant：archiveSave在build前，仅字节完整性不是授权，Loadpurpose枚举/RestoreBind/ref不能mint权限。可选第一批per_task staticcredential有限buildQuery/parse/sign/usage显式operator精确task/ref/purpose/source/runtime许可+authenticatedLoad私有scope，禁utils.fetch/refresh/content/report/batch，实际transport仍globalexactdestination；需要明确expiry/revocation权限合同再实接，不自动授权未来签名queryURL。其余affinity/type60/legacy与keycustody/备份/GCM/fullactivation未完，49/1760硬限保留；金融同事务RFC独立未批，timeout未改。
- 本轮完整 `go build -p 2 ./...`、model/helper/plugins/ext tests和对应vet均exit0；三库最终修复后pass。全仓Windows失败历史仍未宣称消失。
- 只提交精确源码和脱敏验收路径，不 `git add -A`扫数据库/缓存。

## 禁止误报

不访问旧生产/不迁客户数据/不做付费上游；不把纯Policy单测称为live白名单；不把CLI插件fixture称为已激活；不把同tagAlready-up-to-date称同步验收；不把API登录或手动刷新称为官方表单自动跨应用成功。
