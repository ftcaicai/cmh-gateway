# P0 上线阻断项：PostgreSQL 验证记录（2026-11-19）

依据 `OEM-MIGRATION-TASKS.md` §6 P0"三库验证"。本记录只写实际执行过的命令和结果；按负责人指令只验证 PostgreSQL，**MySQL 与 SQLite 本次未跑**，不计为三库通过。未执行项单列在最后。

## 环境

- 远端 PostgreSQL 16.13（x86_64-pc-linux-musl，Alpine），主机 `100.66.1.1:5432`，`postgres` 超级用户。连接串不写入本记录。
- 在同一服务器新建四个专用空库 `cmh_oem_test`、`cmh_auth_test`、`cmh_migrate_fresh`、`cmh_migrate_upgrade`；验证后保留，可由运维自行删除。
- `cmhlogic` 是网关开发实例正在使用的主库，只执行了只加表的升级迁移；**未对其跑任何测试套件、回填或撤销**。
- 本地另起过 PostgreSQL 16.13（Windows，端口 55432）作为前置复验，结论一致，已停止。
- 工具：`go1.26.10 windows/amd64`，`ext/brand/cmd/oem-migrate` 与两个忽略区一次性辅助程序（`ops/.cache/db-test/`，不提交）。

## 1. 全新库迁移（`cmh_migrate_fresh`）

`oem-migrate -engine postgres` 连续执行 3 次，均输出 `OEM extension tables ready`。结果 13 张 `cmh_*` 表：

- `cmh_auth_scope_bindings`：`idx_cmh_auth_scope_record (kind, record_id)` 唯一索引、`expires_at` 普通索引。
- `cmh_oem_site_domains`：`verified_host` 为可空 varchar，`idx_cmh_oem_site_domains_verified_host` 唯一索引。
- `cmh_oem_site_logos`：`data` 列为 `bytea NOT NULL`。
- `cmh_oem_site_appearances`：主键 `site_id`。

## 2. 旧版库升级（`cmh_migrate_upgrade`）

1. 用 `3bfc18485` 版本的 `ext/migrations/oem.go` 结构建出上一发布版的九张表并各写一行样本数据（品牌、审计、客户邀请/关系/审计/备注、团队邀请/成员/审计）。
2. 建官方 `users`、`user_sessions`、`auth_flows` 并写入 1 条有效会话、1 条已过期会话、1 条未消费登录流程。
3. `oem-migrate -engine postgres` 执行 2 次通过；表数 12 → 16，九张旧表的全部样本行与唯一索引保留（含备注换行内容、版本号）。
4. `-backfill-main-auth` 第一次 `main-site auth bindings added: 2`（有效会话 + 流程，过期会话正确跳过），第二次 `0`，不覆盖已有绑定。
5. 域名/外观/Logo 实际读写（通过 `SiteStore`，`CMH_BRAND_CNAME_ROOT=brands.platform.test`）：
   - 两个未验证域名可并存（可空唯一索引下的 NULL）；非所有者修改被拒。
   - 写入 `verified_host` 后 `ReloadBrandHosts` 从库重载，`API.ALPHA.EXAMPLE:443` 解析到该品牌，未验证域名仍为主站。
   - 第二个品牌写同一 `verified_host` 被唯一索引拒绝。
   - 换域名后旧已验证域名立即失效；移除域名删除行。
   - 外观保存版本 1；旧版本、未知主题、非所有者读取均被拒。
   - 16×16 PNG 写入 `bytea` 后逐字节回读一致；非图片拒绝；移除后行删除。本轮品牌共 7 条操作记录。
6. `-revoke-brand-sessions`：第一次 `brand sessions revoked: 1`，第二次 `0`；`sid-brand` 状态 `revoked`、原因 `brand_auth_disabled`，主站会话与过期会话保持 `active`。

## 3. 开发实例主库升级（`cmhlogic`）

升级前：47 张表，`cmh_*` 已有上一版的 10 张（九表 + 绑定表），`cmh_oem_sites` 1 行、`cmh_auth_scope_bindings` 13 行、`users` 2 行、`user_sessions` 13 行。

`oem-migrate -engine postgres` 执行 2 次通过。升级后 50 张表，新增 `cmh_oem_site_domains`、`cmh_oem_site_appearances`、`cmh_oem_site_logos` 及 `idx_cmh_oem_site_domains_verified_host`；上述所有行数不变，品牌行 `e48d1e1b…|owner 1|version 1` 原样。未执行回填与撤销。

## 4. 测试套件（专用空库）

| 套件 | 结果 |
|---|---|
| `go test -p 1 ./ext/brand -run 'TestOEMSitePersistenceAndOwnership/postgres\|TestOEMCustomerOwnership/postgres\|TestOEMTeamDatabaseMatrix/postgres' -count=1`（`CMH_OEM_TEST_POSTGRES_DSN`） | 三项 PASS，包 39.0s |
| `go test -p 1 ./ext/brand/authtest -run 'TestBrandLogin/postgres' -count=1`（`CMH_AUTH_TEST_POSTGRES_DSN`） | 11 个子用例全部 PASS，包 5.1s |
| `go test -p 1 ./ext/brand -run 'TestHostIsolation\|TestSelfServiceDomain\|TestStorefrontAppearance' -count=1`（SQLite 内置 fixture，修复后回归） | PASS |

## 5. 发现并修复

`oem-migrate -revoke-brand-sessions` 在任何配置下都会 panic：`common.RedisEnabled` 默认为 true，只有网关的 `InitRedisClient` 会清掉它，而该命令从不调用，于是官方 `RevokeUserSession` 写 Redis 拒绝围栏时对空客户端解引用。修复：撤销前显式置 `common.RedisEnabled = false`（扩展区文件 `ext/brand/cmd/oem-migrate/main.go`，官方区差异门禁仍通过）。panic 发生在写库之前，库中会话未被改动，因此此前文档中"关闭 H8 前先撤销"的步骤在本修复前实际上无法执行。

## 未执行（不计为通过）

- MySQL、SQLite 本次按指令未跑；`cmh_oem_site_domains` 可空唯一索引在 MySQL 上的语义仍需实跑。
- 真实 DNS 匹配成功路径仍由直接写 `verified_host` 替代，域名定时任务 `cmh_brand_domain_verify` 未在运行实例中观察。
- 多节点同步主题 `cmh_brand_domains` 未验证（辅助程序里 `system_tasks` 表不存在，bump 失败只记日志）。
- 真实 TLS、Origin 守卫冲突、人工独立复核未做。
- `cmhlogic` 上未开启 `CMH_BRAND_AUTH`，未验证开启后的主站请求行为。
