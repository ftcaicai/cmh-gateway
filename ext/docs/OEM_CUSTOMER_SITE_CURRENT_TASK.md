# OEM 客户站对照与实施检查点

## 目标

按用户要求以本地 `D:\work2\gnzz` documented-377 源码和公开参考 `https://api.7q.com.cn/login` 推进真实 OEM 客户站；此前交付的 `/cmh/` 是商家管理工作台，不等于客户品牌站。保持旧系统、生产、资金及旧数据不访问、不迁移的边界。

## 已核实差异

- 旧 `brandweb/index.html` 独立品牌壳；`brandweb/site.js` 的 `authPage` 为客户、团队、商家提供一个登录入口，显示账号/密码、注册链接与找回入口，登录后根据身份进入不同页面。
- 旧 `middleware/brand.go` 通过已验证域名、品牌启用/TLS 状态定位品牌，限制路径与身份；`controller/brand.go` 的 BrandLogin 核验真实品牌归属与会话。不能把它替换成仅凭客户端的品牌标记，也不能将主站全局登录伪装成品牌隔离。
- 原先 `ext/brand/tenant.go` 仅有启动环境变量的 host→ID 原型，`ext/brand/routes.go` 只返回品牌占位首页；原 `/login` 为 404，品牌域阻断所有 POST 与官方 API。现有 `cmh_oem_sites` 只有名称、邮箱、描述和所有者，没有已验证域名、发布状态或品牌认证关系。
- 实际 Chrome 在公开参考页观察到顶部品牌/导航、蓝色欢迎区、右侧账号密码表单和注册/找回链接；未登录、不填写表单。该视觉观察不证明登录后的功能或线上规则。

## 当前工作

已在映射的品牌测试域名提供 `/` 与 `/login` 的蓝白响应式静态预览，按公开页面调整为白色顶部、浅蓝欢迎区与白色说明区；明确标注认证尚未开通，不含密码输入或可提交表单；未注册其他页面或 POST。最终布局版 `TestHostIsolation` 5.682 秒通过，`go vet -p 2 ./ext/brand` 与 diff-check exit 0；覆盖主站透传、品牌 GET/HEAD、登录/官方认证 POST 拒绝、官方 API/控制台隔离、伪造转发 Host 与 CSP。前一版 `go test -p 2 ./ext/... -count=1` 及完整 `go build -p 2 ./...` exit 0。尚未完成本地浏览器对照或真实认证，不将预览当作可用客户站。

下一步落实品牌域内直接账号密码登录。负责人已批准九具名官方文件/1800增删行有限认证专项，固定计量起点为 `17d70f116946f8099e977cfaa654d9590f96cc33`。独立规则 PR #4（https://github.com/ftcaicai/cmh-gateway/pull/4）在精确 head `3431516b2c2e393d11b5e3fdc969b3bfd3208827` 完整 CI 通过后正常合入：差异门禁 43984791167、后端/前端全测 43984790909（4m10s / 5m31s）均 success；merge `4e46ce80a35a0176c54312a1f621d649c0b29d91` 已同步 main/develop。门禁实际临时Git仓库回归通过旧预算边界、认证1800/1801、二进制拒绝、九文件及缺固定起点拒绝。H1仍49/6000独立计量，认证尚为0/1800；规则批准不等于已实现登录。独立审查代理失败，无结论，不计审查通过。下一步先形成可独立复核的无循环scope/同事务参与接口，再接线官方认证与三库/浏览器验证。现有客户关系不是 relay 或会话隔离证明，不再反复重跑静态预览代替实施。

## 未接线的认证基础（2026-11-17）

新增 `ext/core/auth_scope.go`：私有字段作用域、显式 Main 与 Brand 构造、缺失/零值拒绝而非回退主站；品牌 origin 要求规范 HTTPS、合法 DNS/IP 与端口，站点/浏览器摘要定长小写 hex；分别比较站点和浏览器。它只是受信任宿主值，不是域名验证或认证授权，尚未被 H2 或官方认证流程调用。集中回归 `go test -p 2 ./ext/core -count=1` 最终 0.701 秒、相关 vet/diff-check 通过；前一版本 core/brand 测试与整体 build 通过，不能把它当最终全认证验收。

事务接口候选另见 `rfc/2026-11-17-brand-auth-transaction-contract.md`：model 唯一提交者，扩展同步回调同 tx，flow 消费/session 与绑定整体回滚；HTTP 不得用旧 MainScope 包装器填补丢失 scope。补充 `AuthScopeBinding` 私有字段值：flow/session 分种类、精确记录和用户、品牌与浏览器匹配、到期即拒绝；构造不证明身份、记录真实性或域名控制权。集中值测试 0.684 秒、相关 vet 通过；集成 core 0.086 秒 / brand 8.284 秒及 vet 通过。尚无数据库绑定或官方接线，不能当事务验收。

合同已消除“查不到绑定就当历史主站”的歧义：候选启用流程要求维护窗口显式迁移历史主站记录，启用后缺失绑定拒绝；此迁移仍未实施，未批准生产执行。也明确官方因子使用发生在最终事务之前：错误 scope 必须在因子使用前拒绝；正确作用域的因子使用不承诺随后续绑定失败回滚。独立复核代理此前均失败无结论，新有限复核正在收回，未计作通过。认证专项官方 diff 仍为 0/1800。

## 参考安全要求

复用官方认证和会话，而不是复制旧凭据逻辑；适用 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 与 [Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)，包括 TLS、服务端授权、抗枚举/暴力攻击、会话失效和敏感信息不落日志。尚未声称 ASVS 全面符合。
