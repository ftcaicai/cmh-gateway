# Windows 官方 rc50 纯基线定向复验（2026-11-13）

## 结论与边界

对照 [当前全量回归记录](<ext/docs/acceptance/p1/full-backend-windows-regression.md>)，在同机 Windows / Go 1.26.10 上实际运行原版 rc50 的三个指定测试：

- **controller：复现** `TestAuditDatabaseMatrix` 的 SQLite fresh / upgrade 两个子用例在 TempDir 清理时无法删除仍被占用的 `audit.db`。
- **middleware：复现** `TestAccessAndPanicLogsFollowLogRotation` 在 TempDir 清理时无法删除仍被占用的日志文件。
- **relay/channel：本次未复现**；同名测试单次执行通过。不能把之前的 `wsarecv` / 错误子串差异认定为已由纯基线复现，也不能据单次通过判断问题不存在。

仅证明以上具体测试/症状；不是全量基线通过，不把所有失败概括为“环境问题”，不据此豁免现有门禁。插件工厂清单失败不在本次执行范围内。

## 来源、隔离与完整性

- 原始 tag：`v1.0.0-rc.50`。
- 解析到的官方 commit：`f2aa91232ed3e5878a614762ace6736a2b74257f`。
- 官方 tracked tree：`e2d94be65a94c8db3bc8a662181dd20c3992ed01`。
- 归档前检查 D: 可用空间 `866219986944` 字节；tag 共 2826 个 tracked blob，内容合计 `30708313` 字节。目标此前不存在。
- 归档是本地 Git 对象导出的源码，不是新仓库、clone 或共享历史 worktree；没有依赖/模块缓存拷贝，没有源文件或测试补丁。
- 实际测试目录：`D:\work2\cmh-gateway\ops\.cache\rc50-baseline\src-lf`；整个 scope 被既有 `.cache` ignore 规则忽略。
- 实际使用的 [归档](<ops/.cache/rc50-baseline/rc50-lf.tar>) SHA-256：`7894f49674c50eae37ca6a0ef69e4f27e3bb9165da931ddafc71ecb025ce59d4`。
- 归档前后分别对全部 **2826** 个文件执行 `git hash-object --no-filters`，与 tag 的原始 Git blob ID 比较：两次均 **0 差异**。不是只比较三个测试文件。

实际生成命令（从当前 checkout 根目录执行）：

```powershell
git -c core.autocrlf=false -c core.eol=lf archive --format=tar --output=ops/.cache/rc50-baseline/rc50-lf.tar v1.0.0-rc.50
tar -xf ops/.cache/rc50-baseline/rc50-lf.tar -C ops/.cache/rc50-baseline/src-lf
```

最初默认 `git archive` 的尝试受 Windows 换行配置影响，37 个非 Go 文件（含模块清单）的原始 blob hash 不同；仅关 `core.autocrlf` 仍不足，最终同时指定 `core.eol=lf` 后全部匹配。前两次归档/解包目录保留在同一忽略 scope，**未用于测试**。首次列出 tar 内容经 `Select-Object -First` 截断造成管道退出非零；随后完整 tar 列表检查及解包均 exit 0。未覆盖或删除已有路径。

## 运行环境与安全界限

每个测试进程仅临时设置：

```powershell
$env:GOSUMDB='sum.golang.google.cn'
$env:GOPROXY='https://goproxy.cn,direct'
```

- `go version go1.26.10 windows/amd64`。
- 未设置 `CGO_ENABLED`；Go 默认实测为 `0`，没有强行开启/关闭 CGO。
- 共享原有 `GOMODCACHE=C:\Users\ftcai\go\pkg\mod` 与 `GOCACHE=C:\Users\ftcai\AppData\Local\go-build`，未创建隔离依赖缓存。
- `GOWORK`、`GOFLAGS` 为空。未读取、复制或记录任何凭据值。
- 检查 `SQL_DSN`、`LOG_SQL_DSN`、`REDIS_CONN_STRING`、`AUDIT_MYSQL_DSN`、`AUDIT_POSTGRES_DSN`、`AUDIT_CLICKHOUSE_DSN` 均未设置；归档只有示例环境文件，无私有环境文件拷贝。
- controller helper 为 SQLite 使用 `t.TempDir()/audit.db`；矩阵自行设置 `SQL_DSN=local`、清空 `LOG_SQL_DSN` 并执行迁移。外部 MySQL/PostgreSQL 分支仅在对应 `AUDIT_*_DSN` 非空时运行，本次按原 helper 跳过，**没有三库通过结论**。
- 此矩阵调用的默认 `migrateDB` 包含 `SystemTask` / `SystemTaskLock`，无需手工补表或改测试。本次该 focused matrix 输出未出现 `no such table: system_tasks`，因此没有复现全量记录中其他测试的这类告警。
- middleware 使用临时日志目录；relay 使用测试内 `127.0.0.1:0` 监听器和注入 transport。没有生产服务启动、生产 DB/Redis 连接或收费上游请求。

## 精确执行与状态

时间均为 `2026-11-13`、`+08:00`。以下命令工作目录均为上述 `src-lf`；未执行全量 suite，也未修改测试使其通过。

