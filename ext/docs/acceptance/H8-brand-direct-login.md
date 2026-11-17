# H8 品牌域直接登录：本地验收记录（2026-11-17）

依据 `rfc/2026-11-17-brand-direct-login-boundary.md` 与 `rfc/2026-11-17-brand-auth-transaction-contract.md`。本记录只写实际执行过的命令和结果；**未执行项单列在最后，不计为通过**。实现默认关闭（`CMH_BRAND_AUTH=1` 才启用），未做任何生产操作。

## 范围

品牌域密码登录＋官方两步验证码/备用码、刷新、退出、客户本人最小资料页（`/login`、`/account`、`GET /api/ext/brand/self`）。品牌域注册、找回、OAuth、Passkey、访问令牌、模型调用入口保持关闭。仅该品牌的有效客户可登录。

## 官方区预算

`bash ops/ci/check-upstream-diff.sh`（基线 v1.0.0-rc.51）：

```
official-zone files changed (whitelisted): 49 / 49
H1 official-zone lines changed (whitelisted): 4641 / 6000
authentication files changed: 9 / 9
authentication lines changed from 17d70f116946f8099e977cfaa654d9590f96cc33: 353 / 1800
combined official files changed: 58 / 58
OK: official zone untouched except registered hooks.
```

## 自动化回归（SQLite 3.50.4）

`go test ./ext/brand/authtest/ -count=1` 通过。该套件走真实 gin 路由与官方 controller（Login / VerifyLogin / RefreshAuth / AuthLogout / UserAuth），覆盖：

- 开启后主站登录与主站会话照常可用；主站会话在品牌域被拒。
- 无绑定记录的既有会话被拒；`BackfillMainAuthBindings` 回填后可用，重复执行新增 0。
- 密码错误、非客户、其他品牌的客户三者响应体相同，且不产生 flow、session、刷新 Cookie。
- 品牌域 POST 缺少/错误 `Origin`、缺少浏览器 Cookie 均 403；注册入口 404。
- 品牌会话在主站、其他品牌、同品牌其他浏览器均 401；品牌域官方 `/api/user/self` 404；品牌域访问令牌 401。
- 越界刷新不旋转、不清 Cookie；越界退出（刷新 Cookie 与 Bearer 两条路径）不撤销；会话状态与刷新摘要前后一致。
- 撤销客户关系后既有会话与刷新均被拒，重新登录得到与密码错误相同的响应。
- 两步验证：品牌挑战只含 2FA；有效验证码从主站、其他品牌、其他浏览器提交均失败，且 `LastUsedAt` 为空、`FailedAttempts` 为 0（因子未被消费）；品牌域 Passkey 二级入口不可用。
- 注入“session 绑定写入失败”：无 session 行、flow 仍未消费、无刷新 Cookie（整体回滚）；随后正常完成，重放同一 flow 失败。
- 登录验证仅有 Passkey 的账号在品牌域登录失败，不产生 flow 与 session；同样状态的**非客户**得到与密码错误相同的响应（复核后新增）。

官方既有测试 `go test -p 2 ./model/ ./service/ -count=1` 通过。`./middleware/` 仅 `TestAccessAndPanicLogsFollowLogRotation` 失败；在未改动的基线（`git stash` 后）同样失败，属 Windows 下既有的日志轮转问题，与本改动无关。

## 真实浏览器（应用内置 Chromium）

环境：本地临时 SQLite 库，网关二进制由当前工作区构建；通过真实接口完成初始化、注册、建品牌、定向邀请与客户本人接受；`oem-migrate -backfill-main-auth` 输出 `main-site auth bindings added: 4`；之后以 `CMH_BRAND_AUTH=1`、`CMH_BRAND_PROTOTYPE_HOSTS=brand.localhost=<站点ID>` 启动。

- `/login` 显示数据库中的品牌名，样式与脚本经 `/brand-assets/` 加载，控制台无错误，CSP 为 `script-src 'self'`。
- 非客户账号（密码正确）：停留在 `/login`，显示通用失败提示，无会话提示 Cookie。
- 客户账号：进入 `/account`，显示账号、显示名称、所属品牌；网络请求依次为 encryption-key 200、login 200、refresh 200、brand/self 200；`localStorage` 与 `sessionStorage` 均为空，访问令牌仅在页面内存。
- 点击“退出登录”回到 `/login`；再访问 `/account` 被重定向到 `/login`。375 宽度下布局正常。

对同一运行实例的命令行检查（状态码）：

```
brand token, brand host + its browser cookie     200
brand token, brand host, other browser cookie    401
brand token, brand host, no browser cookie       403
brand token on main site /api/user/self          401
main token on main site /api/user/self           200
main token on brand host                         401
brand refresh cookie replayed on main site       401
brand logout attempted from main site            401
brand token still valid after those attempts     200
brand login with main-site Origin                403
brand register (closed)                          404
brand official /api/user/self (closed)           404
brand refresh on brand host                      200
```

