# H6 扩展迁移启动边界（设计提案，2026-11-14）

状态：未批准、未实施；不启用archive、grants或资金表，不申请资金实施权限。

## 原位置的问题

旧方案在model/main.go的migrateDB末尾增加观察调用。但迁移会修改schema且可能失败，并非只读观察；void回调忽略错误不能证明启动就绪。当前InitDB仅master运行migrateDB，nonmaster提前返回。独立log DB有不同初始化路径，不应接收主库扩展表。已有MigrateContinuation明确未注册，其三库测试不等于H6启动接线。

## 非侵入分析及候选位置

插件不能完成宿主schema生命周期；ext独立函数可迁移自己的表，但未经启动桥不能保证在需要该schema的服务前执行；H2请求中间件及H5路由注册都不是迁移屏障。

候选复用已登记main.go的InitResources：model.InitDB成功后、authz及后续worker前。ext可调用导出的model.DB，core只传无业务启动合同，不引入model/GORM依赖。它避免新增model/main.go官方路径，但仍是main.go的新用途，现有H1登记不自动授权。精确位置及全部更早消费者须在实施前重查，不能仅凭函数名保证preworker。

## 待确认合同

1. 默认无feature/noop不迁移、不增加DB访问；配置要求schema时，错误必须阻止serving，返回固定安全错误，不吞错继续。
2. master负责独立cmh_*迁移；nonmaster只能readiness验证，不抢跑DDL。readiness的版本判据、滚动升级兼容窗口与主节点并发协调尚未设计，不以HasTable一项代替。
3. 只允许扩展表加表/加列，不更改官方表，不自动改独立log DB。不自动注册continuation仓库、密钥或财务模块。
4. policy-only当前早期配置校验仍在DB前；H6后DB阶段不能绕过该屏障或改变archive/grant禁用约束。

## 拟议验收与授权边界

真实SQLite/MySQL/PostgreSQL分别验证fresh及代表性已发布版本升级、连续两次启动、既有数据/index保留、master迁移失败拒绝serving、nonmaster缺schema拒绝与已有兼容schema通过；分别配置log DB并确认无扩展DDL。多主并发和中断恢复须有确定性故障断言。默认noop必须维持官方行为。以上均未执行，不声称数据库兼容或H6完成。

原位置新增model/main.go将超出现有49具名路径；候选main.go虽然已经计数，也仍需H6用途、错误语义、迁移范围及独立预算批准，不能借H1余额。精确schema和readiness合同未闭合前不落代码，不更新HOOKS当前白名单；H6继续预留。若未来涉及资金表，负责人当前不批准资金实施的决定仍优先，不能通过H6绕过。
