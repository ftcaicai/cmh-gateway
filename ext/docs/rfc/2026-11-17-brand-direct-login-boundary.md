# 品牌域直接登录：有限认证专项（已批准，先 rules PR）

## 当前授权（2026-11-17）

负责人已明确选择“批准有限认证专项（推荐）”：仅下列9个具名官方文件，相对固定起点 `17d70f116946f8099e977cfaa654d9590f96cc33` 合计最多1800增删行，注释/测试同计；H1原49文件/6000行及命名单文件40行独立保留，联合最多58文件/7800行。仅密码＋官方TOTP/备用码、刷新/登出、客户本人最小资料；其他品牌认证方式保持拒绝且不降级MFA。先独立rules PR正常CI通过并合入，再实施；事务合同独立复核、同事务绑定和真实三库/浏览器验收不减。不是生产、资金/H4/H6、旧数据或新增角色授权。下文“尚未批准/候选”系形成该授权前的调查历史，不推翻本段；1800是风险上限而非完成保证。


## 已确认方向

负责人于 2026-11-17 选择“品牌域内直接账号密码登录”。此决定确认用户体验方向；不视为批准新增官方文件、放宽原 H1 预算或部署生产。不得继续把不可登录预览当成该需求的交付。

## 为什么现有非侵入方案不足

- 插件是上游协议机制，不能参与官方账号登录、MFA 或会话签发。
- 扩展复制 `controller.Login`/MFA 是禁止的；直接调用 `CreateLoginSession` 仅证明给定用户可签发，不证明已完成密码和 MFA。
- H2 可以按 Host 拒绝请求，H5 可以注册扩展页面；但它们不能在官方私有 `setupLogin` 完成 MFA 前后，原子附加品牌作用域，也不能确保一个品牌签发的全局 Bearer 不被主站其他接口接受。
- 仅放行登录 URL，再在客户端检查 membership 不成立。仅在响应写出之后拦截 cookie/body 也不成立：会话已经建立，且密码验证后 MFA flow 仍可能被别的 Host 完成。

## 必须满足的合同

1. 仅在由运营配置且独立核验的品牌域启动；精确 Host 与 HTTPS Origin，不采用用户输入或转发 Host 作为品牌来源。正式 DNS 验证、代理信任与 TLS 就绪是独立发布门槛，原环境 Host 映射仅用于合成本地测试。
2. 保留原官方密码加密、验证码、限流、MFA、Passkey、会话版本与失效链。第一批可显式关闭未覆盖的 OAuth/注册/找回，但不得将已启用 MFA 降级为密码直登。
3. 在主认证成功后、MFA flow 创建之前以可信品牌上下文校验允许身份，错误保持通用，避免账号归属枚举。flow 必须绑定品牌及发起浏览器的服务端上下文；主站或其他品牌不能完成该 flow。
4. 签发前再次校验实时品牌关系。会话的品牌绑定须随签发事务持久化，失败不能输出可用 cookie/access token；不能用可由客户端伪造的 header 或未持久化上下文替代。
5. 全局 UserAuth/refresh/logout 校验持久化会话作用域：品牌会话只能在相应品牌的精确客户 API 使用；主站及其他品牌拒绝，主站会话也不能自动变为品牌会话。跨品牌 refresh 不应旋转或撤销合法会话。PAT/relay token 不进入客户会话入口。
6. refresh cookie host-only、Secure/HttpOnly/SameSite，精确 Origin 验证须在品牌域落实，不以所有品牌共享的全局 trusted-origin 列表代替租户检查。代理 TLS 终止需显式信任边界。
7. owner、客户及团队角色必须分别定义；初批客户只允许该品牌 active customer，不暗中把 Admin 只读管理关系变成旧站全权商家身份。撤销/禁用后后续请求拒绝；不声称追溯撤回在途响应。

## 已证实需评估的官方文件（候选，不是批准清单）

