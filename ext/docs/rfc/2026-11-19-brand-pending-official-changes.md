# 品牌线剩余两项需官方区改动的事项（待负责人决策）

状态：待决策。与 `2026-11-19-brand-origin-guard.md` 同批提交给负责人；本文两项各自独立，可分别批准或搁置。未批准前不动官方文件。

## 1. 商家后台修改域名前的二次验证

**问题**：`PUT /api/ext/oem/sites/:id/domain` 只要求登录会话与 `cmh_oem:write`。旧系统在改域名前要求再次输入密码。一个被盗用的后台会话可以把品牌域指向攻击者控制的域名。

**已排除的非侵入方式**

- 扩展区自行校验密码（`common.ValidatePasswordAndHash` 对 `users.password`）：等于另建一套密码校验入口，没有官方的限流、锁定、审计与加密传输，违反 `ext/AGENTS.md` "不另建密码系统"和 OWASP 认证要求。
- 复用官方通用验证 `POST /api/verify` + `X-Security-Proof`：`service.GetVerificationRequirements` 与 `ConsumeOperationProof` 按 `scope` 走闭合的 `switch`（`service/security_verification.go:165-270`），未列出的 scope 返回 `ErrProofScope`。扩展区无法注册新 scope；借用已有 scope（如 `access_token.generate`）会让为该操作签发的 proof 可用于另一个操作，违反 proof 与操作绑定的设计。

**候选官方改动**：`service/security_verification.go` 增加一个可由扩展注册的 scope 前缀（例如 `ext.` 开头的 scope 走"任意已启用方法、无上下文"的默认分支），约 10 行；`access_token_scope.go` 的映射表同步加一条。之后扩展在 `setDomain` 前调用 `middleware.RequireSecurityProof(c, VerificationOperation{Scope: "ext.cmh_oem.domain"})`，后台复用官方 `web/src/features/security` 的验证对话框。涉及 2 个新官方文件，不在 H1 或 H8 名单内，需单独 rules PR。

## 2. 品牌域登录与刷新响应裁剪

**问题**：`POST /api/user/login`、`/login/verify`、`/auth/refresh` 在品牌域返回官方完整的本人资料（额度、分组、角色等）。页面脚本只读 `access_token` 与 `require_verification`，其余字段只回给本人，但品牌客户不应看到平台侧字段。

**已排除的非侵入方式**：响应由官方 controller 直接写出；H2 只能在响应前拒绝或在响应后观察，Gin 没有改写已写出正文的安全方式（包装 `ResponseWriter` 缓冲再改写属于对官方控制流的隐式接管，且需逐字段理解官方 DTO）。

**候选官方改动**：在 H8 已批准的 `controller/user.go` 与 `controller/auth_session.go` 中，构造登录/刷新响应处按请求 scope 判断：品牌 scope 只返回 `access_token`、`expires_in`、`require_verification`、`flow_token`，约 15 行，在现有 1800 行预算内，不增加文件。需负责人确认是否接受该预算消耗。

## 需要负责人回答

1. 第 1 项：批准新 scope 前缀的 rules PR，还是接受"改域名无二次验证"作为已知限制并写入手册。
2. 第 2 项：是否在 H8 既有文件内实施响应裁剪。
