# OEM 品牌档案：后端阶段验收

日期：2026-11-16。目标仍为完整可用 OEM 后台第一版，本记录只覆盖品牌档案链，不是整版交付或生产上线证明。

## 已落地

- 扩展独立表 `cmh_oem_sites`、`cmh_oem_site_audits`，不增加官方列。
- 显式迁移命令 `go run ./ext/brand/cmd/oem-migrate -engine sqlite|mysql|postgres`，DSN 从 `CMH_OEM_MIGRATION_DSN` 读取；不注册 H6、不在请求执行 DDL、不打印连接串。
- 官方浏览器会话、Casbin `cmh_oem` read/write、本人品牌所有权三层检查；无独立密码/角色数据库。
- 创建、读取、版本化保存、审计列表；保存与审计同事务，返回事务内的本次版本快照。
- 严格输入限制、跨所有者 404、旧版本 409、安全不可用错误，不暴露 SQL。

## 实际验证

| 验证 | 实测结果 |
|---|---|
| 官方会话与真实 Casbin | 非 root Admin 种子读写、显式拒绝/恢复/再次撤销；普通用户不升权；匿名/禁用账号拒绝 |
| 所有者隔离 | 第二 Admin 对他人详情、更新、审计均 404，原记录及审计不变 |
| 严格 JSON | 未知 owner 字段、重复/转义重复、大小写别名、null/数组、超长正文 400，资料与审计零变化 |
| PAT | 个人访问令牌 GET/POST 均 403，资料与审计零写入 |
| 事务与持久化 | 新建迁移两次、带数据迁移两次、独立连接重读、旧版本冲突、审计表失效时更新回滚 |
| SQLite 3.50.4 | 持久化子矩阵 0.06s |
| MySQL 8.0.44 | 持久化子矩阵 0.13s |
| PostgreSQL 17.12 | 持久化子矩阵 0.28s |

真实三库运行命令为忽略区脚本 `powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-oem-matrix.ps1`，内部执行 `go test -p 2 ./ext/brand -run '^TestOEMSitePersistenceAndOwnership$' -count=1 -v`，包 6.058s，三种引擎均实际执行而非 skip。专属 loopback fixture，仅清理两张 OEM 表；数据库引擎正常停止，作业已收齐。不是最低支持版本全覆盖，也不是三库官方认证链覆盖。

加入 PAT 补测后的 `go test -p 2 ./ext/brand/... -count=1` 6.066s、相关 vet、diff-check 全通过。此前真实 Casbin 补测 6.107s。完整网关 `go build -p 2 ./...` 通过；前端生成最终构建后仍需再次构建用于页面验收。

## 独立复核与修正

普通只读复核未见直接越权/审计半提交；发现原测试仅 root 不能证明 Casbin 种子，以及提交后重新查询会被后续版本覆盖。已分别补真实非 root 授权撤销测试、改为事务内读取提交快照。专用 Security Review 路由不可用，不冒称专用审计或完整 ASVS 合规。

适用参考：[OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)。复用官方认证，不改变密码、刷新或会话协议；新增接口服务端鉴权、失效验证、审计无凭据。

## 前端集成证据（不等同实际浏览器）

品牌页面 `/cmh/brands` 已实现列表、创建、编辑、冲突重新加载及审计。React Hook Form 7.86.0 / Zod 4.4.3 / resolvers 5.9.1，复用官方导出 Field/Input/Button 等组件。最终 26/26 客户端测试、typecheck、lint（零错误警告）、format-check、生产构建全部通过，产物 640.6kB / gzip 206.8kB。

主代理实际读实现发现第一版把完整 Site 直接当表单输入，导致 id/时间戳等未知字段进入更新请求并被严格 API 拒绝。已修为所有选择/保存/重载处显式三字段投影，API 创建/更新再独立投影，补 extended-Site wire 回归。初稿 native form 已改为项目要求的 RHF/Zod。邮箱精确语法由服务端 mail.ParseAddress 判断，不承诺客户端语法等价。

以上客户端测试使用 mock network；真实 Chrome 会话刷新、表单、wire、双窗口冲突证明正在独立执行，完成前不将构建通过称作页面可用验收。

## 真实 Chrome 品牌链验收

加固后独立运行 8 项检查全部通过，exit 0。全新 SQLite、合成非 root Admin，官方 CreateLoginSession → HttpOnly refresh cookie → 官方 refresh → 内存 Bearer；服务器明确 HOST=127.0.0.1，清空 operator manifest 环境，启动 cwd 为新隔离目录避免读取仓库 dotenv。没有旧数据、生产凭据、模型令牌或渠道。

实际表单 POST 200 仅含 description/name/support_email；保存 PUT 200 为三字段加 version=1。整页刷新后可重新读取；审计显示创建和更新。同源 iframe 的第二个独立 React 文档保留旧版本，实际 PUT 409，未覆盖已保存值。CDP 暂停真实详情 GET 响应时，新建/选择/关闭/保存/全部三个输入框禁用；原样释放响应后恢复，关闭先前发现的异步覆盖问题。没有伪造响应正文。刷新 cookie 不可脚本读取，Web Storage 未存 JWT。

二进制 SHA256 `654cf5c5070761da9825e92ba4426c6036dee93ddcf2857d58dda0e499f63395`；品牌页面 SHA256 `549649647fd7f702ad3618dfec2f3e2699267bb29aae7746505c6e5a3a863473`；嵌入 index SHA256 `779d8fc686eb4732b433b2f2d2af958c8f345bc27debe44580af10dbe6f7a936`。原始脱敏结果位于忽略区 `ops/.cache/oem-browser-1794807970693/results.json`。清理复核 exit 0，fixture 进程为 0，HTTP 38189/CDP 39389 均关闭。

该证明绑定已构建品牌 bundle；运行时客户增量已修改 API 源文件，不能把当前 API 源 SHA 当作该旧 bundle 源 SHA，也不能据此称客户 UI 已验收。第二文档不是第二 Chrome tab；合成会话注入不等价真人密码/MFA 登录，不证明生产 TLS。

## 尚未完成

- 客户/目录增量完成后重新构建、整版浏览器回归及发布验证；品牌阶段真实浏览器链已通过。
- 客户归属/团队/目录及完整工作台导航。
- 品牌档案不代表域名生效、账号开通或网关请求租户隔离；这些不能用 CRM 字段替代。
- 资金 H3/H7、H4/H6、旧数据迁移、生产域名、付费模型调用仍未授权。
- 官方没有 nil enforcer reset，测试清理在旧 DB 为 nil 时留下空 fail-closed 策略快照，不声称恢复 nil。