| 文件 | 需要解决的阶段 |
|---|---|
| `controller/user.go` | 密码验证后/MFA开始前、认证完成输出前的可信品牌检查；不复制认证 |
| `service/login_verification.go` | 登录 flow 的品牌绑定与消费时核验，覆盖 MFA/Passkey 完成链 |
| `service/auth_session.go` | 会话签发/刷新时作用域持久化与校验，失败不泄漏凭据 |
| `middleware/auth.go` | 浏览器会话在所有主站/品牌路由上的作用域拒绝，不能仅加品牌API检查 |
| `controller/auth_session.go` | 刷新/登出的租户检查顺序，避免跨域凭据导致错误轮换或撤销 |

还需追踪 model 的实际事务写入入口与各 Passkey 完成路径，证明挂载点覆盖后才能形成精确逐函数/文件清单。扩展侧 DTO/interface 留在 `ext/core`，品牌实现及 `cmh_*` 绑定表留在 `ext/brand`/`ext/migrations`，不得给官方表加列。跨模块事务参与者的生命周期、失败原子性也必须明确，不能靠提交后补写绑定。

现有49文件/6000行额度仅授权H1，不授权本认证专项。上述候选至少含此前未登记文件，不得因预算余量而直接修改。尚无可信增删行预算，不能随意报一个小数字让负责人批准后再扩张。

## 实际事务入口核对

- `model/user_session.go:135-163`：普通 `CreateUserSession` 调用 `createUserSessionWithTx(DB, session)` 后发布缓存，并没有可供扩展加入同事务的公开参数。仅在 controller 输出前补写品牌绑定不能保证会话与绑定原子提交。
- `model/login_verification.go:52-89`：MFA 完成通过 `ConsumeAuthFlowWithAction` 的事务执行 `validate(tx, flow, state)`，再 `createUserSessionWithTx(tx, session)`，提交后才发布缓存。现有服务回调用于官方安全状态/旧绑定迁移；不能从扩展覆盖私有官方调用链，也不能假称该回调已自然覆盖无 MFA 登录。
- 因而需要同时覆盖普通签发和 flow 完成的同一受审计模型事务桥，保持缓存发布在已知提交之后；只有 service/controller 检查不够。`model/user_session.go`、`model/login_verification.go` 是新增候选官方文件，尚未批准修改。还需核对 refresh 的锁/版本事务及 Passkey 原生签发是否走同一模型入口。

## 刷新、登出及遗漏入口核对（2026-11-17）

- `service/auth_session.go:234-266` 先取 session/user，用户版本变化时第262行会撤销，再执行 refresh 旋转；品牌作用域拒绝必须在这些副作用之前，不能只在 controller 发响应前检查。`model/user_session.go:473-563` 的旋转使用 CAS，过期 previous token 会写 deny fence 并撤销整族；跨品牌错误提交不能触发此行为。需要在当前请求可信作用域与持久 session 绑定匹配后才进入原旋转链，失败不改变 cookie 或 session。
- `controller/auth_session.go:47-95` 的登出先解析 Bearer，可直接 `model.RevokeUserSession`，且不经过 UserAuth；另一路 `service.RevokeByRefreshToken` 也直接撤销。仅修改 `middleware.UserAuth` 不够，登出须对所用凭据分别检查 session 作用域后再撤销。不得因错误品牌请求清除合法主站 cookie 或借 global trusted Origin 撤销别的品牌会话。
- `middleware/auth.go:205-219` 的统一内部 Bearer 分类与 `:428-448` 的 `TokenOrUserAuth` 分别调用 `ValidateLoginSession`，后者不走前者。两个路径都须校验请求作用域；`TryUserAuth` 也必须保持“越界品牌会话不能变成已认证主站用户”。纯 session 校验 API 无 request 上下文，不能偷偷从全局变量获取 Host。
- `controller/login_verification.go:17-39,42-146` 包含 TOTP 和 Passkey 二级校验；`controller/passkey.go:320-420` 的原生 Passkey 完成直接走 `setupLoginAtAuthVersion`，不经过密码后的 `setupLogin`。若第一批不实现品牌 Passkey，品牌 Host 必须显式关闭这两类路径，且对只配置 Passkey 的 MFA 账号安全失败，不能降级为免 MFA。主站也须拒绝消费品牌绑定 flow，否则关闭品牌路径不等于安全。
- `model/auth_flow.go:148-175` flow 创建直接写 DB，`245-287` 消费+action 同事务；品牌 flow 的绑定创建必须与 flow 行一同提交、消费须在验证因子及任何认证副作用之前检查请求/浏览器绑定，并在事务内再次检查。只在 `CompleteLoginVerification` 末端验证不够，Passkey 状态可能已在 controller 第137行更新。

