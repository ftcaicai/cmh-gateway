# P1 纯值资金事件原型（非接入验收）

## 范围

按未批准资金实施 RFC 的“先以纯值类型和合成用例验证契约”约束，新增隔离包 `ext/fundingcontract`。没有 DB、Redis、批量队列、价格/token API、全局 hook 或官方资金调用；不能导出为真实余额或用它替代官方钱包。

同一 synthetic asset 分成 Available / Reserved / Consumed 三个状态。Reserve/Append 从可用移到预留；Settle 从预留移到已消费；Refund 仅释放尚未消费的预留。Delta 有符号分量之和为零，三状态总和等于合成期初值。Consumed 是合成 sink，不是平台实收；这不是多账户借贷账本，也不证明客户/OEM/代理跨账户守恒。这里 Refund 不是已消费资金的售后退费接口。

不可变 tenant/request/source binding；同一 request 内跨事件种类 ID 不得复用：完全相同重放返回零 delta，改变 kind/amount 拒绝，改变归属拒绝。新 State 复制历史，失败不改原 State。整数限制为 2^53−1，非负余额及边界先约束，最大三个分量求和不越 int64；无浮点转换、透支、隐式补款或舍入。

Violation 仅为独立 ID 的零金额观察，允许出现在预留前后，不代表应收费或可以从退款后钱包扣款。真实违规费金额、资金来源及恢复规则保留待决，不擅自制定。

## 验证与退出边界

集中确定性12用例覆盖部分结算/释放、追加、终态、重放冲突、三类归属错配、负数/超额/极值、分支不可变与失败保留。最终独占源码 `go test -p 2 ./ext/fundingcontract -count=1 -v` PASS 0.684s，相关vet exit0；整包 `go test -p 2 ./ext/... -count=1` PASS（domestic8.479s），extension vet及完整 `go build -p 2 ./...` exit0。并行交接后移除重复观察断言并重跑上述最终集中测试。独立只读复核无阻断，并另跑测试0.095s及vet通过。官方门禁保持49/4397，本轮没有官方改动。

**不满足且不宣称：**真正同事务、唯一约束落库、token 原子更新、财务终态恢复、并发/重启/故障注入、真实三库迁移、Redis×BATCH 守恒、真实账本分类或 H3/H7 接管。资金实施 RFC 仍未批准；本包没有被生产代码导入或安装。
