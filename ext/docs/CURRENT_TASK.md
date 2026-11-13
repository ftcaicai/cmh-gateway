# 连续推进检查点

更新时间：2026-11-13；范围 P0→P1→P2。用户授权阶段级连续推进，遇到规则/架构/费用/生产凭据决策才停。

## 当前事实

- 已推 `b5a2864c8`：独立控制台、会话客户端、静态资源与品牌兜底修复；远程main/develop一致；此前CI在ab4c69bee两次success。
- P0：20前端测试 +23 Go回归子场景 +13HTTP +7真实Chrome会话项通过。
- 用户已批准R3最小官方前端回跳例外（ext/AGENTS v0.7）；真实官方生产表单复验5项PASS：匿名自动回CMH、已登录beforeLoad回CMH、重载、退出、普通dashboard仍SPA。证据 `acceptance/p0/T0.2-bridge-pass.md`。旧失败证据保留。
- 官方相关前端99+10测试、类型1488文件/2项目、4文件lint与production构建均通过；独立只读R3安全复核无阻断问题。门禁8文件/97行。
- 全量Go回归exit1：controller/middleware Windows占用文件cleanup；relay/channel HTTP2错误文案；plugins固定清单17vs18由本次cmh插件引入，不能忽略。详见 `acceptance/p1/full-backend-windows-regression.md`。
- P1：完整生命周期独立只读复核推翻“四个唯一出口”。追加预留、Rollback、直接PostConsumeQuota、Realtime/违规/MJ、token更新、后台任务重启身份都不在旧假设覆盖内。未实施任何资金钩子；用户同事务选择保持，不自动改成替代钱包或双扣最终一致。证据 `acceptance/p1/funding-lifecycle-audit.md`；设计RFC `rfc/2026-11-13-funding-transaction-boundary-review.md`未批准。
- P2：`ext/domestic`纯不可变fail-closed策略实现并测试通过；未接H1、未激活live gate。独立审查发现Add/Update+Distribute+pricing三处也不足：tag/copy/upstream-sync/status、多个客户目录、WS第二create绕过。证据 `acceptance/p2/admission-surface-audit.md`。
- 最新fetch官方release仍rc.50，同tag检查只是基线祖先验证，**不是新release升级演练**。

## 当前需要的用户决策（不是逐小节确认）

R3最小前端例外已批准并完成5项真实Chrome表单验收；R4既有官方测试精确清单修复也已批准，加入有效自有icon后 `go test -p 2 ./plugins ./ext/...` 全通过。当前官方预算9文件/110行，剩1文件/40行；不能暗示后续完整H1/资金扩展能自动放进预算。

资金/白名单原设计仍待RFC完善，不能提前宣称150行预算可行。

## 下一动作

1. 插件代理 `4a627187-a387-4309-aa3f-e4c800618afd` 已完成：官方CLI lint通过，43/43 golden通过，真实CallTransform+补丁应用测试通过；默认无scope/开关全false，不自动注册或激活。父代理复跑全ext测试及CLI通过。
2. R3回跳已经批准并完成五项真实Chrome表单验证；下一步处理工厂插件清单官方测试登记（需用户确认），并在纯官方基线复验Windows失败，不能笼统归为环境错误。
3. P1先验证FundingSource工厂+tx-aware契约可行性；不用函数提前return跳过token/会话内部状态，不伪造缓存余额。
4. P2后续H1需完整旁路覆盖、模型公开名/最终上游/实际插件身份分开；150行/10文件预算未证实，不能增文件白名单把实现绕过去。

## 命令与环境

- Go1.26.10已缓存，调用使用临时 `GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct`。
- `go test -p 2 ./ext/... -count=1` exit0，`go vet ./ext/...`此前exit0。race未通过：CGO/C编译器缺失，不宣称通过。
- 控制台pnpm.cmd（历史bun缺失）；官方完整web使用portable exact Bun `ops/.cache/web-build-p0/bun-1.4.4/bun-windows-x64/bun.exe`，未全局安装。
- 官方真实构建web/dist/index.html SHA256 AAA9C1A4B062FC05A6DDE3FA5006DCCC73911FAB3BEE4333F9462B8EE8B1D14B；entry JS index.112f5f678d.js SHA256 BD0DFFE7C917D577EE894327B3E37FAE45C3CE5C0F761239100E2D4AB3B0CE38。真实资源不能当临时占位随意删。
- 门禁目前4官方文件/38行，exit0；未核对分支保护，CI成功不代表强制阻断merge。

## 临时资源与作业

- 全部本地gateway与独立Chrome已退出，后台build输出已收；本轮官方build由独立代理完成且其jobs已收。
- `%TEMP%/cmh-gateway-ui-acceptance.exe`和旧proto.exe为本任务测试二进制，后续验收仍需，不删除。
- `%TEMP%/cmh-p0-*.db`及独立Chrome profile为合成资料，禁止提交；清理前核对精确绝对路径与进程，不做广泛匹配删除。
- 官方Web依赖/构建缓存 `web/node_modules`、`web/dist`、`ops/.cache` 为本轮所建，后续UI验收仍需，保留。
- 未提交文件：纯白名单、插件与证据/RFC、新UI验收脚本；提交时精确路径，不 `git add -A`扫数据库/缓存。

## 禁止误报

不访问旧生产/不迁客户数据/不做付费上游；不把纯Policy单测称为live白名单；不把CLI插件fixture称为已激活；不把同tagAlready-up-to-date称同步验收；不把API登录或手动刷新称为官方表单自动跨应用成功。
