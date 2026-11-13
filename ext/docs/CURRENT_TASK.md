# 连续推进检查点

更新时间：2026-11-13；范围 P0→P1→P2。用户授权阶段级连续推进，遇到规则/架构/费用/生产凭据决策才停。

## 当前事实

- 已推 `2d4fc7721`：main/develop一致，R3与R4远程CI各两次success；当前干净工作树。R4修复完整plugins与ext测试通过，见对应RFC。
- P0：20前端测试 +23 Go回归子场景 +13HTTP +7真实Chrome会话项通过。
- 用户已批准R3最小官方前端回跳例外（ext/AGENTS v0.7）；真实官方生产表单复验5项PASS：匿名自动回CMH、已登录beforeLoad回CMH、重载、退出、普通dashboard仍SPA。证据 `acceptance/p0/T0.2-bridge-pass.md`。旧失败证据保留。
- 官方相关前端99+10测试、类型1488文件/2项目、4文件lint与production构建均通过；独立只读R3安全复核无阻断问题。门禁8文件/97行。
- 全量Go回归exit1：controller/middleware Windows占用文件cleanup；relay/channel HTTP2错误文案；plugins固定清单17vs18由本次cmh插件引入，不能忽略。详见 `acceptance/p1/full-backend-windows-regression.md`。
- P1：完整生命周期独立只读复核推翻“四个唯一出口”。追加预留、Rollback、直接PostConsumeQuota、Realtime/违规/MJ、token更新、后台任务重启身份都不在旧假设覆盖内。未实施任何资金钩子；用户同事务选择保持，不自动改成替代钱包或双扣最终一致。证据 `acceptance/p1/funding-lifecycle-audit.md`；设计RFC `rfc/2026-11-13-funding-transaction-boundary-review.md`未批准。
- P2：`ext/domestic`纯不可变fail-closed策略实现并测试通过；未接H1、未激活live gate。独立审查发现Add/Update+Distribute+pricing三处也不足：tag/copy/upstream-sync/status、多个客户目录、WS第二create绕过。证据 `acceptance/p2/admission-surface-audit.md`。
- 最新fetch官方release仍rc.50，同tag检查只是基线祖先验证，**不是新release升级演练**。

## 当前需要的用户决策（不是逐小节确认）

R3桥接与R4精确工厂测试修复已获批准并通过。H1专项现已明确批准累计32文件/2500增删行、准入型多调用点与无业务core导入，规则v0.8；不授权资金重构。下轮直接按RFC A/B/C实施，不再重复征求相同预算。

资金同事务方案仍单独设计复核；H1审批不等于H3/H7审批或三库测试通过。

## 下一动作

1. `cmh-param-normalize`官方CLI lint与43/43 golden、真实CallTransform+patch通过。它会随官方整目录embed自动工厂注册，但未创建启用的请求实例；旧“不注册”描述已纠正。
2. R3五项Chrome表单通过、R4工厂测试完整通过。纯官方rc50字节精确隔离archive定向复验已完成：controller同名SQLite矩阵和middleware日志文件cleanup失败复现；relay HTTP2单次通过、未复现，不能笼统归环境。证据 `acceptance/p1/windows-baseline-reproduction.md`。relaykit/tokenkit各 `GOWORK=off go build -p 2 ./...` exit0。
3. 用户已明确批准H1专项累计32文件/2500增删行、准入型多调用与无业务core接口导入；宪法v0.8已单独提交，RFC状态已更新。`ext/core/admission.go`与`ext/domestic/gate.go`纯值合同/不可变策略最终plugins+ext测试、vet、门禁全部exit0；未注册core.Models，不宣称live防线完成。下一轮从RFC A的候选/目录权威入口开始实做，补齐公开/上游/实际执行者投影，逐文件登记，不重复要求预算确认。
4. P1先验证FundingSource工厂+tx-aware契约可行性；不用函数提前return跳过token/会话内部状态，不伪造缓存余额。
5. P2后续H1需完整旁路覆盖、模型公开名/最终上游/实际插件身份分开；审批32/2500不是覆盖完成，必须stub网络/预扣计数和三库验证。当前PATH无Docker/Podman/MySQL/PostgreSQL程序或相关服务，后续可准备独立本地引擎但不可借用生产DSN。

## 命令与环境

- Go1.26.10已缓存，调用使用临时 `GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct`。
- `go test -p 2 ./ext/... -count=1` exit0，`go vet ./ext/...`此前exit0。race未通过：CGO/C编译器缺失，不宣称通过。
- 控制台pnpm.cmd（历史bun缺失）；官方完整web使用portable exact Bun `ops/.cache/web-build-p0/bun-1.4.4/bun-windows-x64/bun.exe`，未全局安装。
- 官方原始rc50构建旧SHA在失败复现证据；当前R3桥接资源/测试二进制的完整SHA绑定在 `acceptance/p0/T0.2-bridge-pass.md`。两者都是实际production模式构建，不是占位页。后续正式发布必须重新冻结源码重建，不把本地测试exe当上线制品。
- 门禁目前9官方文件/110行，H1专项累计硬上限32/2500（已明确批准），逐文件白名单不变。未核对分支保护，CI成功不代表强制阻断merge。

## 临时资源与作业

- 全部本地gateway与独立Chrome已退出，后台build输出已收；本轮官方build由独立代理完成且其jobs已收。
- `%TEMP%/cmh-gateway-ui-acceptance.exe`和旧proto.exe为本任务测试二进制，后续验收仍需，不删除。
- `%TEMP%/cmh-p0-*.db`及独立Chrome profile为合成资料，禁止提交；清理前核对精确绝对路径与进程，不做广泛匹配删除。
- 官方Web依赖/构建缓存 `web/node_modules`、`web/dist`、`ops/.cache` 为本轮所建，后续UI验收仍需，保留。
- 未提交文件：纯白名单、插件与证据/RFC、新UI验收脚本；提交时精确路径，不 `git add -A`扫数据库/缓存。

## 禁止误报

不访问旧生产/不迁客户数据/不做付费上游；不把纯Policy单测称为live白名单；不把CLI插件fixture称为已激活；不把同tagAlready-up-to-date称同步验收；不把API登录或手动刷新称为官方表单自动跨应用成功。
