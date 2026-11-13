# H1-A 控制器、上游模型应用与通用列写入（2026-11-13）

**本批专项完成；全局ModelGate未安装，H1整体仍待预扣/重试/长连接/实际插件制品与目的地边界。**

## 实现

- AddChannel在请求结构规范化后预验实际身份（status默认与keyless-login状态），先于凭据检查和持久化；Update在读取当前行后、repair-capable getter/首次Key写之前预验GORM有效补丁。使用既有翻译键Invalid request，拒绝不输出上游或凭据。
- generic UpdateChannelOtherInfo在门禁激活时走锁内事务：复制Mapping/Setting字符串与隔离Key状态图；只根据返回的真实持久化列构造有效身份，不接受callback未写入的内存假值。GORM字段/数据库列别名规范化，重复别名、未知字段、身份不支持类型、SQL表达式/GormValue/subquery均拒绝。状态/能力或路由重建同事务，提交后同步。
- generic映射/绑定变更也发布通道同步修订；不只是models/status才同步。仅身份不扩大、Key隔离不恢复、other_info只改status原因/时间的限制性更新可在政策撤销后保存。
- upstream自动/手工模型apply共用写入函数：选定delta应用于锁内当前Models，保留当前类型/映射/绑定；原OtherSettings快照变化则拒绝重试，不能用旧discovery覆盖新operator ignore/pending；实际模型/能力同事务。
- 新UpdateWithAdmissionKey共用原active Update事务路径，将Key+补丁+Ability作为一次原子编辑；本地callback计算当前Key派生值，事务内最终身份检查；失败不发布Key/cache，成功后才同步与撤销旧凭据。默认noop保持官方原分支。

## 独立复核与真实先失败后修复

复核者 `6aec54d2-4761-42ff-aad8-9c7b062f11cf` 只读指出：
1. Mapping/Setting-only generic写未发同步，DB/cache身份不同。已加入实际delta后同步与修订递增断言。
2. 锁内仍覆盖旧ignored/pending数组会丢并发操作意图。已以OtherSettings快照冲突拒绝；通过GORM查询callback精确交错修改，断言新ignored保持、Models未变，随后新快照重试成功，无sleep。
3. 控制器Key先提交、最终补丁另事务会导致失败请求仍换Key，且旧拒绝mapping修复+换Key被旧身份拦截。新增测试实际复现：Ability后段故障后Key从fixture变must-roll-back-key，另combined repair失败。随后统一到atomic API，完整真三库通过。不是把该缺陷归为历史问题或只写限制。

第二位复核者 `9a80eace-8cb5-4a6c-931a-19c0ff8d295b` 只读核对最终修复，确认上述限定问题关闭；未重复跑矩阵，也不认证完整H1。

## 真三库

同一已扩展的 `ext/domestic/host_admission_test.go`，SQLite3.50.4、MySQL8.0.44、PostgreSQL17.12均实际执行；独立loopback引擎，无生产/旧系统/付费调用。

```
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1
# 加载忽略目录仅本地测试DSN后运行
go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v
```

最终run exit0：SQLite2.00s、MySQL0.31s、PG1.22s，包9.329s。包含全部旧候选/目录/价格/写入回归及本批：
- Add实际handler整批拒绝且无channel落库；Update拒绝模型补丁前Key不变。
- generic map数据库/Go别名、返回身份与callback不一致、string type/Expr/重复别名拒绝。
- mapping-only允许写触发sync revision。
- Apply非白名单模型拒绝并保留pending；允许模型delta成功并创建enabled Ability。
- 旧discovery与新忽略设置确定性交错，旧提交拒绝，不覆盖当前意图；再次重试成功。
- Controller后段Ability故障Key/Name同时回滚；旧拒绝mapping与Key一次修复成功。
- Key-only API继承存储multi-key信息，2个Key追加成3，MultiKeySize确为3。
- generic callback原地删除隔离Key状态不能获得限制性豁免。

最初正向Apply测试未显式初始化Redis/审计上下文而panic，现fixture显式关闭未初始化Redis、设操作者/角色、InitLogDB同库审计并恢复全局。未修改生产逻辑迎合测试。并行源文件未写完的undefined/unused编译错误已纠正重跑；无界并发compiler panic按已知工具链问题以 `-p 2`构建，不写为代码通过。子代理controller `-run '^$'`仅编译，不计行为测试。

## 相关模块回归与构建

`go test -p 2 ./model ./relay/helper ./plugins ./ext/... -count=1`最终exit0：model6.706s、helper8.090s、plugins5.782s、domestic2.292s，其余ext通过。随后 `go vet ./model ./relay/helper ./plugins ./ext/... ./controller` 与 `go build -p 2 ./...`均exit0。没有把controller的空筛选编译检查算作行为测试；实际handler行为在真三库集中测试执行。

## 范围限制

- 本批没有上游真实收费调用；自动discovery网络preview不等于发布，自动分支共享保存函数已接线，但尚未单独跑真实auto模式HTTPstub全链路，不把manual apply覆盖称完全等价E2E。
- 当前身份字段未包含所有BaseURL/override/plugin运行产物，后续需同步扩展有效快照合同，仍不可激活部分门禁。
- Key/编辑单事务只在门禁激活分支，默认官方行为不重构。plugin.CheckKey为本地hook且API禁止utils.fetch，不把它误称已用网络计数验收。
- 三库证明当前实测版本和事务回归，不是所有数据库最低版本/旧数据升级/金融守恒验证；H3/H7未实现。
- 单个actor授权middleware未重写，fixture直接挂handler用于控制器契约，不称全套权限认证E2E。
- 累计官方区19/32文件、1444/2500增删行，逐文件登记；全仓Windows历史失败仍不写全绿。

## 资源收尾

两本地测试DB已优雅停止，服务父job均exit0；保留ignored便携引擎/测试数据以便继续复验，不提交DSN、随机密码或SQL数据。
