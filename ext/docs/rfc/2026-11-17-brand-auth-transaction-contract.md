# H8 同事务参与接口候选（未接线）

规则 PR #4 已合入；本文是可编码接口候选，不是认证完成或独立复核通过证据。

## 所有权与依赖

- `ext/core` 只定义请求 scope、无凭据绑定投影和校验接口，不导入 Gin/GORM/model/service。scope 不是权限，可信构造者属于 Go 宿主 TCB；请求只由 H2 已解析的服务端配置构造，不能从 header/body 直接解码。
- 官方 model 是唯一 transaction/commit owner。普通 session 创建和 login flow 创建新增 scoped 变体；旧导出 API 委托显式 MainScope，保留关闭扩展时行为。MFA 完成使用现有 ConsumeAuthFlowWithAction 的 tx，禁止嵌套新事务。
- model 定义同步参与者 `type AuthScopeParticipant func(tx *gorm.DB, binding core.AuthScopeBinding) error` 与启动期注册入口。扩展通过其公开入口注册函数，不把 GORM 带进 core，不通过 core 的 any 句柄泄漏事务。model 只调用参与者，不导入 ext/brand；ext/brand 可以导入 model 已公开接口。注册后不可热替换；回调返回前必须完成所有 SQL，不留 tx、不起 goroutine、不 Commit/Rollback、不递归创建 flow/session。
- 回调是受信任宿主代码，不是恶意 Go 插件沙箱。model 在同一个 tx 中创建官方行并同步调用参与者；任一失败整体 rollback，缓存、Cookie、access token 均在已知提交后产生。

## 最小接口与阶段

`AuthScope{Kind Main|Brand, SiteID, Origin, BrowserDigest}` 仅描述当前请求；不包含密码、flow token、refresh secret 或 access token。品牌 Origin 是精确 scheme/host/effective-port；BrowserDigest 是随机 host-only HttpOnly cookie 的摘要，不能用 UA/IP 代替。

`AuthScopeBinding{Scope, Kind Flow|Session, RecordID, UserID, ExpiresAt}` 用于同事务写入。Flow RecordID 使用官方持久 TokenHash，不持久化原 token；session 使用 SID。扩展表分别唯一约束记录标识。主站同样写显式 main 绑定；启用前有效历史记录须按下文维护窗口迁移，启用后缺失绑定拒绝。绑定表错误一律拒绝。

core 接口候选：`CheckIdentity(ctx context.Context, scope AuthScope, userID int) error`；`CheckBinding(ctx context.Context, scope AuthScope, kind BindingKind, recordID string) error`。前者检查真实 active customer/site/owner 状态，后者检查持久作用域。model 同事务参与者必须在 tx 内复核 active 关系，不能只信之前 CheckIdentity 的结果。主站 scope 不能默认为空值；零值拒绝。

## 接线顺序

1. H2 创建 scope；品牌仅开放精确密码/TOTP/refresh/logout/最小 self 路径，所有其他方法/路径继续拒绝。Origin 校验不继承全局跨品牌 trusted-origin 宽免。
2. 密码及官方验证码成功后，检查身份；创建 MFA flow 时同事务绑定 scope+browser digest。不支持的 MFA 方法拒绝，不降级。
3. RequireLoginVerification 返回前验证 scope+browser，再进入官方因子校验，防跨站消费备用码/TOTP；完成消费/会话创建时在现有 tx 内重查 flow 绑定与身份，并插 session 绑定。
4. 普通无 MFA session 在 scoped model 创建事务内同样写绑定。主站原生 Passkey 的旧 API 明确 MainScope，必须拒绝品牌 flow。
5. refresh/logout 先做无副作用持久 binding 检查，再进入原 token 真伪/版本/旋转或撤销；错误不清 Cookie、不旋转、不撤销。SID 匹配不是凭据验证。
6. UserAuth/TryUserAuth/TokenOrUserAuth 在所有浏览器身份入口检查 scope。不能只保护品牌 self；主站不能接受品牌 Bearer，品牌也不能接受主站会话或 PAT。

- 旧导出 MainScope 包装器仅供原主站/非请求内部调用兼容，不得作为 HTTP scope 丢失时的 fallback。所有批准范围内 HTTP 登录/MFA/刷新/登出/身份解析必须显式取得 RequestAuthScope；缺失或零值拒绝。旧 MFA 完成包装器还须通过持久 flow 主站绑定核验，不能凭 MainScope 参数把品牌 flow 变成主站 session。普通无 MFA 的遗漏无法靠最终 SID 查表补救，因此 HTTP 接线测试必须断言 H2 scope 确实抵达签发事务；不可仅测 core 零值校验就声称覆盖。

## 启用与历史主站记录的确定性分类

不能把“没有 binding 行”本身当成主站证明。首批使用显式迁移命令，在网关停止签发/刷新/消费 flow 的维护窗口内，把当时仍有效的官方 session 和 login-verification flow 分批回填显式 Main 绑定；每条绑定须引用实际官方行及其 user/expiry，幂等核对而非覆盖。该命令不属于 H6 自动迁移，不读取旧系统。迁移结束后验证不存在有效的未绑定行，才允许 operator 启用 H8。主站旧 API 后续创建也必须在同事务写 Main 绑定。

启用后，找不到绑定一律拒绝，不依据 CreatedAt 时间戳、客户端声明或启动时刻推断“历史”。绑定表不可用、参与者未注册、重复注册、配置与迁移就绪标记不一致均阻止品牌认证启用；不能静默 noop。H8 完全未启用时保持官方兼容路径，品牌认证仍关闭。启用配置只在启动安装，不支持热切换；回滚须关闭品牌认证并保留绑定读取拒绝，不能使品牌会话重新成为全局会话。进程滚动期间混用不支持 H8 的旧二进制不属于安全部署方式，必须维护窗口整体切换。

此迁移/安装流程尚未实现；它将增加显式部署门槛，不代表已经完成历史兼容证明。若实际接口预算无法覆盖主站全部创建路径，应停止该实现并报告，而不是豁免未绑定记录。

## 因子副作用与事务失败的精确语义

官方 `VerifyTwoFactorCode` 在完成 flow/session 事务前可能消费备用码、记录 TOTP 使用或更新失败次数。现有九文件专项没有授权把 `service/security_verification.go` 重构进同一事务。因此首批保证是：错误品牌/浏览器在调用它之前被拒绝，绝不消费因子；正确作用域已完成官方因子后，若后续绑定事务失败，flow 消费/session/绑定必须整体回滚且不发凭据，但因子使用记录不承诺回滚。用户须重新使用有效因子，不自动重放已消费备用码。不能把数据库绑定回滚测试写成“所有 MFA 副作用都回滚”。

事务内仍须重验 flow scope、有效身份及先前验证结果的确切 flow/user/auth-version 绑定；对正确请求在因子验证与最终事务间发生的撤销，仅承诺不签发会话，不承诺撤销已经发生的因子使用。此限制须进入错误恢复说明和负向验收；不因此弱化跨作用域因子零消费检查。

## 必须补证

先独立复核上述 model 注册依赖是否违反规则、历史主站兼容和启用前已创建记录分类、注册缺失与H8启用一致性；若接口存在绕过，先修正再写官方调用点。真实三库验证 callback 失败时 flow 消费/session/绑定整体回滚，错误 scope 不消费因子/refresh、不影响合法 Cookie；正确作用域的官方因子使用副作用按上文限制，不承诺回滚。BATCH 开关不应改变认证绑定原子性。所有事务集成测试尚未实施。
