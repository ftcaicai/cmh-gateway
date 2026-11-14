# 资金首批隔离复核（只读设计，未实施）

方向已批准；资金实施、扩预算及生产未批准。

## 已核实风险

- controller/token.go:431–473 status_only仍调用宽字段model/token.go:314–321 Update，绝对写remain_quota，可能覆盖并发结算。
- model/token.go:382–439先异步Redis再batch/SQL；model/quota_reserve.go:202–238含zero/unlimited/Redis/fallback。必须在副作用前隔离，并防御低层持久化与历史flush。
- model/utils.go:43–99先换出batch map再落库，无归属/请求幂等。首批token应从创建即专属，不接管已使用普通token。
- model/token_cache.go:12–25,46–135 MAC排除余额，10秒fence不能阻止旧快照回填；hydration保留整数counter。model/token.go:284–304 fromDB=true也会水合缓存。outbox DEL不是一致性证明。
- model/user.go:996–1054硬删除直接批量删Token，绕开Token.Delete；先锁User。品牌交易不可Token→User反序，未结算归属需保留或拒删。
- 未发现生产Save(token)不等于通用GORM旁路已受保护，未来Updates/Delete仍需约束。

## 候选范围不是预算

仅token子集涉及model/token.go、model/quota_reserve.go、model/token_cache.go、model/utils.go、model/user.go、middleware/auth.go六文件，controller/token.go可能第七。资金session、factory、settlement、日志及派发桥另计。原8–12文件/700–1500行只是估算，不是可靠上限或实施申请。认证实施另须OWASP验证，当前未改认证。

## 派发/结算顺序

relay/compatible_handler.go:101–159含passthrough/outbound/参数覆盖/egress，163才派发。入口纯文本/非stream不足，须证明最终有效请求及计费上界。172意外SSE、188–194音频usage不能回退非首批资金路径。

service/text_quota.go:400无返回值，462–468先统计后结算，错误仅日志。须持久intent及派生统计顺序设计，不能撤回已发响应。dispatch标记后崩溃或解析失败不证明上游未收费，不自动全退/重派。

## 未验收

真实三库、Redis×BATCH四组合、旧快照超fence、flush竞争、状态/编辑/删除/用户硬删并发、各事务失败点回滚、重放冲突、outbox重启及uncertain处理均未做资金实现验收。不称H3/H7或P1完成。

后续：精确交易接口/桥接清单，独立复核后另报硬预算，不使用H1额度实施财务。
