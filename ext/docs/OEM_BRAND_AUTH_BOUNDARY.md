# OEM 品牌认证接入调查（尚未实施）

## 已核对的事实

- 旧 `D:\work2\gnzz\middleware\brand.go:41-69` 从请求 Host 查询已验证域名，并核验品牌、所有者与 TLS 就绪；未知域名不进入主站。旧 `D:\work2\gnzz\controller\brand.go:699-764` 先认证客户/团队/所有者，再核对用户、品牌和 session version 后建立会话。这是旧产品行为描述，不表示该实现可直接复制。
- 新 `ext/brand/tenant.go:62-86` 当前仅把环境配置的 Host 解析成上下文 ID，允许 GET/HEAD，不是域名所有权或 TLS 就绪证明。若直接放行官方登录、注册、刷新等路径，将越过现有品牌域隔离。
- 新 `controller/user.go:53-100` 官方密码登录复用加密开关、用户验证及共享多因素策略，`controller/user.go:146-195` 验证后创建官方会话；复制其登录逻辑会违反复用和安全要求。
- 新 `router/api-router.go:75-87` 将登录、MFA、passkey 与刷新分为多个精确路径，并配有官方限流、body 限制、Turnstile 或 Origin Guard。不是只开放一个 POST 就能覆盖完整登录。`service/auth_session.go:325-342` refresh Cookie 为 host-only、HttpOnly、SameSite Strict，Path 为 `/api/user/auth`。
- 新 `middleware/auth.go:161-176` 可区分真实浏览器会话与 PAT；`ext/brand/customers.go:236-252` 返回客户关系（包括 inactive），调用者必须在每个请求明确核验 site ID 和 active。`ext/brand/team.go:45-85` 是商家 Admin 只读协作权限，并不自动等于旧站普通员工角色。

## 必须解决的工程问题

1. **可信域名**：环境映射只能用于本地原型；正式域名需绑定品牌、验证控制权、启用/停用与 TLS 就绪，并明确未知 Host 的默认处理、反向代理信任边界和回收。不能用 query、body 或 `X-Forwarded-Host` 选择品牌。
2. **认证与授权分层**：先由官方流程完成登录和 MFA，再以已解析品牌及实时 active 客户关系做服务端对象授权；不能把成功登录全球账号等同于获得任意品牌权限。所有客户数据 API 均须重查关系，撤销仅保证后续请求拒绝，不保证已在途响应被收回。
3. **凭据作用域**：放行官方 refresh Cookie 时须验证 Path、host-only 与 SameSite 行为；若 allowlist 上存在其他官方 UserAuth API，普通全局会话可能访问非品牌资源。需逐路径精确审计并保留官方限流、验证码、加密和 MFA，禁止宽前缀放行。
4. **用户体验与现存差异**：旧站支持客户、团队、商家统一登录以及注册/恢复；当前仅有客户邀请关系和限 Admin 只读协作，不能宣称角色、注册、恢复或 relay/API Key 隔离已等价。
5. **测试**：在隔离的合成环境中证明 A/B 品牌跨访问拒绝、未归属/已撤销/禁用用户拒绝、会话/PAT 区别、验证码与 MFA 不绕过、Cookie/Origin/登出、直接访问被禁止 API、服务重启以及真实 SQLite/MySQL/PostgreSQL 迁移与故障；不得使用旧数据或生产账号。
6. **反向代理与 Origin 的额外约束**：`middleware/auth_origin.go:18-34` 在 `SessionCookieSecure=false` 时明确不检查 Origin，不能用于正式品牌站。开启时 `isAllowedSessionOrigin` 仅从 `Request.TLS` 判断 HTTPS，不信任客户端 `X-Forwarded-Proto`；TLS 若在代理终止，后端看到 HTTP 的请求同源比较不成立，仍可能通过 `common.SessionCookieTrustedURLs` 全局信任列表。`common/session_cookie.go:44-83` 要求 secure 模式设置至少一个 HTTPS `SESSION_COOKIE_TRUSTED_URL`。正式方案须验证各品牌同源匹配、代理到后端的信任边界及全局信任列表对跨品牌请求的影响，不能简单把所有品牌源加入列表并声称隔离；现阶段未开放这些路径。

## 当前回归（非登录验收）

扩展 `TestHostIsolation` 在 H5 之前实际注册了四个官方认证路径的合成处理器，再验证品牌 Host 上官方密码登录、刷新、注册和 MFA verify POST 均为 404；主站同一登录路径返回处理器的 200，证明不是未注册路由产生的 404。主站请求携带伪造 `X-Forwarded-Host` 仍由真实 Host 决定且不切换品牌。最终 `go test -p 2 ./ext/brand -run '^TestHostIsolation$' -count=1` 通过（5.609 秒），`go vet -p 2 ./ext/brand` 通过。这验证当前关闭边界，不证明域名所有权、可用登录或生产代理配置。

附加双向对照：品牌 Host 携带伪造主站 `X-Forwarded-Host` 请求已注册登录 POST 仍为 404；测试后恢复 Host 映射和 Gin 模式。`go test -p 2 ./ext/brand -run '^TestHostIsolation$' -count=2` 通过（5.592 秒），vet 和 diff-check 通过。这不代表生产代理信任链已验证。

本调查未修改认证代码、未授权生产域名或实际登录，当前预览继续拒绝所有 POST。OWASP 指引：[Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)。尚无完整 ASVS 验证。
