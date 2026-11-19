# 品牌域刷新/退出与官方来源守卫的冲突（待负责人决策）

状态：待决策。本文只描述问题、三种处理方式及各自代价，不预设结论；未批准前不修改官方文件。

## 问题

官方 `middleware/auth_origin.go` 的 `SessionCookieOriginGuard` 挂在 `POST /api/user/auth/refresh` 与 `POST /api/user/auth/logout`（`router/api-router.go:77-78`）。`SESSION_COOKIE_SECURE=true` 时它要求请求 `Origin` 等于"请求自身的 origin"或全局 `SESSION_COOKIE_TRUSTED_URL` 列表中的一项。"请求自身的 origin" 由 `request.TLS != nil` 决定 scheme（`auth_origin.go:60-64`），不读 `X-Forwarded-Proto`。

品牌域的部署形态是前置反向代理终止 TLS、以 HTTP 转发到网关（操作手册 §9 的 Caddy 示例）。于是网关看到的请求 origin 是 `http://brand.example`，浏览器送来的 `Origin` 是 `https://brand.example`，两者不等，守卫返回 403 `AUTH_ORIGIN_FORBIDDEN`。品牌域登录本身不受影响（登录路由没有这个守卫），但刷新与退出不可用，表现为登录后约一个 access token 周期即被登出，且无法主动退出。

H8 的 H2 解析器已经对品牌域做了更严格的检查：登录、刷新、退出的 `Origin` 必须精确等于 `https://<品牌域>`（`ext/brand/auth_scope.go` `brandAuthScope`），否则 403。因此官方守卫在品牌域上是重复检查，而且因为 scheme 推断方式与代理形态不符而误拒。

`SESSION_COOKIE_SECURE=false` 时守卫不检查 `Origin`，问题不出现，但生产不能用该模式。

## 已排除的非侵入方式

1. **扩展区绕过守卫**：H2 在守卫之前运行，可以 `c.Abort()` 但不能跳过后续链中的某一个中间件再继续官方 handler；Gin 没有"跳过下一个"的原语。在 H5 另注册一条品牌专用刷新/退出路由等同于复制官方认证入口，被 `rfc/2026-11-17-brand-direct-login-boundary.md` 明确禁止。
2. **让网关自己终止 TLS**：使 `request.TLS != nil` 成立。与 §9 "网关不终止 TLS、证书由代理按需签发"的设计相反，且主站同样经代理部署时也不会这么做。
3. **把品牌域加入 `SESSION_COOKIE_TRUSTED_URL`**：可以让品牌域通过守卫，但该列表是全局的：品牌 origin 同时对主站的刷新/退出受信，反之主站也在列表里所以对品牌受信。不过 H2 对品牌 host 的精确 `Origin` 检查仍然生效，所以"主站 origin 向品牌域发刷新"仍被 H2 拒绝；真正放宽的只是"品牌 origin 向主站发刷新"。主站刷新还要求携带主站 host-only 的刷新 Cookie，品牌页面脚本没有它，浏览器也不会跨 host 附带。域名自助接入后品牌域是动态的，这个列表无法预先配置，这是方案 3 的硬伤。

## 三种处理方式

| 方式 | 改动 | 代价 |
|---|---|---|
| A. 官方文件例外（推荐） | 把 `middleware/auth_origin.go` 列为 H8 第十个文件，在 `SessionCookieOriginGuard` 入口加一处"请求 scope 为品牌且 H8 已对该请求做过精确 Origin 校验则跳过"的判断，约 6–8 行；需要 `ext/core` 提供"scope 已校验 Origin"的只读标记 | 认证专项文件数 9→10；行数在 1800 预算内；需先走 rules PR 更新 `ops/ci` 门禁与 `HOOKS.md`；下次同步官方多一个潜在冲突文件 |
| B. 接受全局受信列表 | 不改代码；运营在启动参数里把每个已验证品牌域写入 `SESSION_COOKIE_TRUSTED_URL`，每新增品牌域需重启全部节点 | 与域名自助接入"无需重启"矛盾；列表增长无上限；品牌 origin 对主站受信的放宽虽有 H2 和 host-only Cookie 兜底，但是一个不必要的面 |
| C. 品牌域不提供刷新/退出 | H2 从品牌白名单去掉这两条路径；access token 到期即需重新登录；退出改为前端清内存 | 体验明显差于旧系统；"退出"不撤销服务端会话，不符合会话管理要求；仍需说明为何品牌会话不能撤销 |

## 推荐 A 的具体形态（供审阅，不是实现）

- `ext/core/auth_scope.go`：`AuthScope` 增加只读方法 `OriginVerified() bool`，仅品牌 scope 由 H2 在精确 `Origin` 校验通过后置真；主站 scope 恒假。
- `middleware/auth_origin.go`：

  ```go
  if scope, err := core.RequestAuthScope(c.Request.Context()); err == nil && scope.OriginVerified() {
      c.Next() // H2 already required Origin == https://<brand host> for this request
      return
  }
  ```

  放在 `SessionCookieSecure` 判断之后、`requestBrowserOrigin` 之前。主站请求没有该标记，行为不变；H8 关闭时没有品牌 scope，行为不变。
- 测试：在 `ext/brand/authtest` 补两例：开启 `SessionCookieSecure` 且 `request.TLS == nil` 时，品牌域刷新/退出带精确 `Origin` 为 200，带错误 `Origin` 仍 403（由 H2 拒绝）；主站同条件下不带受信 `Origin` 仍 403（官方守卫不变）。

## 需要负责人回答

1. 选 A、B 还是 C。
2. 选 A 时，是否接受认证专项文件数由 9 扩到 10（行数预算不变），并由谁提 rules PR。
