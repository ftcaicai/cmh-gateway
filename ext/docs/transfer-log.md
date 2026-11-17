# 选择性能力移植记录

本仓库只移植业务能力，不导入旧系统数据；完成日期须在实际业务链和验收全部完成后填写。

| 功能 | 范围 | 旧源码依据 | 当前状态 | 验收证据 |
|---|---|---|---|---|
| OEM 后台第一版 | 品牌资料、客户归属、目录/准入可见性、操作审计与手册；不含资金同事务、H4/H6或生产迁移 | `D:\work2\gnzz` documented-377；源映射记录 product commit `bdbaf7e89934eb736d176e36f43457ca9c95646c`，subtree `5f86da3665b42a0aaa2d2a7db028d8b3dd505212`；旧 `router/brand.go`、`model/brand.go` | 阶段已验收（2026-11-16）：品牌档案 API/页面、定向邀请/本人接受/撤销/审计、私有客户备注、本人官方组只读目录及已有 Admin 的只读协作；最新 `3bfc18485` 组合 Chrome 28 项通过，九表 SQLite/MySQL/PostgreSQL fresh/升级/重复迁移与独立连接 CAS 通过，main/develop 远程门禁 success。第一版明确范围已完成，非旧产品全量等价；不含普通员工权限、资金、官方账号启停或 relay 租户隔离 | `ext/docs/OEM_V1_CURRENT_TASK.md` |

| OEM 品牌客户公开入口 | 依据旧品牌站公开入口、登录页视觉与导航推进；不导入旧数据、不复制旧认证或资金系统 | `D:\work2\gnzz` documented-377；`brandweb/index.html`、`brandweb/site.js` 的认证页面及 `middleware/brand.go` 域边界；公开参考 `https://api.7q.com.cn/login` | 进行中：当前品牌域只有原型，原有 `/cmh/` 是商家后台而非客户站；站点品牌解析、客户登录与调用隔离尚未实现 | 待实际验收 |

旧系统 BrandSite 同时包含资金守卫，BrandCustomer 关联旧官方 User 扩展字段；新系统不得整包复制并假称资金/调用隔离已成立。新品牌所有者引用官方用户 ID，权限使用官方 Casbin，独立档案与余额/令牌/计费无关。
