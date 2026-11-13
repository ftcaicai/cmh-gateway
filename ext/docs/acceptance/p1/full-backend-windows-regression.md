# 全量后端回归结果（Windows，2026-11-13）

命令：临时 `GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct`，Go1.26.10，`go test -p 2 ./... -count=1`。**exit1，未通过。**

完整本地工具输出：`C:/Users/ftcai/AppData/Local/Temp/dsh-subprocess-iplTBW/dsh-subprocess-24912-3-6f797b78758b-stdout.log`。只存分类不复制原始测试行中可能出现的合成Token/URL全量。

| 失败包 | 已观察事实 | 归因状态 |
|---|---|---|
| controller（602.909s） | 多个SQLite测试TempDir清理报Windows文件被占用；audit.db未关闭句柄。日志也出现未创建system_tasks表警告 | 未在干净基线复验，不能直接认定扩展无关或忽略 |
| middleware | TestAccessAndPanicLogsFollowLogRotation清理oneapi日志失败，同样Windows文件在用 | 待纯基线复验 |
| plugins | TestBuiltInTaskPluginResponsesAndUsageContracts期待17工厂目录，实际18，新增cmh-param-normalize | **明确扩展引入**，需修精确清单或改打包策略，不skip |
| relay/channel | TestUpstreamGetBody_HTTP2CannotRetryWithoutGetBody期待错误含cannot retry err，Windows实际wsarecv断连文案不同 | 待纯基线/同环境复验，不改生产网络代码迎合测试 |

通过的包包括common/model/pkg/jsplugin和本项目ext等。部分数据库矩阵没有MySQL/PostgreSQL环境而skip，因此不能把SQLite测试或全量命令等同三库通过。race因CGO未启用也未执行成功。

## 后续

- 自有插件被 `plugins/embed.go` 整目录embed并RegisterFactory是代码事实；原“不嵌入不注册”说明已纠正，请求实例启用仍独立。
- 最小清单更新提案在 `rfc/2026-11-13-plugin-factory-inventory.md`，需用户确认官方测试例外；不能扩大白名单骗过门禁。
- 当前桥接的99+10官方前端测试、实际5项Chrome表单测试与ext专项测试均通过，但不代表全量回归已通过。
