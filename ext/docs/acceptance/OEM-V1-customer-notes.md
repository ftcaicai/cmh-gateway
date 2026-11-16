# OEM V1 客户私有备注验收（进行中）

## 精确业务合同

- 商户专用 `GET/PUT /api/ext/oem/sites/:id/customers/:user_id/notes`，复用官方浏览器会话和 `cmh_oem` read/write；同时要求精确品牌所有者及已接受客户关系。撤销后的历史关系仍允许原商户管理备注，不恢复活跃关系。
- GET 返回 `note/version/updated_at`；不存在备注时空字符串、版本 0。PUT 只接受 `note/version`，无所有者或用户身份字段。备注最大 2000 UTF-8 字节，允许 LF/tab，拒绝其他控制字符，纯文本显示。
- 首次版本 0 使用唯一 UserID 槽位及版本 CAS；后续按当前版本 CAS。不得依赖 MySQL OnConflict DoNothing 的 RowsAffected 来判定新插入成功。
- 备注更新、版本加一和 `customer.note_updated` 审计必须在同一事务；返回事务内取得的已提交快照。审计不存备注正文。
- 不改变 Customer / 客户本人 membership DTO，不向客户、邀请列表、客户列表或审计泄露私有正文。不写官方角色、账号状态、密码、额度或模型令牌。
- 新增 `cmh_oem_customer_notes` 独立扩展表，迁移显式执行；无 H6 自动 DDL。已有五表到六表须重新实测升级、重复迁移及数据保留。

## 当前验证状态

后端与页面源码已交回，最终实际 Chrome 组合验收 **20 项 PASS**，尚待提交发布。此前 14 项全部保留，新增备注真实 LF/tab 保存、冷刷新持久化、独立 React 文档旧版本 409 后显式重新加载、CDP 暂停真实 GET 后切换品牌不串草稿、精确 note/version wire、membership 与审计不含正文等检查。结果为 `ops/.cache/oem-notes-browser-1794815564961/results.json`；12 项生产输入 SHA 前后完全一致。二进制 SHA256 `45601a98a041a11c93ea0e1f0060a7b7b42d20545bdbdb2e04505533d089c173`，嵌入 index `36015aff6fe21a7bbc37e6cd3cfee9d9bcf789a8b3e2b65bda31e72dc23fa348`。验收及清理任务 exit 0，进程 0，38189/39389 无监听；无旧数据、生产或付费调用。合成官方会话不等于真人密码/MFA、TLS 或上游供应商验证。

- 真实 SQLite 3.50.4 / MySQL 8.0.44 / PostgreSQL 17.12：`powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-oem-customers-matrix.ps1`，最终矩阵 exit 0，分别 0.75 / 0.87 / 1.09 秒，包 8.370 秒。六表新建重复两次、已有五表含资料/审计/归属升级重复两次、新连接重读及主键唯一性均通过。
- 两独立连接池在备注不存在版本 0、已有版本 1 的相同版本竞争均只有一方成功、一次版本递增及一条审计；不是已证实锁重叠或 race 检测认证。审计失败更新回滚、历史已撤销关系编辑/清空、UTF-8/控制字符/字节/版本边界、跨所有者/错误品牌 404、自助端点 403 及正文不泄露均通过。
- 集成 `go test -p 2 ./ext/... -count=1`（brand 7.027 秒、domestic 8.779 秒），`go vet -p 2 ./ext/...`、`go build -p 2 ./...` 全部 exit 0。所有独立数据库引擎正常停止，后台任务已回收。
- 页面实际 35/35 客户端 wire/schema 测试、TypeScript、lint（零 warning）、format、Rsbuild 均通过；构建 826.6 kB / gzip 264.3 kB。这不替代 DOM 验收。页面按品牌/客户隔离挂载，409 后必须显式重新加载，写操作不自动重放。

## 独立复核与已发现事项

- 主集成者检查发现初版版本号仅限制 int64 上界，已修复为 0..(2^53)-2，并执行负数/上界回归。页面初版重复挂载备注编辑器已在最终交还前移除。
- 独立普通只读复核已检查最终后端与 UI，无具体阻断；未代运行矩阵，不冒称专用安全审计或完整 ASVS 认证。服务端每次核对所有者/品牌/客户关系，正文不进入客户 DTO/审计；官方浏览器会话与 Casbin 维持原边界。
- 团队委派不属于备注接口授权：官方 `Can` 对普通用户没有系统角色时在显式 override 前拒绝，不能假传 Admin 或提升全局角色来补团队功能。备注沿用现有商户边界，不借此放宽权限。
