# RFC（已批准）：跨应用登录返回边界

状态：**负责人于 2026-11-13 明确选择“允许最小桥接例外（推荐）”**。仅批准本 RFC 的 R3 边界，例外同步登记于 `ext/AGENTS.md` §2.5/§6.1 与 `ext/docs/HOOKS.md`；其他官方区只读规则不变。

## 已复现的问题

官方 rc.50完整前端 `NODE_ENV=production` 构建，1532个官方跟踪文件哈希均不变。新建loopback空SQLite实例，独立真实Chrome：匿名 `/cmh/whoami` → 官方 `/sign-in?redirect=%2Fcmh%2Fwhoami` → 实际填写用户名密码、点官方提交按钮 → 登录成功、URL变为 `/cmh/whoami` → **页面仍由官方SPA持有，显示404；控制台未加载**。验收脚本等待30秒仍失败，不能以URL正确报通过。

源码原因：`web/src/features/auth/hooks/use-auth-redirect.ts:66-68` 使用 `navigate({href:targetPath,replace:true})`，相对路径为SPA跳转；官方路由没有 `/cmh`。已登录打开sign-in的beforeLoad另有同类分支 `web/src/routes/(auth)/sign-in.tsx:43-46`。API Cookie→Bearer复用正确，问题不是鉴权失败，不能改服务端会话来“修”。

## 已批准边界

允许**一个极小的前端跨应用导航边界补丁**：经过官方sanitizer后，目标为精确 `/cmh` 或 `/cmh/` 下路径时，使用document navigation（TanStack `reloadDocument`），其余官方路径行为保持不变。包括普通登录成功与已登录sign-in回跳两分支。不得接受任意外域，不携带Token到URL，不改变登录/MFA/密码逻辑。

非侵入方式均无法修复该分支：任务插件不运行于浏览器路由；`ext/` 独立实现或 H5 服务端路由只能在浏览器真实请求时加载应用，不能拦截官方 SPA 已消费的 navigate；H2 只处理服务端 Host/context，不控制前端导航。因此复用既有 `auth-redirect.ts` 导出一个返回安全 href 和 document 标志的微型 helper，两个调用方共享，避免扩展 import 或复制官方逻辑。

R3 登记四个既有官方文件：`auth-redirect.ts`、`use-auth-redirect.ts`、`routes/(auth)/sign-in.tsx`、既有 `auth-redirect.test.ts`，实现和测试均计入150行/10文件预算。没有新增官方文件、路由、UI/i18n 文案或依赖。同步时须重新核对 sanitizer 契约、两个调用方及 TanStack document navigation 行为。

## 不选此建议的替代

保持官方前端绝对只读：新应用需要自己的登录壳，复用官方认证流程/组件并在自身路由内完成登录；需重新验全部登录方式、MFA/OAuth/回跳，范围更大。或用户手动回到 `/cmh/`，但这不满足当前P0自动回跳验收，不能称完成。

## 必需验收

- 未登录控制台→实际官方登录表单→自动加载控制台并显示当前账号；刷新与退出仍正常。
- 已登录sign-in带CMH目标的beforeLoad分支也document navigation。
- 普通dashboard回跳不产生行为变化。
- `//evil`、反斜杠、javascript协议、外域、`/cmh-evil`不触发桥接或被官方sanitizer拒绝。
- 不改官方署名；保留源码提供义务；完整构建、前端类型/lint/相关回归与真实浏览器复验。

## 安全依据与验证（2026-11-13）

实施前阅读 [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[Unvalidated Redirects and Forwards Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Unvalidated_Redirects_and_Forwards_Cheat_Sheet.html)；[ASVS 当前稳定版 5.0.0](https://asvs.owasp.org/)（[要求 CSV](https://raw.githubusercontent.com/OWASP/ASVS/v5.0.0/5.0/docs_en/OWASP_Application_Security_Verification_Standard_5.0.0_en.csv)）适用控制：`v5.0.0-1.2.2` 安全 URL 协议、`v5.0.0-3.7.2` 自动跳转目的地允许列表、`v5.0.0-14.2.1` 不在 URL 携带会话凭据。继续使用官方 URL parser/same-origin sanitizer，不二次 decode；只对已规范化的路径做 `/cmh` 边界匹配。此补丁不新增凭据、日志或 Cookie 操作，不代替后端认证/授权，不改变失败/MFA challenge/过期分支。

真实执行（进程 PATH 前置 `ops/.cache/web-build-p0/bun-1.4.4/bun-windows-x64`，Bun 1.4.4，未安装或改依赖）：

- 先运行新回归：`bun run test src/features/auth/lib/auth-redirect.test.ts`，实现前 12 个新用例失败、原 6 个通过；实现后通过。最终补入 root query/hash 和编码斜杠边界，现有文件共 20 个用例。
- `bun run test src/features/auth src/lib/__tests__/auth-session.test.ts`：实际命中的 auth 模块 10 文件 / 99 测试通过（含 OTP、pending MFA challenge、verification、OAuth/Passkey）；后一个过滤路径未匹配额外文件，不将其计为已测。
- `bun run test src/lib/auth-session.test.ts`：另行运行实际 session resolver 测试文件，10/10 通过，覆盖 beforeLoad 依赖的会话解析行为。
- `bun run typecheck`：1,488 文件 / 2 项目无类型错误。
- `bun x oxlint -c .oxlintrc.json` 后接本次四个官方文件：0 warning / 0 error。
- 四文件移除受保护头的内存副本经 `oxfmt --stdin-filepath` 检查，与输入逐字一致；未写回或改动任何受保护头。
- `NODE_ENV=production bun run build`：Rsbuild 2.1.6，7.98s 成功，产物 `web/dist` 已重建（67,803.3 kB / gzip 20,839.8 kB）。
- `ops/ci/check-upstream-diff.sh`：基线 `v1.0.0-rc.50`，官方区 **8/10 文件、97/150 行**（新增 R3 四文件 59 行，含测试）；`git diff --check` 通过。

浏览器复验：父任务已重建当前 production embed，真实 Chrome 完整官方表单五项全部通过：`AUTOMATIC_CROSS_APP_RETURN`、`AUTHENTICATED_SIGNIN_RETURN`、`CONSOLE_RELOAD_SESSION`、`LOGOUT_RETURNS_REAL_SIGNIN`、`OFFICIAL_DASHBOARD_STAYS_SPA`。普通 dashboard 路径经官方重定向到 `/dashboard/overview`，文档内存标记仍存在，证明未改成 document navigation。详见 `ext/docs/acceptance/p0/T0.2-bridge-pass.md`。独立代码复核未发现阻断性桥接问题，未单独复跑浏览器；MFA相关单测通过不代表已进行真实MFA到CMH的Chrome验收。不把单测/构建当作浏览器验收，未进行整个系统ASVS认证或全套会话审计。同源双应用共享官方会话是既定部署边界，不宣称满足全部L2隔离要求。
