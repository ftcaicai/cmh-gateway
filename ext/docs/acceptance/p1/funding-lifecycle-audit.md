# P1 资金生命周期只读复核（2026-11-13）

状态：**架构假设未通过；未实施资金接管挂载点，未连接生产或执行付费调用。**
基线：`v1.0.0-rc.50`，本地官方资金代码；独立复核代理 `1c744a3a-ed8d-4af2-9db1-d00bf1249cac`。本记录是源码审查，不是运行时或三库验收。

## 需要撤回的旧结论

此前方案称 `BillingSession.preConsume/Settle/Refund` 与 `taskAdjustFunding` 是四个唯一资金出口，可在入口加 `handled` 直接返回。这一结论不成立。官方没有一个现成的共同事务，不能将“ext 自己完成扣款”说成“官方与 OEM 同事务”。用户已选择同事务，未经明确授权不能改为双账本最终一致，也不能自动切换成替代官方钱包的业务模式。

## 已核实的路径

| 范围 | 源码位置 | 漏掉的语义 |
|---|---|---|
| 会话初建 | `service/billing_session.go:389-489` | `preConsume` 之前读取官方钱包，拒绝额度不足并选择订阅；入口接管无法绕过或替换此选择 |
| 正常预扣/结算/退款 | 同文件 `47-128,208-261,364-381` | 包含锁、会话终态、token 限额、资金提交标记、失败补偿、RelayInfo 同步；直接 return 会跳过这些，不只是跳过钱包 |
| 追加预留 | 同文件 `158-200,264-315` | 直接增减钱包/订阅与 token，具体类型 switch 不接受任意第三方 FundingSource；预扣失败还有独立反向补偿 |
| 直接结算 | `service/quota.go:435-485`，`service/billing.go:65-108` | 未使用 BillingSession 的兼容分支仍写钱包、订阅、token |
| 实时与违规费用 | `service/quota.go:90-151`，`service/violation_fee.go:101-125` | 不由四个入口完整覆盖；退款后可追加独立违规事件，不能共享同一去重键 |
| 旧 Midjourney | `service/midjourney.go:34-136` | 独立提交/退款、钱包与 token 更新，完全绕过 H7 |
| 通用异步任务 | `service/task_billing.go:105-149,267-380` | funding/token/统计/日志/Quota 标记各自提交；taskAdjustFunding 无 Gin context，不能只凭 Host 识别租户 |
| 轮询与恢复 | `service/task_polling.go:99-175,211-222,279-299,681-712,790-801`，`model/task.go:413-436` | 终态 CAS 与资金不是同一事务；进程重启后没有请求上下文，必须持久化租户/请求/预留/任务归属，失败终态不能等同已经完成退款 |
| 认证限额 | `middleware/auth.go:572-620`，`model/token.go:224-254` | 禁用、到期、耗尽 Key 在计费前校验；不能用伪造用户余额或 unlimited token 绕过 |
| 缓存与批量更新 | `model/quota_reserve.go:105-238`，`model/user.go:1340-1486`，`model/token.go:382-438` | Redis 预留、异步缓存、内存批量队列与 SQL 无共同事务；需要 Redis × BATCH 四组合实测，不只两种 BATCH |
| 非调用资金入口 | `model/subscription.go:770-862`，`model/topup.go:90-118`，`model/redemption.go:136-185`，`model/checkin.go:94-139`，`model/user.go:587-621`，`model/user_quota_adjustment.go:25-85` | 支付、兑换、签到、推广、管理员直接 SQL/Save 写资金，grep Increase/Decrease 并不能证明覆盖 |

订阅退款的额外风险：`model/subscription.go:1424-1442` 外层事务内调用 `PostConsumeUserSubscriptionDelta`（`1528-1548`），后者使用全局 DB 另开事务；源码不足以证明退款与退款标记原子提交。未执行故障注入，不能把此判断说成已复现上游漏洞，也不在本任务中直接改官方逻辑。

## 可行性约束

1. **优先探索 FundingSource 工厂而非整段方法提前返回**。官方 `service.FundingSource` 是现成抽象，但构造器、Reserve/rollback、NeedsRefund 仍有具体类型判断，不能只注入一个对象就称完成。
2. 用户所选同事务应定义清楚：哪些资金余额、预留、token 限额、账本、outbox 必须同一提交；派生日志允许 outbox 恢复，不等于财务本体最终一致。
3. 需要持久归属，不改官方表时可用 ext 自有请求/预留/任务关联表；关联创建与派发前持久化必须验收，后台不得退回官方钱包路径。
4. 任意实现先保持品牌资金调用关闭；不把未实现租户请求路由到官方钱包。原型可以使用合成记录，但不能使用伪造 Redis 缓存或官方额度来模拟成功。
5. <=150 行、<=10 官方文件目前只是预算，**尚未证明可行**；不足时提交 RFC，不压缩代码、删校验来过预算。
6. 必需证据：真实三库新建/升级/重复启动；Redis × BATCH；故障注入、重放、重启、并发、追加预留回滚、终态已写资金未写等恢复；独立复核。

## 当前结论与下一动作

P0 的静态页与官方登录契约可独立继续。P1 不按旧四入口结论落地，先做资金边界设计复核与 RFC；未获确认前不将接管架构写为已批准，不扩大差异预算，不宣称守恒通过。P2 的纯白名单逻辑可独立设计，但未经完整资金生命周期验收不开放品牌付费流量。
