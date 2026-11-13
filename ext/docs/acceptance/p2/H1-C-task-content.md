# H1-C Task内容最终目标（2026-11-14）

本批限定已持久任务内容代理的最终HTTP目标，不证明source/config归属冻结或钱账一致性。所有Gate仍默认不安装。

## 实现

- controller/video_proxy.go沿用已加载的渠道，checkTaskContentDestination要求持久task.ChannelId与实际channel.Id匹配、type正数、Origin非空；Upstream为空使用持久Origin回退。exact task_content关系包含actual Method/URL摘要/有效Host摘要和持久模型/渠道，不套当前模型发布或渠道启用门禁，不伪造Executor/Hook。
- 初始目标检查在descriptor/client Range等headers确定后、doTaskMediaRequest之前；每个redirect在原crossorigin credential规则、credentialless清理及client header恢复后重新检查，再发送下一跳。
- fixed artifact403错误，不回传策略私有错误或signedURL。原owner/auth入口、SSRF/proxy/TLS、Range/HEAD、媒体超时、contentPath规则未改。data: inline内容本地解码，无网络可审批，保留原路径。

## 真实测试与审查

扩展既有controller/task_generic_test.go的TestTaskContentDestinationAdmission，同一DB fixture配真实loopback HTTP：

- 空审批初始target network0，捕获DTO精确来自持久task/channel，不是当前发布模型。
- exact批准后206 Range/body/Content-Range/private-no-store保持。
- credentialless cross-origin redirect第一跳允许、第二未批，第二服务network0；第二精确批准后正常媒体响应。
- 旧任务Upstream为空时精确回退Origin；缺对应回退关系先拒，单独批准后成功。

测试调用实际private proxyTaskMedia，不冒称公共owner/auth整个端点E2E；原公共VideoProxy/GetTaskArtifactContent授权流程未修改，源码核对均在原owner/auth后收敛此函数。

只读独立复核b4d9a264-7466-4ee0-922f-af30f8b1f232（未开发/未测试、无DB/生产收费调用）核对初始/redirect时机、真实Host fallback、私有错误脱敏、Range/HEAD/SSRF和client浅复制保留，未发现限定target-only范围阻断fail-open。额外建议的空Upstream回退已加入实测。

## 实际验证

忽略本地脚本ops/.cache/db-test/run-content-destination-matrix.ps1只加载专属合成DSN，设置TEST_TASK_DB_DIALECT与官方test DSN变量，循环执行：

`go test -p 2 ./controller -run '^TestTaskContentDestinationAdmission$' -count=1 -v`

SQLite3.50.4：test0.02s/package0.334s；MySQL8.0.44：test0.29s/package0.595s；PostgreSQL17.12：test0.16s/package0.468s，全部exit0。每引擎unique table prefix、自动迁移合成Task/Channel/User并清理，无官方schema改动。

定向媒体/目标controller测试0.523s通过；`go test -p 2 ./service ./relay/channel/task/jsplugin ./ext/... -count=1`通过service14.233s/task1.336s/domestic2.127s及其余ext；相关vet及完整go build -p2 ./...均exit0。进程GOMAXPROCS2/Go镜像，无全局设置。专属MySQL/PG已优雅退出，无生产/旧系统/付费调用。

累计41/48官方文件、3061/6000增删行，精确登记实现与既有官方测试文件。仍留未完：持久source/config/目标能力冻结、重启后旧版本恢复、poll failure与资金恢复语义、candidate generation/affinity、Save目录/type60和启动配置加载。目标准入不是旧任务执行权限的完整替代，不能提前激活。

## 持久延续接续事实

本轮只读调查确认现有TaskPluginSnapshot为审计元数据，禁止放raw source/secrets；Registry只保留current，不存在跨重启历史artifact档案。官方版本表有SourceHash/Source可查，但版本可被删除、factory仅当前embed，不能当耐久archive保证。poll错误会累计并自动终态/退款，缺档案不能随意当一般transport失败。下一步需扩展区不可变档案与task-bound reference、精确旧源未发布编译恢复和可恢复缺档类，不修改钱包归属或把当前撤销变成丢弃既有财务能力。
