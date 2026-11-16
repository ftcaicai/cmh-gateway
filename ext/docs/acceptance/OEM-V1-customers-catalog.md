# OEM V1 客户与目录验收记录（进行中）

## 业务合同

- 仅关联已存在且启用的普通官方用户。没有 User.Insert、密码生成、官方余额/令牌/状态变更或旧数据导入。
- 管理员必须通过官方浏览器会话及 cmh_oem read/write Casbin，并被所有者谓词限制到自己的品牌。本人查询/接受接口仍要求官方浏览器会话，但不要求商户权限；PAT 不是替代入口。
- 创建邀请使用 crypto/rand 32 字节，固定 24 小时，绑定目标 UserID。仅成功创建响应返回一次 hex token，持久化只存 SHA256；列表和审计 DTO 不含 hash/token。
- 接受通过目标用户匹配、期限和邀请 CAS，将消费、全局唯一关系和审计放在同一事务。OnConflict DoNothing 后再次读取持久化行并校验 invitation/site/owner/active，不能依赖 MySQL RowsAffected 来推断插入成功。
- 同一已接受且 active 关系可幂等重放，即使原邀请期限已过；不同邀请、不同品牌及 revoked 关系不可重新绑定。撤销不删除官方用户或禁用其 relay，归属 UserID 仍保留。
- 这是明确的关系管理，不等于调用租户隔离。官方用户状态为导出 fresh lookup 的读时检查，不是与官方账号变更同事务锁定的保证。

## 已执行与待执行

- 源所有者完整 `go test -p 2 ./ext/brand/... -count=1` 通过 6.737s，vet/diff-check 0，覆盖 SQLite 目标/所有者/期限/重放、第二邀请冲突回滚、审计缺表导致接受与撤销回滚、PAT 拒绝及严格输入。
- 父会话稳定生产代码 `go build -p 2 ./...` exit 0。
- 最终新五表真实矩阵通过：SQLite 3.50.4（0.70s）、MySQL 8.0.44（0.97s）、PostgreSQL 17.12（1.10s），包 2.969s。脚本 `ops/.cache/db-test/run-oem-customers-matrix.ps1`，脱敏输出 `ops/.cache/oem-customers-final-matrix.log`。每引擎 fresh 五表迁移两次；重建此前两表并写品牌及审计数据，再升级五表两次；独立连接验证旧数据保留。仅清理专属 fixture 的五张 OEM 表及本次创建的合成 User/session/PAT 行，未删除官方表或数据库；引擎均正常停止，所有作业已收取。
- 两独立 GORM pool、不同邀请争抢同一目标，启动屏障及 10 秒 context：各引擎仅 1 成功、1 活跃归属、1 accepted 审计、1 已消费邀请。证明真实独立连接并发启动与最终不变量，不承诺锁内精确重叠或 race 认证。
- 客户审计增量后最终客户端 31/31 测试、类型、lint（零 warning）、format、生产 build 通过（816.5kB / gzip 262.2kB）；父会话独立执行 31/31 与 typecheck 通过。客户端测试不替代 DOM/E2E。
- 独立 UI 最终只读复核暂无具体阻断：site keyed 卸载隔离旧响应；token 仅局部状态、mutation gcTime=0/reset；本人接受不被商户 403 隐藏；POST 无自动重放。审计按 site 隔离 query key，邀请/撤销使当前审计失效，固定动作映射不输出原始未知值。
- 独立服务端身份/事务只读复核未发现具体阻断；参考 OWASP Authentication、Session Management、CSRF Prevention。其单连接竞争证据不足已由上述独立连接补充解决，但不扩大结论。集中 HTTP 补测通过：同一非 root Admin 会话客户权限 deny→recover、第二 Admin 四类跨 owner 404、仅 refresh cookie GET/POST 拒绝、禁用目标 400；启用真实 critical limiter 阈值 1 后首次无效接受 400、其次 429，均零客户/邀请/审计写入。`go test -p 2 ./ext/brand/... -count=1` 7.373s / vet / diff-check 0；HTTP 专项 count=2 再通过 1.216s。上述 HTTP fixture 为 SQLite，不冒充三库认证全链。
- 邀请/接受复用官方 UserCriticalRateLimit，服从官方 CriticalRateLimitEnable 开关及阈值，不是无条件硬限。最终客户多身份真实浏览器闭环正在执行。没有完整 ASVS 或生产 TLS 合规声明。

## 最终组合真实浏览器验收

14 项检查全部 PASS，exit 0：品牌八项完整重跑；商户 DOM 创建定向邀请 200；普通目标客户在独立 CDP BrowserContext 实际表单接受 200 并显示活跃关系；商户重载看到 Accepted，经实际 ConfirmDialog 撤销 200；商户审计 UI 实际显示 invited/accepted/revoked 三类；客户冷加载显示无活跃关系。普通客户商户列表 403、本人 membership 200。真实 catalog 200 且界面显示空目录和实际策略状态，未伪造模型、渠道或上游。

脱敏结果 `ops/.cache/oem-browser-1794810440952/results.json`，构建前后记录的生产源 SHA 完全一致。二进制 SHA256 `80964ada4f261dc0ec149c2a4a7c0ea360cd10da1fde8d0eb873d53ddf13813a`；嵌入 index `60b20e87b1f28bd22672dc4bf7a792b447e36fad858ccd4665619526a55ede67`；客户页面 `0fe28a122948335f4b3a74788d81a89c1acd265b3928ac2fe1e4616e8998de48`；目录页面 `d2a2a6dc6e7fb0eba26602e78f2d09f91af466c030f54f7743d26ab8c5accf43`。邀请 token 仅进程内存，不写结果或输出。明确 loopback HOST、清空 operator 配置、fresh cwd、fresh SQLite，无旧库或生产操作。最终清理复核 exit 0：匹配进程 0，38189/39389 均关闭，全部子任务收齐。

早期失败运行源于 fixture 对独立 context 切换误用全浏览器清 Cookie 导致商户返回 401；已修正并完整重跑，不隐藏失败也不将早期部分通过算作最终证据。合成官方会话注入不是真人密码/MFA E2E，品牌双编辑器为 iframe 中独立 React 文档，客户为独立 BrowserContext；本地 HTTP 不证明生产 TLS 或 relay 租户隔离。

## 模型目录

只读返回当前官方组 enabled abilities、enabled channel 及启用 publication gate 可见的模型 ID，去重排序；同响应仅额外返回 model/destination policy enabled 两个布尔。使用 error-returning 官方能力快照和 fresh 官方用户/渠道 lookup；失败不能伪装空目录。没有价格、组、渠道 ID、密钥、URL、配置和审批明细；不创建新运行权限。取消仅在导出 DB API 前后检查，不声称中断 DB I/O。

独立目录源任务实际 SQLite `TestOEMCatalog` 5.788s/vet/diff 0，覆盖跨组、禁用 ability/channel、gate 拒绝、输出白名单、失效用户、取消、缺表、nil DB。尚不宣称真实生产模型调用或三库目录 HTTP 全链。
