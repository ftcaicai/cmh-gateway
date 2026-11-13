# H1-A 渠道持久化与价格关联目录（2026-11-13）

**本批已实现并专项验收；H1整体未完成，core.Models仍未安装业务实现。**

## 接线范围

1. `model/channel.go`：单个/批量新增、部分Update、tag编辑/启用、手动/自动状态；开启门禁时以实际有效快照预验。Update按官方GORM非零值/非nil语义合并当前身份字段（Type/Status/Models/ModelMapping/Setting），不使用全Save覆盖Key。默认空实现保留原路径。
2. 新增channel与Ability同事务；tag锁定按ID排序的集合，全批预验后只写该集合；状态和Ability.enabled同事务，提交后才更新缓存/同步主题。
3. 三种凭据callback写前只验证实际持久化字段对应的身份，不允许callback未写入的模型/类型/Setting改变伪造授权；冻结Mapping/Setting指针指向的字符串，避免浅拷贝污染前像。
4. `model/pricing.go`：真实允许Ability/channel先过滤，后生成模型/vendor/group/endpoint。无关exact元数据和禁用元数据不进入投影；只列已引用供应商。当前插件执行者尚无制品证明，不能凭同名元数据附加plugin schema/variant/别名继承价格。
5. 价格读取在门禁激活时重新构造权威视图，任何查询失败先清空全部派生目录，不返回一分钟旧缓存。默认价格数值、表达式、计量与扣款逻辑未改。
6. `model/model_meta.go`：owner只从符合准入的实际候选按原优先级/weight/ID顺序选择，批量查询，无逐模型N+1。

## 独立复核及修复

复核者 `4a185f9c-b965-4bfb-b49c-9b543d7694b3` 未参与开发，仅只读审查，未重复运行测试。

- P1已修：callback前像原来是浅拷贝，原地修改 `*ModelMapping`/`*Setting`可把未持久化的允许身份传给准入，但实际写入仍是旧被拒身份。三个callback现在在执行前复制指针值，只叠加真正写入列；三库新增指针攻击回归。
- 父任务审查修复：active Insert单独Create/AddAbilities会留下半成品，现同事务，并通过真实ORM能力写入故障注入证明回滚；status和Ability变更也纳入事务。
- P2宿主Key隔离已修：政策撤销后，单Key禁用但其他Key仍启用时不能被整个channel准入挡住。受控UpdateChannelStatus仅在请求Enable时验准入，禁用仅更改状态/隔离元数据，不扩大执行权限。新两Key回归证明key0隔离可持久化；再次Enable未解除隔离（同状态提前返回也会拒绝，不能专称该条证明了政策检查执行）。其他任意callback的Key-only维护在政策撤销时仍可能失败，未声称已统一解决。
- 允许tag更改标签/分组/优先级/权重的Ability结果也直接断言，不只测拒绝。

## 真三库验收

复用独立loopback测试引擎：SQLite3.50.4，Oracle MySQL8.0.44，PostgreSQL17.12 windows-x64。无真实凭据或旧业务，随机测试账户保存在ignored cache，未注册全局服务。

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1
# 内部实际执行，进程专用测试DSN及GOSUMDB/GOPROXY
go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v
```

最后包含所有修复的三库同一测试exit0：SQLite1.59s、MySQL0.35s、PG1.60s；包总8.205s。测试重复运行于已有本地库，主库InitDB迁移重复启动成功；本批未改schema，**不将此等同全版本升级兼容或最低版本验证**。

集中测试扩展已有 `ext/domestic/host_admission_test.go`：
- 后一个批量渠道拒绝，前一个也不落库。
- 单个Insert能力写入失败，channel回滚；GORM callback只用于故障注入，底层仍是真数据库。
- mapping-only部分Update拒绝并保留旧模型/类型；name-only允许且保留mapping。
- tag全部预验拒绝、正常tag/分组/优先/权重结果。
- 拒绝manual/credential/scheduled/task-plugin恢复启用；callback修改未持久化identity及原地改pointer均不能洗白身份，Key/Status仍原值。
- 禁用清理仍可用，两Key隔离场景。
- 被拒高优先级owner不能覆盖合格候选；仅允许模型价格、引用vendor、允许endpoint，无denied/ghost元数据；撤销最后允许channel后暖价格缓存即时清空。
- 关闭DB故障后价格/vendor/endpoint/group视图不回退旧数据。日志中的database closed是故意故障注入的预期观察，不是失败。

## 回归和构建

```powershell
$env:GOSUMDB='sum.golang.google.cn'
$env:GOPROXY='https://goproxy.cn,direct'
go test -p 2 ./model ./relay/helper ./plugins ./ext/... -count=1
go vet ./model ./relay/helper ./plugins ./ext/...
go build -p 2 ./...
```

全部exit0：model13.962s、helper13.069s、plugins6.311s、domestic6.456s，其余ext通过。最终完整构建成功。门禁17/32官方文件、964/2500增删行；git diff --check通过。远程CI另查提交后结果。

实现前新增写断言曾捕获未接线的允许写入；并行文件尚未完成时曾遇helper未定义编译失败，生产写完重跑。初始子任务Go默认GOSUMDB=off导致工具链校验失败，均用进程环境纠正，不改仓库或全局配置。指针漏洞测试在修复落地后编译通过，**不虚称该项先红后绿**。

## 未完成与上线限制

- 全局Gate未激活；controller可能先有Key/网络等side effects，再触发本层持久化拒绝。controller预验与upstream模型自动同步共享写入口、generic UpdateChannelOtherInfo列更新仍要接。
- 当前身份只覆盖公开/最终映射模型、类型及已知builtin；BaseURL/ParamOverride/HeaderOverride、实际plugin制品与网络目的地仍需后续检查，不能开启部分门禁对外服务。
- 无并发竞态专项证明，不把单线程三库测试当作所有并发/租户/财务原子性证明；锁/事务设计经过代码复核，后续按入口增加确定性并发测试。
- H3/H7资金同事务方案未落地，不改钱包、quota、价格公式或上游usage；本批不是资金守恒验收。
- 全仓Windows已知原基线失败仍保留，不宣称全部仓库测试全绿。
- 三库服务已优雅关闭，后台输出已收；专属引擎与测试库留在ignored cache供后续复验，不提交任何凭据/数据。
