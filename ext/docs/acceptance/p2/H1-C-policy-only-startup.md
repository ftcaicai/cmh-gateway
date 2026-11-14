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

## 实际构建锚点补充验收（2026-11-14）

源码 `5f596065a7e4a890486e24cce2c81832cde836a3`，无生产代码修改。实际EncodeOperatorManifest生成deny-all，原始SHA256为 `760d4043c11fb7b4af6e45cbcb5a7597ae54473974f0a54a2e07c8bcee9e68af`；执行：

```powershell
go build -p 2 -ldflags "-X github.com/QuantumNous/new-api/ext/domestic.BuildOperatorManifestSHA256=760d4043c11fb7b4af6e45cbcb5a7597ae54473974f0a54a2e07c8bcee9e68af" -o ops/.cache/build-anchor-proof.exe .
```

构建exit0，二进制SHA256 `68916329EFAA2FCA7E1EF24F900F79088A966F3B84B7D2E8E66000F24A4CBEF2`。三个实际子进程均使用全新无dotenv目录、清空继承环境，仅明确合成配置和故意无效SQL_DSN：

| 场景 | exit | 固定policy拒绝 | 到达故意DB失败 |
|---|---:|---|---|
| 正确文件，仅build SHA，无env SHA | 1 | 否 | 是 |
| 正确文件，冲突env SHA | 1 | 是 | 否 |
| 文件追加LF，保留原build SHA，无env SHA | 1 | 是 | 否 |

全部无SQLite文件，进程已退出，构建job已收；父代理复核执行脚本的隔离、错误断言及finally清理。证明真实链接注入、build-only锚点及env不能覆盖build锚点，不是成功Serving、真实DB、制品来源签名或生产启用证明。预main factory例外不变。

仍不据此宣称 P1/P2 完成；原有严格兼容限制及跨节点运维、档案密钥合同未解决。