| 实际命令 | 起止时间 | exit | 结果 |
|---|---|---:|---|
| `go test -p2 ./controller -run '^TestAuditDatabaseMatrix$' -count=1` | 14:37:04.868–14:37:06.386 | 1 | 命令拼写问题，根包 setup 失败，**未执行 matrix** |
| `go test ./middleware -run '^TestAccessAndPanicLogsFollowLogRotation$' -count=1` | 14:37:04.876–14:37:28.437 | 1 | 指定测试失败；包耗时 11.957s |
| `go test ./relay/channel -run '^TestUpstreamGetBody_HTTP2CannotRetryWithoutGetBody$' -count=1` | 14:37:04.918–14:37:20.897 | 0 | `ok .../relay/channel 5.619s`，未复现 |
| `go test -p 2 ./controller -run '^TestAuditDatabaseMatrix$' -count=1` | 14:38:13.395–14:38:33.650 | 1 | 指定矩阵失败；包耗时 12.386s |

任务原命令中的 `-p2` 不是此 Go 的 `-p 2` 写法；Go 的 unknown-test-flag 处理导致后续包名不再成为包列表，尝试编译根包，报 `main.go:45:12: pattern web/dist: no matching files found`。保留这条失败而不把它冒充 controller 运行证明；经父代理确认后规范为空格形式，再实际执行同一 focused matrix。没有为此构建/添加 `web/dist` 或扩大测试范围。最初在未覆盖继承的 `GOSUMDB=off` 时，Go 版本检查报 checksum database disabled；按指定进程环境设置后得到上述准确工具链版本，全部测试均使用指定值。

### 精简失败文本（仅省略本机临时目录前缀）

```text
--- FAIL: TestAuditDatabaseMatrix (7.04s)
    --- FAIL: TestAuditDatabaseMatrix/sqlite (7.04s)
        --- FAIL: .../upgrade=false (3.63s)
            database version: 3.50.4
            TempDir RemoveAll cleanup: unlinkat <TempDir>\001\audit.db:
            The process cannot access the file because it is being used by another process.
        --- FAIL: .../upgrade=true (3.41s)
            database version: 3.50.4
            TempDir RemoveAll cleanup: unlinkat <TempDir>\001\audit.db:
            The process cannot access the file because it is being used by another process.

--- FAIL: TestAccessAndPanicLogsFollowLogRotation (1.88s)
    TempDir RemoveAll cleanup: unlinkat <TempDir>\001\oneapi-20261113143726.log:
    The process cannot access the file because it is being used by another process.
```

未复制控制器日志中合成 token、hash、SQL 行值或整份输出。矩阵输出的预期唯一约束错误不是本次失败依据；失败位置为 cleanup。helper 的 SQLite 分支返回已打开的 DB，却不像外部 DB 分支登记 Close cleanup；矩阵还重复 `InitDB` 打开连接。这与观察到的文件占用相符，但未做句柄级跟踪，不扩大为所有 controller 失败的根因证明。middleware 清理前也没有关闭最终 logger 句柄；仅记录本次具体症状。

## 与当前 suite 的逐项核对

原始全量输出只做局部读取/搜索，没有复制全量数据：

- 当前 matrix 同名两个 SQLite upgrade 子用例均报同样 `audit.db` Windows 清理失败；纯 rc50 **匹配**。
- 当前 rotation 同名测试报同样日志被占用清理失败；纯 rc50 **匹配**。
- 当前 HTTP2 同名测试报 `wsarecv: An existing connection was forcibly closed by the remote host.` 且不含断言要求的 `cannot retry err`；纯 rc50 本次通过，**未匹配**，仍待后续受控调查，不能归因为扩展无关。
- 当前 checkout 三个测试文件相对 rc50 的 `git diff` 为空。原始 Git blob ID：controller `dde488636732f931f45b4719bf17cbdb570976ca`；middleware `1116f4dec93c8fa6e8968d6c8629964286258b78`；relay `4370e22b3429f2046c9c80920a563de1112f4ed9`。相同测试源码不等于完整运行条件相同。

## 保全与任务收尾

- 起始对当前 checkout 2890 个 tracked 文件记录 SHA-256；测试后第一次完整比较为 **0 文件内容变化**（含父代理原有修改）。证据落盘后的再次比较，仅 [CURRENT_TASK.md](<ext/docs/CURRENT_TASK.md>) 出现并发的父代理所有文档变化，本 worker 未编辑它；其余 2889 个既有 tracked 文件仍全部相同，**当前源码未变**。本 worker 唯一非忽略新增交付为本文档，未覆盖其他人的文件。
- 本 worker 未执行 commit、push、生产操作或源码编辑。共享 checkout 的 HEAD 在期间由起始 `26be9626c97341c4d991abc6c7e506df07bc8181` 变为 `2d4fc7721f3550d974cea7a3f416c707ad31ae3b`；不声称 HEAD 未变，内容保全由逐文件 hash 独立证明。
- 四个托管 jobs 均已逐一用 `job_output` 收取：`pwsh-218afd01fb6e4549`（拼写失败）、`pwsh-f6f9304f47954463`（middleware）、`pwsh-98d304215bda4c57`（relay）、`pwsh-1804aa2e75ac432a`（规范 controller）。`job_list` 全部 completed；**本 worker 活跃 jobs：0**。
- 归档及完整性清单仍保留于忽略 scope，未在证据落盘前清理。
