# H1-C Type61保存/目录与执行分别审批（2026-11-14）

本批关闭“已准运行的type61插件没有request spec就始终unknown、无法保存/目录发布”的兼容缺口，不启用全局Gate。

## 明确不同权限

- 保存和无requestfilter目录投影只在channel_save/candidate使用独立Role=publication、PathScope=/v1/models。该pair严格限定，v1摘要仍包含精确实际源、有效config、完整binding与mapping/param/header控制。operator必须单独明确批准，不从task/provider批准推导。
- task模型须有当前绑定插件自己声明的canonical model witness；provider可用Meta.RelaysModel(public)真实非管理route witness，不猜默认chat路径。未知plugin/坏settings/request-plugin计划仍拒，不假称builtin。
- ChannelPluginAdmission执行投影改用共享的channel/model/mapping基础helper，明确只接task/provider角色；不会递归调用publication投影或继承其Declared/executor。Attempt/Dispatch无filter也不获得publication执行身份。

## 实际三库

在既有ext/domestic/host_admission_test.go加集中type61fixture：仅runtime批准时ValidateChannelAdmissionSnapshot拒；单独publication批准后真实Channel.Insert及Ability目录包含published-task；publication批准不能通过实际taskdispatch Gate，runtimeAPI拒publication角色，identityAPI拒publication配其他path。替换source字节后目录立即移除且保存拒，未按version标签自动信任。

`ops/.cache/db-test/run-admission-matrix.ps1`实际 `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v` exit0：SQLite3.50.4 2.50s/MySQL8.0.44 0.82s/PostgreSQL17.12 1.41s，包10.339s。其余已有集中runtime/网络/存档等回归同跑。引擎已优雅停止，无旧/生产或收费调用。

## 回归事实

首次 `go test -p2 ./model ./middleware ./relay ./ext/... -count=1` exit1：model/relay/ext通过，middleware唯一失败为TestAccessAndPanicLogsFollowLogRotation的Windows临时日志被占用导致TempDir RemoveAll cleanup失败（完整输出已检查FAIL行，不把system_tasks缺表诊断噪声当唯一失败）。这是历史已有同类基线失败，本批不更改日志实现/官方测试来掩盖，**不宣称全middleware绿**。

随后 `go test -p2 ./model ./relay ./ext/... -count=1` 全过（model8.590s/relay0.412s/domestic2.295s及其他ext）；`go test -p2 ./middleware -run 'Test.*Channel|Test.*Plugin|Test.*Model' -count=1` 3.678s通过；相关vet及完整 `go build -p2 ./...`均exit0。既有官方区diff累计47/48文件、3833/6000增删行，未新增官方文件，保留最后1slot和2167行，不保证全部收口可容纳。

独立只读复核506cd0b6-2763-4116-a405-f5848e641f0b未开发/未执行测试，核对Save/catalog/runtime调用关系及role/hash隔离后未发现本限定范围阻断问题；provider-only/mappedalias组合额外case尚未专项实测，不认证所有插件目录形态。

## 未完成

单独publication审批只解决保存/目录身份，不是所有端点可执行、opaque正文模型授权或目标审批；provider/task运行仍各自role/path，不能因为目录可见就跳实际dispatch。旧pinned generation候选、dynamic affinity、type60/legacy、task-scoped延续grant/凭据刷新、启动operator配置与keycustody等尚未收口。所有Gate noop/Store nil保持，资金同事务RFC独立。
