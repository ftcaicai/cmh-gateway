# H1-C 持久延续加密档案基础（2026-11-14）

**本批只实现扩展区耐久字节档案，不接生产/启动/轮询，不代表旧任务已经冻结运行。** 官方区本批无变化，累计仍41/48文件、3061/6000增删行。

## 合同与实现

- ext/core/continuation.go值DTO：Binding精确taskID/userID/channelID，非bearer Ref{ID,Digest}，敏感host-only Artifact{pluginKey,sourceSHA,Source,RuntimeJSON}。Load只接受poll/content/report，submit不可用。没有全局installer或noop可伪装已接线。
- ext/continuation/archive.go独立Store：source<=2MiB有效UTF8、摘要一致；RuntimeJSON<=256KiB有效UTF8对象，保留原字节，不宣称schema/canonicalJSON/unique-key验证。
- 标准AES-256-GCM，crypto/rand freshnonce，AAD包含schema1、task/user/channel完整Binding、Ref及KeyID；源/runtime只在密文中，GORMlogger禁用避免SQL细节输出。引用/摘要不是加密，不能替代operator source审批。
- constructor明确提供activeKeyID+32byte keyring，复制输入；无env默认/自动临时生成密钥。支持新key写、保留旧key读，不提供改写/删除/prune或自动销毁旧key。
- immutable primaryRef + insertOnConflictDoNothing后authenticatedLoad/exactpayload比较，重复保存保持原行；篡改行不会被Save悄悄覆盖或修复。
- ext/migrations/continuation.go只管理新cmh_task_continuations，Ref/BindingSHA/Digest/KeyID与密文；size6MiB标签让MySQL使用大容量文本而非64KiBTEXT上限，SQLite/PG由GORM对应text。显式migration未注册，不改官方表/列或自动启用H6。
- 错key/缺key/缺档/绑定不符/损坏统一ErrContinuationUnavailable，cancel保留context错误；这个独立错误将来必须与poll普通transport失败区别处理，当前未改变poll计数/timeout/退款逻辑。

## 真实三库

单一集中ext/continuation/archive_test.go，专属合成DSN、内存固定fixture keys（不是生产key），新表存在即测试失败而不删未知表。fresh migrate2次，写70KiB源码后再次migrate2次，确认原数据/主键幂等保留。SQLite临时文件及各引擎**独立重新打开连接+新Store**恢复原source/config字节，不能叫完整进程重启后task poll验收。

覆盖：重复Save同ref且行数不增；wrong task/user/channel/ref隔离；submit-purpose拒绝；错误/缺失key拒绝；新activekey写+旧key读；constructor输入key突变不影响store；返回buffer突变不影响后续读；sourcehash不符/nullruntime拒绝；ciphertext行无source/runtime明文；另一task有效密文互换AAD失败；腐坏密文Load与重复Save都拒绝不覆盖。

`ops/.cache/db-test/run-continuation-matrix.ps1`加载ignored独立env后运行 `go test -p 2 ./ext/continuation -run '^TestContinuationArchiveDatabases$' -count=1 -v` exit0：SQLite3.50.4 0.06s、MySQL8.0.44 0.18s、PostgreSQL17.12 0.21s，包5.890s。专属引擎已优雅关闭，合成扩展表由fixture清理，无旧/生产数据或付费调用。首个新增schema没有旧发布版本可升级；“已有档案数据上重复migration”已验，未来schema演进仍需released-version升级实测。

扩展全套 `go test -p 2 ./ext/... -count=1` exit0（archive0.159s/domestic7.296s，其他ext通过；未配置MySQL/PG的这次普通回归跳过它们，真三库见上）；`go vet ./ext/...`和 `go build -p 2 ./...`均exit0。独立只读安全复核d455cb0f-9d3c-4b1c-b23a-ec02edd8a6d6未参与开发、未自行执行测试，最终重读完整实现与172行集中测试后未发现本未启用档案范围的安全问题；明确保留密钥保管/备份及跨节点GCM使用上限等部署缺口。

## 安全参考与未完

实现前参考 [OWASP Cryptographic Storage](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html) 的认证加密、CSPRNG、最小存储和key/data分离建议，以及 [OWASP Key Management](https://cheatsheetseries.owasp.org/cheatsheets/Key_Management_Cheat_Sheet.html) 的retained-key rotation/backup/usage限制。该未启用软件constructor不等于KMS/HSM、密钥备份/轮换操作流程或ASVS完整合规；真实部署key来源/权限/跨节点GCM用量上限及备份恢复必须在启动接线之前落实，不自动选择生产密钥或弱默认。

未接官方runtime投影/提交前耐久保存/TaskPrivate安全ref/旧源unpublished CompilePlugin/poll分批/content/report恢复/hold异常分类。Artifact能含敏感配置但不应为了冻结而复制现有API Key/credentialblob；后续host投影仅存必要执行控制并沿用原CredentialAccount，不替换账户或钱账归属。此表没有资金写入，也没有从“pure archive通过”推出结算退款守恒。
