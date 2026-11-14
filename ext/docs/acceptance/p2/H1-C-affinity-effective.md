# H1-C：亲和参数 base / effective 双精确准入

日期：2026-11-14；基线：官方 v1.0.0-rc.51。仅在既有 H1 登记范围补齐有限兼容，不启用全局策略、不改变资金生命周期。

## 问题与合同

原 Setup 在审批存储渠道后才执行官方 affinity 模板合并；预扣前和真实执行前又要求 actual 参数摘要与 fresh base 全等，导致已选择的合法模板也一律拒绝。不能用忽略参数摘要解决。

本批要求 **当前 fresh base 与最终实际 effective 两个完整 executor identity 各自获独立审批**。候选仍使用 stored base，不预测模板；Setup 执行唯一一次官方 merge，检查同一 merged map 后才发布新 selection context / 取 key。运行时先检查 fresh base，再只用 selected ParamOverride 替换 fresh 副本，要求其全部 model/executor 与实际 selected 投影相等，最后检查 effective。source/config/base/mapping/header 等其它配置不放宽。

这证明当前 base 获批、最终 effective 获批及除 ParamOverride 外配置一致，**不证明模板来自哪条 affinity 规则，也不冻结旧 base 参数的历史值**。新 base 与旧 effective 若各自获批，允许继续；未批的新 base 仍拒绝。失败 Setup 不承诺清空同 context 原有 key；调用方必须沿既有错误路径中止，本批没有重写重试清理机制。

## 实际接线与集中验证

- model/channel_constraint.go：CheckPluginExecution 双边界，provider 与 task 新提交 adaptor 原本均真实调用该 helper。
- middleware/distributor.go：存储 base 审批后、发布新 selection context / GetNextEnabledKey 前进行一次官方 merge 与 effective 审批；同一 map 写回原上下文。
- relay/request_billing.go：重建实际 selected context 后复用统一检查，仍先于估价/免费跳过/预扣；不改账务方法。
- ext/domestic/host_admission_test.go：沿公开 GetPreferredChannelByAffinity 匹配真实 header 规则（cache miss 仍有模板）→Setup→预扣前检查→provider 真实本地派发；上游收到 base_marker 与 affinity_marker。operations 恰好两条，防止重复合并。base-only 候选可入围但 Setup 拒绝且不发布 key；effective-only 候选和 Setup 均拒绝。撤销任一审批、选中后 context / RelayInfo 参数篡改、未批 fresh base 均拒绝，准备后的 provider 不再联网。预扣拒绝探针没有估算/Reserve 调用，**不是资金守恒认证**。
- 既有 pinned A/current B、其它配置漂移、任务源/目标 veto 与有限授权回归一起运行。首批新增 affinity positive 仅 provider wire；后续第41轮补充 task 的有效 envelope、捕获及撤销验证，见下节；不等于全协议覆盖。

### 第41轮任务合同补验

集中 fixture 新增 `affinity_exact_envelopes/task_submit`：公开 affinity header 规则匹配与实际 Setup 选择，stored base / effective task+path 身份独立审批，实际 ValidateRequestAndSetAction → BuildRequestBody → DoRequest 向本地 stub 提交。选中 operations 长度2，CaptureContinuationArtifact 的 RuntimeJSON.ParamOverride 与选中 effective 完全一致；准备后的同一 adaptor 撤销任一审批均在 hook / 新网络前拒绝，reservation probe 为0（不是资金守恒证明）。

首次真实 wire marker 断言 RED：实际请求为 `{"model":"affinity-task","prompt":"synthetic"}`，没有 base_marker / affinity_marker。源码与实际调用一致：task 描述符归插件所有，task 路径不经过普通 provider 的 ApplyParamOverride。最终按真实 descriptor wire 断言，不人工应用 override，不修改生产语义，**不能声称 task operations 已应用到上游 body**。初次失败已调查，非删除应有安全断言。

最终 focused SQLite3.50.4 PASS：package7.761s，task子用例0.02s。集成 ext 全测试 / task 包测试及对应 vet exit0：ext/domestic7.672s、task1.365s。最终完整三库 exit0、package21.236s：SQLite3.50.4 7.58s / MySQL8.0.44 6.67s / PostgreSQL17.12 6.72s；task_submit 分别0.02/0.01/0.01s。两独立引擎已优雅停止，相关 jobs 均收取。

## 命令与结果

本地缓存 Go1.26.10；临时 GOSUMDB=sum.golang.google.cn、GOPROXY=https://goproxy.cn,direct、GOMAXPROCS=2。

```powershell
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File 'D:\work2\cmh-gateway\ops\.cache\db-test\run-admission-matrix.ps1'
# 脚本载入 ignored 合成 DSN，执行 go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v
go test -p 2 ./model ./relay ./relay/channel/providerplugin ./relay/channel/task/jsplugin ./service ./ext/... -count=1
go test -p 2 ./middleware -run 'Test.*Channel|Test.*Plugin|Test.*Model' -count=1
go vet -p 2 ./model ./middleware ./relay ./relay/channel/providerplugin ./relay/channel/task/jsplugin ./service ./ext/...
go build -p 2 ./...
bash ops/ci/check-upstream-diff.sh
```

最终真实数据库矩阵 exit0，package 21.946s：SQLite3.50.4 8.27s、MySQL8.0.44 6.62s、PostgreSQL17.12 6.77s；新增 affinity 组分别0.04/0.02/0.01s。两独立 loopback 引擎已优雅停止，启动 jobs 已收取；无生产或外部模型调用。

最终相关测试 exit0：model8.964s、relay0.426s、provider0.177s、task1.344s、service14.445s、ext/domestic8.584s；middleware 上述定向8.501s。对应 vet / 全 build 均 exit0，最终 job 已收取；不把定向 middleware 当全包通过。Windows 全量占用 cleanup 历史、race / 最低数据库版本缺口仍保留。

首次新增候选断言错误使用仅协议路径 FilterRequestPath，未提供 provider executor witness，使 base-only 在三个数据库都失败；调查确认 candidatePluginRole 只认实际身份约束，改 fixture 为真实 FilterTaskPluginIdentity 空 key 的 provider witness 后重新全矩阵通过，未放宽生产检查或删断言。

## 复核与预算

独立只读 agent 对最终三个生产文件及两 adaptor 实际调用链复核：有限合同下无阻断发现；不等于专用 security-review 工具审计（当前 schema 无该 route），未宣称 ASVS 完整合规。集成者复核最终 diff，并执行上列回归。

相对 rc51 门禁49/49官方文件、4398/6000增删行，剩1602行/0任意新文件；命名 middleware/task_plugin.go 例外仍14/40。本轮仅复用三个已登记生产文件，未修改该例外；80%范围复核保持。所有全局 Gate 仍 noop、TaskContinuations / ContinuationGrants 仍 nil。type60/legacy/requestplan、完整启动/密钥运维、P1同事务资金仍未完成。
