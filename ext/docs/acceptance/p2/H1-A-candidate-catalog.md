# H1-A 候选与客户目录首批接线验收（2026-11-13）

**状态：首批已接线并通过专项真三库；整个H1/P1/P2仍未完成，业务注册未激活ModelGate。**

## 实际变化

- 共享host映射解析：多跳、base-name fallback、自映射、循环均由官方model API处理，ext不复制规则。
- ChannelSatisfiesFilters覆盖pinned谓词，cached filterCandidateIDs即使filters为空也检查准入，DB候选检查查询失败关闭。被拒高优先级不饿死低优先级合格备选。
- GetGroupEnabledModels过滤实际启用候选；Dashboard静态/插件目录采用每次2次批查询的类型/模型索引，不逐模型查询；Retrieve保持原notfound响应契约。
- 默认core空实现不建立投影或增加DB查询，静态retrieve即使测试DB已关闭仍保持官方行为。
- 本批不改变表结构/数据写入方法。未知映射、坏settings、插件执行者不确定都不能伪装builtin。未安装domestic Gate，默认用户行为不因未完成门禁而受影响。

## 独立只读复核

复核者agent `b1aba4e2-69c1-43ef-8bf2-58c02c86510f` 未参与实现。

两项真实缺陷已在提交前修复并加三库回归：
1. legacy类型1在generic task route可固定sora执行未声明公开别名；单凭Meta.Models不命中就称builtin不安全。现在ChannelModelAdmission接收真实plugin filters，有固定/候选插件身份但尚未制品确认时保持未知执行者；相同type的普通chat仍可用。
2. 禁用channel残留enabled Ability会在DB模式被选中，而cache目录已过滤。现在候选阶段核对channel Status，disabled高优先级能力不能覆盖允许备选。channel_save清理语义未变。

独立复核已再次只读核对两项修复和集中回归，确认可在当前未激活H1-A范围内关闭；未重复执行三库。未再发现默认映射兼容问题；publication元数据owner/endpoint索引仍需按允许候选过滤，属于后续未完成项，不隐藏。

## 真三库环境

| 引擎 | 实测版本 | 隔离 |
|---|---|---|
| SQLite | 3.50.4，Go驱动 | 每次新t.TempDir/admission.db，显式common.SQLitePath并恢复 |
| MySQL | Oracle Community8.0.44 windows-x64 | 独立datadir，127.0.0.1:53316，no-defaults，mysqlx关闭 |
| PostgreSQL | 17.12 x86_64-windows msvc19.44 | 独立initdb，127.0.0.1:55439，SCRAM |

所有服务、数据、测试用户均独立新建于忽略缓存，不读生产DSN或旧系统配置，不注册服务，不改全局执行策略，不输出凭据。环境脚本只在测试子进程加载，CMH_TEST_MYSQL_DSN/CMH_TEST_POSTGRES_DSN缺失时明确skip而非假称通过。本轮两者均实际配置执行。

官方便携制品来源：
- [PostgreSQL Windows页面](https://www.postgresql.org/download/windows/) → [EDB binaries](https://www.enterprisedb.com/download-postgresql-binaries) → [17.12 zip](https://get.enterprisedb.com/postgresql/postgresql-17.12-1-windows-x64-binaries.zip)；SHA256 `A2CD8A774389347B70BE7748AC4A37EF43CB6EE539C5FC3E85AE264B15E4A04C`与发布方.sha256一致。
- [Oracle MySQL8.0.44 zip](https://cdn.mysql.com/Downloads/MySQL-8.0/mysql-8.0.44-winx64.zip)；发布方.md5=`F5E62718C0E77122AEE3476386F48051`核对一致，本地另算SHA256 `4D9316D2955B0BD2B484E08BDA67C18D7979E0622B84F2E9146D97F04ED9A2FC`（不称发布方SHA256）。

## 实际命令与结果

```
# ignored harness加载仅本地测试env，不写凭据到仓库
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1
# harness实际运行
# GOSUMDB=sum.golang.google.cn GOPROXY=https://goproxy.cn,direct
go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v
```

最后包含两项复核修复的三库完整运行exit0：SQLite1.13s、MySQL0.20s、PG1.20s，总7.821s；三引擎同一断言，cache开/关均执行。

断言包括：映射正常/链/自映射/链自映射/循环/base/空目标；helper初始自映射保留request、chain-self更新；DB/cache低优先合格备选、pinned拒绝、未知plugin拒绝、legacy固定未声明模型拒绝、禁用残留能力拒绝、分组目录、类型目录、单模型retrieve、关闭DB failclosed和默认noop兼容。随后增加stale IsModelMapped/base-self/cycle状态断言，完整model/helper/plugins/ext测试通过。

```
go test -p 2 ./model ./relay/helper ./plugins ./ext/... -count=1
go vet ./model ./relay/helper ./plugins ./ext/... ./controller
```

最终exit0：model13.291s、helper2.801s、plugins0.912s、domestic7.032s，其余ext通过；vet exit0。最终 `go build -p 2 ./...` exit0，门禁13/32文件、396/2500行通过，`git diff --check`通过。另误用`-run '^TestModelList'`的controller命令显示`[no tests to run]`，**不计入验证**。

测试故障注入故意关闭DB后调用目录，日志出现database is closed是对应failclosed断言，不是测试失败。测试精确删除91001–91004合成记录再清空cache、关闭连接，未TRUNCATE或删schema。

## 遇到的测试器错误（已纠正而非抹去）

- 起初并行生产API未写完，controller报undefined；等API落地后重跑。
- RelayInfo初始化应填写嵌入ChannelMeta，不是直接复合字面量UpstreamModelName。
- 设置SQLITE_PATH环境变量不会自动更新common.SQLitePath，曾误连默认合成库报index already exists；现显式新TempDir路径并恢复。
- 嵌套PowerShell的变量/引号展开两次失败；改为独立忽略script传递环境。
- PostgreSQL关键词DSN被官方chooseDB当MySQL；改本地test URL为postgres://，不改官方DB逻辑。
- 最初legacy类型owner过度保守阻断普通OpenAI；按真实模型及请求plugin filters改进，未全局放开plugin。

## 关闭与资源

三库验收后用独立mysqladmin/pg_ctl优雅退出；MySQL/PG父级job均exit0已收，监听端口均退出。便携引擎和专属测试库保留在 `ops/.cache/db-test`供后续迁移/写入矩阵；凭据仅在忽略文件中，不提交/不呈现。

## 未完成范围

渠道Add/PATCH/tag/copy/auto-sync/credential-status的写前最终快照原子预验、pricing/vendor/endpoint投影、owner元数据、prebilling/retry/WS每事件/实际plugin计划与最终目的地仍待接线。部分插件或开启任何request-plugin实例时本批暂不证明builtin执行者，不能因此提前启用全局Gate。

这次证明此读查询/候选/目录改动支持三个实际版本，不是最低版本兼容或全库迁移升级/钱包守恒/生产TLS/权限完整认证。全量后端Windows原有失败记录仍有效，不宣称全仓绿。
