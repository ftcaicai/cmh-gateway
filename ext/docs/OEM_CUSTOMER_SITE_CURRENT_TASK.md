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

## H8 已接线（2026-11-17，默认关闭）

九个具名官方文件已接线，实测 353/1800 增删行，差异门禁通过。设计与候选合同一致：model 为唯一提交者，`model.AuthScopeParticipant`（Admit/Bind/Check）由 `ext/brand/auth_scope.go` 实现，flow/session 行与 `cmh_auth_scope_bindings` 同事务写入；H2 在请求 context 放入主站或品牌 scope（站点、精确 origin、host-only 浏览器 Cookie 摘要）；登录、两步验证、刷新、退出及全部浏览器身份中间件按 scope 核对，越界请求无副作用。品牌域只放行密码登录、两步验证码、刷新、退出、`/account` 与 `GET /api/ext/brand/self`；仅有效客户可登录。品牌页面移入 `brandweb/`。开启、回填与关闭步骤见操作手册第 8 节。

与候选合同的差异：身份核对接口为 Admit（登录策略前）并在 Bind（事务内）与每次 Check 中复核；启用开关是启动时环境变量而非“迁移就绪标记”，未回填就开启的后果是主站既有会话全部被拒（失败关闭）；关闭前必须运行 `oem-migrate -revoke-brand-sessions`，否则品牌会话会被主站接受——这是当前实现的已知约束，不是合同中“保留绑定读取拒绝”的等价实现。

验收状态见 `acceptance/H8-brand-direct-login.md`：SQLite 自动化套件与本地真实浏览器流程通过；**MySQL/PostgreSQL 未执行、TLS 为模拟、无人工独立复核**，因此本专项尚不能记为完成，也不得启用于生产域名。

## 参考安全要求

复用官方认证和会话，而不是复制旧凭据逻辑；适用 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 与 [Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)，包括 TLS、服务端授权、抗枚举/暴力攻击、会话失效和敏感信息不落日志。尚未声称 ASVS 全面符合。

## 客户站页面与商家后台产品化（2026-11-17）

品牌域新增首页、模型价格（`/pricing`，数据来自公开只读接口 `GET /api/ext/brand/pricing`）与接入文档（`/docs`），带统一导航；这些页面不依赖 `CMH_BRAND_AUTH`，未开启时登录页只显示“尚未开通”提示。模型价格显示 `default` 分组的标准价：按官方定义换算（倍率×2 美元/百万 token×分组倍率，或按次价×分组倍率），展示币种为 CNY 时乘以系统汇率，其余展示类型按美元；表达式计费或插件计费的模型只显示“按模型计费规则计价”。品牌专属价格（H4）不存在，页面不改变任何计费。接入文档不含平台主体与域名，接口地址与密钥由品牌客服提供——品牌域目前没有密钥管理与模型调用入口。

商家后台 `/cmh/` 接入官方同款样式（Tailwind 插件取自 `web/node_modules`，只读引用 `web/src/styles/index.css`），改为侧边导航布局；语言跟随主站已选语言。品牌档案列表显示可复制的站点 ID。

未验证：价格页只在空模型库上看过空状态，未用真实模型数据核对金额；页面截图因浏览器窗格不在前台未取得，只做了 DOM 与控制台检查。品牌域注册与企业空间未做，见交付说明。

## 域名自助接入（T2.5，2026-11-17）

每个品牌一个自有域名：所有者保存域名后得到专属 CNAME 目标 `<32位挑战>.<CMH_BRAND_CNAME_ROOT>`，公网 CNAME 精确匹配即验证通过；验证结果写入 `cmh_oem_site_domains.verified_host`（唯一索引保证一个域名只服务一个品牌）。H2 解析器读内存映射，不查库；映射在本节点变更时立即重载，其他节点通过官方同步主题 `cmh_brand_domains` 重载。待验证域名由官方系统任务 `cmh_brand_domain_verify` 每分钟检查一批（20 个、单次 DNS 5 秒、整轮 45 秒）。换域名或移除立即使旧域名失效并换新挑战；平台自有域与接入根域不可认领。证书由前置代理按需签发，网关只提供许可端点 `GET /api/ext/brand/tls-allowed`。商家后台品牌档案里新增“自有域名”面板。

已验证：SQLite 上的存储与解析回归（规范化、越权、缺失/不匹配/被占用、换域名即失效、定时批次、审计）；临时实例上经真实接口保存、检查（得到 `cname_missing`）、保留域拒绝、未验证域名不提供品牌站、证书许可 404、清除。

未验证或未做：MySQL/PostgreSQL 未跑（新表含可空唯一索引）；没有真实域名，真实 DNS 匹配成功的路径只由替身解析器覆盖；定时任务在运行实例中是否被调度未确认；多节点同步未验证；后台域名面板只通过了类型检查与构建，没有在浏览器里操作过；无改域名的二次密码确认、无已生效域名的定期复查、无暂停状态、无 HTTPS 就绪检测、网关内不签发证书。

## 外观与 SEO（T2.4 / T2.7，2026-11-17）

新增 `cmh_oem_site_appearances`（主题、SEO 标题/描述/关键词、收录开关、版本）与 `cmh_oem_site_logos`（图片字节）。所有者接口：`GET/PUT /api/ext/oem/sites/:id/appearance`（版本 CAS）、`GET/PUT /api/ext/oem/sites/:id/logo`（base64，空串为移除，服务端完整解码校验类型与尺寸）。品牌站模板输出转义后的 title/description/keywords/robots/Open Graph、canonical（仅来自已验证域名）、主题属性与 Logo；新增 `/brand-assets/logo`、`/robots.txt`、`/sitemap.xml`。样式改为 CSS 变量，三个主题为配色与标题字体的差异。后台品牌档案新增“外观与 SEO”面板。

已验证：SQLite 回归（校验、越权、版本冲突、转义、默认回退、canonical/robots/sitemap、登录页恒 noindex、Logo 增删与类型）；临时实例上经真实接口保存主题与 SEO、拒绝非图片与 4096 像素图片、上传并回读 PNG，品牌站页头输出与 Logo 响应符合预期，浏览器中深色主题生效、Logo 加载、控制台无报错。迁移命令连续执行两次成功。

未验证或未做：MySQL/PostgreSQL 未跑（Logo 为二进制列）；后台“外观与 SEO”面板只通过类型检查与构建，没有在浏览器里操作过；品牌站截图未取得，只做了计算样式检查；没有主题预览、没有按主题区分的页面构图、没有 Logo 历史。
