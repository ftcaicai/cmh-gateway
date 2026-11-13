# RFC（待用户确认）：跨应用登录返回边界

状态：**待批准，未改官方前端**。与现行 `ext/AGENTS.md` §2.5/§6.1 冲突，不能按“继续推进”自行豁免。

## 已复现的问题

官方 rc.50完整前端 `NODE_ENV=production` 构建，1532个官方跟踪文件哈希均不变。新建loopback空SQLite实例，独立真实Chrome：匿名 `/cmh/whoami` → 官方 `/sign-in?redirect=%2Fcmh%2Fwhoami` → 实际填写用户名密码、点官方提交按钮 → 登录成功、URL变为 `/cmh/whoami` → **页面仍由官方SPA持有，显示404；控制台未加载**。验收脚本等待30秒仍失败，不能以URL正确报通过。

源码原因：`web/src/features/auth/hooks/use-auth-redirect.ts:66-68` 使用 `navigate({href:targetPath,replace:true})`，相对路径为SPA跳转；官方路由没有 `/cmh`。已登录打开sign-in的beforeLoad另有同类分支 `web/src/routes/(auth)/sign-in.tsx:43-46`。API Cookie→Bearer复用正确，问题不是鉴权失败，不能改服务端会话来“修”。

## 建议（选择前需用户确认）

允许**一个极小的前端跨应用导航边界补丁**：经过官方sanitizer后，目标为精确 `/cmh` 或 `/cmh/` 下路径时，使用document navigation（如TanStack reloadDocument），其余官方路径行为保持不变。包括普通登录成功与已登录sign-in回跳两分支。不得接受任意外域，不携带Token到URL，不改变登录/MFA/密码逻辑。

需要把这两处文件作为“前端桥接例外”登记，而不是声称 `web/` 从未改过。最终真实diff与150行/10文件总预算一起验，不能事前承诺必定够；若超预算另行确认。

## 不选此建议的替代

保持官方前端绝对只读：新应用需要自己的登录壳，复用官方认证流程/组件并在自身路由内完成登录；需重新验全部登录方式、MFA/OAuth/回跳，范围更大。或用户手动回到 `/cmh/`，但这不满足当前P0自动回跳验收，不能称完成。

## 必需验收

- 未登录控制台→实际官方登录表单→自动加载控制台并显示当前账号；刷新与退出仍正常。
- 已登录sign-in带CMH目标的beforeLoad分支也document navigation。
- 普通dashboard回跳不产生行为变化。
- `//evil`、反斜杠、javascript协议、外域、`/cmh-evil`不触发桥接或被官方sanitizer拒绝。
- 不改官方署名；保留源码提供义务；完整构建、前端类型/lint/相关回归与真实浏览器复验。

当前只保存RFC和复现证据，不把建议写为已批准规则。