### 候选范围的当前计数（不是批准）

只读核对的核心候选为 9 个既有官方文件：`controller/user.go`、`controller/login_verification.go`、`controller/auth_session.go`、`service/login_verification.go`、`service/auth_session.go`、`model/auth_flow.go`、`model/login_verification.go`、`model/user_session.go`、`middleware/auth.go`。实际 `git diff --name-only v1.0.0-rc.51 -- <这9文件>` 为空，故全纳入时是在原49文件以外新增9个，累计58个，不是使用现有H1文件余量。若第一批品牌原生 Passkey 也开放，还要评估 `controller/passkey.go` 以及 Passkey flow 包，不可把它们藏在上述9个里。测试以一个扩展集成测试文件集中验证为优先，任何必须修改的官方测试文件另列，不享受免计数。

本阶段仍缺可编码的无循环事务参与接口、完整 noop 兼容证明和实际差异行数预算；不能凭上述候选表启动修改或申请笼统“全部允许”。

## 首批范围建议与校验顺序（待独立审批）

建议首批只开放密码登录及官方 TOTP 二级校验、刷新/登出、客户本人最小资料页。品牌原生 Passkey、Passkey 二级校验、OAuth、注册/找回继续精确拒绝；若官方策略仅提供 Passkey，页面明确显示该方式尚不支持并保持未登录，绝不签发免 MFA 会话。该限制尚未作为用户批准的完整产品替代，审批时须明确列出。主站所有现有认证方式仍保持原行为，同时必须拒绝消费品牌来源 flow/会话。

必须区分“未取得请求作用域”与“合法主站作用域”：既有普通请求保留无品牌分支，品牌相关请求若丢失不可伪造的服务端作用域，必须错误而非回退主站。这个区分需贯穿 controller→service→model 的显式参数，不能依赖 goroutine/global 或把用户传入站点 ID 当授权。

刷新和登出的最小顺序为：解析凭据标识（不视为已认证）→读取持久会话作用域、精确比较可信请求品牌/端点→保留原凭据真实性、用户安全版本及旋转/撤销检查。作用域错误应通用拒绝，不泄漏账号/站点信息；仅通过 SID 查到绑定不表示 token 有效。不能在绑定检查时先调用有旋转/复用撤销副作用的函数来“验证”凭据，也不能用未验证 SID 引起其他会话撤销。主站缺失绑定只有在数据库读取成功且确认为原主站会话时允许；绑定表不可用时不能默认为主站。缓存必须有否定与错误区分，第一批不缓存扩展绑定以简化撤销证明。

## 复核补充：TOTP 不是无副作用检查

`service/login_verification.go:130-142` 在 `RequireLoginVerification` 后调用 `VerifyTwoFactorCode`，然后才完成 flow。`service/security_verification.go:621-644` 实际更新 TOTP 使用记录、消费备用码或增加失败次数。因此请求品牌/浏览器与持久 flow 绑定必须在 `RequireLoginVerification` 返回前核对，不能等 `CompleteLoginVerification` 的事务里才首次判断；事务内复查仍须保留。跨品牌错误请求不得消费合法备用码、改变 TOTP 使用状态或增加他人 MFA 失败次数。这一项必须独立负向测试，而非仅检查没有输出会话。

