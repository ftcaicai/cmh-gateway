# 选择性能力移植记录

本仓库只移植业务能力，不导入旧系统数据；完成日期须在实际业务链和验收全部完成后填写。

| 功能 | 范围 | 旧源码依据 | 当前状态 | 验收证据 |
|---|---|---|---|---|
| OEM 后台第一版 | 品牌资料、客户归属、目录/准入可见性、操作审计与手册；不含资金同事务、H4/H6或生产迁移 | `D:\work2\gnzz` documented-377；源映射记录 product commit `bdbaf7e89934eb736d176e36f43457ca9c95646c`，subtree `5f86da3665b42a0aaa2d2a7db028d8b3dd505212`；旧 `router/brand.go`、`model/brand.go` | 阶段已验收（2026-11-16）：品牌档案 API/页面、定向邀请/本人接受/撤销/审计、私有客户备注与只读目录；最新 `0ac685b39` 组合 Chrome 20 项通过，六表 SQLite/MySQL/PostgreSQL fresh/升级/重复迁移与独立连接 CAS 通过。当前为可用业务增量，完整 OEM 目标仍须完成已批准的已有 Admin 只读协作，非旧产品全量等价 | `ext/docs/OEM_V1_CURRENT_TASK.md` |

旧系统 BrandSite 同时包含资金守卫，BrandCustomer 关联旧官方 User 扩展字段；新系统不得整包复制并假称资金/调用隔离已成立。新品牌所有者引用官方用户 ID，权限使用官方 Casbin，独立档案与余额/令牌/计费无关。
