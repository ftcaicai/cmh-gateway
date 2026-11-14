# H1 policy-only 启动接线（验收进行中）

## 已批准合同

负责人选择 policy-only：在请求及相关 worker 消费策略前完成一致安装；配置失效即启动失败；archive/grants 明确禁用，manifest 有非空 grant 即拒绝。仅允许合成验收，不授权生产启用。全缺省仍为官方兼容 noop，不是错误回退。

## 必须保留的边界

- 四个接口须来自同一次认证的不可变策略：Models / PluginHooks / Executors 同 Gate，Destinations 为同一 manifest 对应具体策略；不是并发热更新，也不承诺跨节点瞬时替换。
- 安装必须在真实 InitResources 的 DB、monitor/auth cleanup 以及返回后的请求/任务/report/system worker 启动前；仅“main 返回后赋值”不够。
- package init 中嵌入式可信 factory 的编译、top-level 和元数据求值已经早于 main。本屏障不宣称阻止它们，也不把编译称为运行期 hook 准入。不得宣称非法配置前零 JS 求值。
- 未解决的 legacy/type60/request plan 等严格拒绝不因启动接线放宽；资金同事务、档案密钥托管/迁移/备份恢复另案。

## 已收取证据

- 实际根二进制 `go build -p 2 -o ops/.cache/policy-startup-proof.exe .` exit0。临时独立工作目录无 .env，清理相关环境并使用故意无效的 SQL DSN（不连接数据库）。partial、bad SHA、bad JSON、非空 grant 四类均 exit1，固定 policy startup rejected，未到 DB sentinel；valid deny-all 与全部缺省均越过策略点，到故意 DB sentinel 后 exit1。六类均无 SQLite 文件。这证明真实接线顺序，不是完整成功 Serving/真实 DB 验收。
- 完整 extension tests PASS（domestic8.388s），extension vet/full build exit0；官方门禁49文件/4404行，named exception14/40不变。独立只读复核无阻断，并实际 focused test PASS0.272s；专用 security-review route不可用，未宣称完整ASVS认证。
- 复核后补强全部六项 globals 在缺省/加载错误/grant错误前后不变及实际四入口deny-all，最终 extension tests PASS（domestic13.535s），vet exit0，job已收取。
- 实施只有 main 早期7行与 ext业务/core桥；未改DB/迁移/schema，不读取档案密钥，无生产启用。既有 factory预main编译例外不变。
- 合成脚本最初因系统脚本策略与无pwsh7失败；改为现有Windows PowerShell5.1兼容API和单进程ExecutionPolicy Bypass后六类成功，未更改系统策略。

仍不据此宣称 P1/P2 完成；原有严格兼容限制及跨节点运维、档案密钥合同未解决。