同理主站 scope 不能等价于“没有传 scope 参数”。旧导出 API 为兼容保留，但必须明确哪些旧调用被限定为主站，哪些 HTTP 调用必须传递解析后的 scope；品牌分支遗漏传递必须在签发/读取持久绑定时拒绝。scope DTO 本身不提供授权，身份依然来自官方认证与扩展实时关系。

独立复核代理已失败且没有结论，不能计作复核通过；实施前需另派真正完成的独立复核。前述发现由主集成者直接读取源码得到。

## 提请负责人的有限工程授权（尚未批准）

不再用无期限只读调查代替实施，提出以下明确止损边界供负责人决定：

- 仅首批品牌域密码登录＋官方 TOTP（含官方已支持的备用码），刷新/登出和客户本人最小资料页。保留主站认证原行为；品牌未支持的 MFA 方法安全拒绝，不免 MFA。品牌注册、找回、OAuth、Passkey、商家/团队品牌身份和 API Key/relay 能力不在首批。
- 官方文件仅上列9个，累计最多58个；不得追加第10个或暗改官方测试文件。新增集成测试集中在扩展区。若证明必须增加文件，先停止该部分，再单独提出精确例外。
- 认证专项单独硬上限为 **1800增删行**，只覆盖9个具名官方文件相对本次开始HEAD的净补丁增删总量（含在这些文件内增加的测试/注释）。这是负责人可选择的工程风险上限，不是已测量工作量或完成保证。现有H1专项49文件/6000行限制保留独立计量，不得拿其余量挪用；当前H1实际4641行，加认证1800的当下联合上界6441行，若H1此后用满时联合最大7800行。
- 必须先提交独立 rules PR，加入具名认证专项门禁、逐函数HOOKS登记和联合计数；正常CI通过后才写官方实现。不得靠压缩代码、移除验证、跳测试凑预算。
- 先在扩展区形成可评审的scope值/事务参与合同；`ext/core` 不导入model/service/relay或GORM，事务提交权仍在官方model，扩展持久绑定与flow/session同事务写入，缓存只在已知成功后发布。接口独立复核通过前不接线，不允许用回调把完整认证方法提前接管。
- 仅隔离新数据的本地合成环境。必须真实SQLite/MySQL/PostgreSQL fresh/升级/重复迁移、绑定故障回滚及完整浏览器安全验收，记录主站noop兼容与所有跨品牌负例。认证专项是否可在该上限内安全收口未知，达到上限就停止并报告具体缺口。
- 本批准不包含生产域名启用、线上认证、旧数据、资金/H4/H6、新角色、全协议客户站完成声明；原拒绝继续有效。

可选决定为“批准上述具名9文件/1800行有限认证专项，先rules PR再实施”，或“保持设计、不批准实施”。此前确认直接登录产品方向不替代本次审批。

## 下一步批准前工作与测试门槛

完成事务/所有认证完成链的只读审计，产出确切文件清单、hook 接口与事务参与合同、合成原型测试计划，再请负责人批准单独 rules/RFC 与累计预算；此前不动官方代码。

验收至少包含：A/B品牌及主站会话互拒；密码正确但不归属通用拒绝；已启用 MFA 不可绕过；flow 跨品牌/跨浏览器/过期/重复消费拒绝；签发绑定失败无可用凭据；跨品牌 refresh 不轮换；撤销、禁用、版本失效；Host/Origin/代理欺骗；cookie 属性与 secret 不落日志；真实三库 fresh/升级/重复迁移及失败原子性；浏览器完整登录至客户页面。全部为待执行，不是已通过证据。

仅用于新系统隔离环境，不访问旧库、生产账号、线上认证或付费接口。旧源码与公开页面仅作为产品对照，不照搬旧安全实现。