## AI 只读复核（不是人工独立复核）

由一个未参与实现、只拿到代码与性质清单的 AI 代理做对抗式只读审查。结论：未发现越界凭据被接受或产生副作用的路径；事务、副作用顺序、Host/Origin 规范化、页面脚本与 CSP、关闭状态下的官方行为均核对无误。发现与处理：

| 发现 | 处理 |
|---|---|
| 中：非客户若账号仅有 Passkey、2FA 被锁定或会话数达上限，会得到与密码错误不同的响应（账号枚举） | 已修：新增 `model.AdmitAuthScope`，在 `setupLogin` / `setupLoginAtAuthVersion` 取得 scope 后、任何策略与限额判断前核对客户身份；补回归用例 |
| 中：品牌域刷新/退出仍经过官方 `SessionCookieOriginGuard`。`SESSION_COOKIE_SECURE=true` 且 TLS 在代理终止时，该守卫按 `http://` 比较而拒绝，除非把品牌 origin 加入全局 `SESSION_COOKIE_TRUSTED_URL`——这又使品牌 origin 对主站刷新/退出受信 | **未解决**：`middleware/auth_origin.go` 不在获批的九个文件内。上线前需负责人决定：追加文件例外，或接受加入全局受信列表（Cookie 为 host-only + SameSite=Strict，当前品牌页为固定内容，暂无已知利用路径） |
| 低：撤销客户后其会话行仍为 active，关系若被恢复可继续使用旧会话 | 已修：撤销接口成功后同时撤销该客户在该站点的品牌会话 |
| 低：关闭 H8 或多节点中某节点未开启时，品牌会话在该处被当作主站会话 | 约束已写入操作手册（关闭前 `-revoke-brand-sessions`、全部节点同时切换）；未做技术强制 |
| 低：登录/刷新响应体仍是官方完整本人资料（额度、分组等），只返回给本人，页面脚本不使用 | 未处理：需改官方响应构造，记为后续项 |
| 低：主站开启 Turnstile 时品牌登录被拒 | 未处理，手册已说明 |
| 提示：绑定表不清理；域名映射键带端口或末尾点号时不匹配 | 前者未处理；后者已修（键与查询同一规范化） |

## 未执行或被替代的验收（不计为通过）

- **本机官方 controller 测试无法给出干净的通过结论。** Windows 下大量用例因临时目录清理时 SQLite 文件被占用而失败，未改动的基线同样失败；在独立基线工作树（HEAD `26c3511d7`）与当前工作区各跑一遍 `go test ./controller/ ./service/ ./model/ -count=1`：失败条目均为 111 条且集合完全相同（无新增、无消失），model 与 service 均 ok。这只说明没有引入新的失败，不等于 controller 通过。完整的官方后端测试须在 CI（Linux）上跑。

- **MySQL / PostgreSQL 未执行。** 本机没有这两种数据库实例。套件已支持 `CMH_AUTH_TEST_MYSQL_DSN` / `CMH_AUTH_TEST_POSTGRES_DSN`（须为专用空库），未配置时为 skip。按官方规则，在两库实跑通过前不得声称本改动三库兼容或完成；`cmh_auth_scope_bindings` 的新建、升级与重复迁移同样只在 SQLite 上执行过。
- **TLS 为模拟。** 本地没有受信证书，浏览器经一个仅把 `Origin` 的 `http://` 改写为 `https://` 的本地反向代理访问 `http://brand.localhost`；`__Host-` 浏览器 Cookie 因 localhost 被视为安全上下文而生效。真实 HTTPS、代理信任边界、HSTS 未验证；本地 HTTP 下官方刷新 Cookie 不带 `Secure`（生产须保证 `SessionCookieSecure` 生效）。
- **浏览器内未走两步验证流程**（仅自动化套件覆盖）。
- **Redis 开启、多节点、`BATCH_UPDATE_ENABLED` 两种模式未验证。**
- **域名归属验证未实现**：品牌域映射仍来自环境变量。
- **人工独立复核未进行**：见下节，只有一次 AI 只读复核，不等同 ext/AGENTS.md §5.6 要求的未参与开发人员签字。

## OWASP 对照（Authentication / Session Management Cheat Sheet；ASVS 5.0 V6、V7）

已落实：登录失败统一响应；沿用官方密码校验、限流与两步验证，不降级；会话含义存于服务端并逐请求校验；登录成功后签发新会话；退出与撤销在服务端生效；浏览器 Cookie 为 `__Host-` 前缀、Secure、HttpOnly、SameSite=Strict、无 Domain；状态变更请求校验精确 Origin；页面 `Cache-Control: no-store`；日志不含密码、验证码、令牌。

未满足或未验证：全程 TLS 与 HSTS（见上）；品牌登录页未接入 Turnstile（主站开启时品牌登录被拒）；非客户与密码错误的响应时间未做等化测量；绑定表无缓存、过期行无清理任务。
