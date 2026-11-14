# 官方rc51同步验收（2026-11-14）

状态：真实release合并与回归进行中，不把无冲突自动merge称所有验收通过。

## 版本与范围

GitHub releases/latest实际返回v1.0.0-rc.51（published_at2026-11-13T09:30:24Z），不是先前记忆里的rc50。执行git fetch upstream --tags，确认tag c9c6da2786b6a0912215127e78e28fbc6650c85f。upstream push URL仍DISABLED。

自有基线03bc32218后，先提交旧任务授权RFC fba46097b（未授予权限），再创建sync/20261114-rc51并执行 `git merge --no-commit --no-ff v1.0.0-rc.51`：**无任何冲突**。新release官方34文件1667add/238delete由merge引入，非我们扩展官方补丁；不需要把官方上游自身文件当新HOOKS名额。自有命名middleware/task_plugin.go仍14行差异。

合并未commit时门禁曾列出上游34文件“未登记”：脚本changed集合为baseline→HEAD与HEAD→workingtree并集，HEAD仍rc50所以是pending merge假阳性。直接 `git diff rc51 -- go.mod/go.sum/relaykit/web/licenses`核对仅剩原4个R3桥接文件差异；未为上游34文件加白名单或改门禁。mergecommit形成后必须重跑门禁通过才能发布。

官方更新包括moego v0.1.0-alpha.3（非AVX x86-64 SIGILL修复）、前端依赖/密码强度反馈/渠道drawer与i18n修复。没有自行修改或重写这些官方变更；实际新基线门禁在完成同步记录后重算，rc50历史保留。

## 验证分工与未完成事实

后端/真实DB子任务f698e266负责全root测试、vet/build、独立relaykit/tokenkit、admission+archive真实三库及全部引擎停止；前端e66d8c1b负责exactBun frozeninstall、fulltypecheck/test/productionbuild及CMH兼容，均只写生成物和忽略本地证据，不修改源码或production凭据。

最终结果需绑定后才能标通过。全Windows已有controller/middleware日志占用cleanup/HTTP2历史必须按实际FAIL调查，不能因上游合并无冲突抹除。若本地遗留失败，以fullUbuntuPR CI结果补充，不把无本地全绿包装成当地通过。非AVX硬件不在本机额外认证，只验证升级编译/runtime回归。

## 最终本地回归结果

- root `go vet -p2 ./...`、`go build -p2 ./...`通过；ext/pkg-jsplugin/plugins/taskadaptor/service全套通过（domestic5.009s/jsplugin18.693s/plugins14.054s/task10.927s/service30.709s）。relaykit与tokenkit分别GOWORK=off独立vet/test/build均exit0。GOAMD64=v1只是构建目标，不是物理non-AVX硬件实测。
- admission真实三库SQLite3.50.4 2.49s/MySQL8.0.44 1.29s/PG17.12 1.69s，package9.569s；archive真实三库0.09/0.33/0.46s，package6.333s，fresh/populated重复迁移/重开/tamper保留。两专属引擎已停。
- root `go test -p2 ./... -count=1`实际exit1，只有controller与middleware包失败。controller重复Windows audit.db占用导致TempDir cleanup失败，累计至10分钟包timeout（611.432s）；报警时override_unchanged仅运行0s，其单独复验 `-run '^TestManageUserQuotaRecordsTopupAndAudit/override_unchanged$' -timeout2m`通过0.768s，不能把累计timeout当该子例死锁。middleware唯一日志rotation文件占用cleanup失败10.903s。全部其它root包通过，本次relay/channel HTTP2未失败；**不宣称Windows全量通过**。
- 当前rc51源码CLI `go run -p2 . plugin lint ...` valid及 `plugin test --fixture ...`43/43通过，非旧binary。
- 官方exactBun1.4.4 frozeninstall通过，fulltypecheck通过，Vitest235文件/2887测试通过181.92s（含authredirect20）；实际production Rsbuild2.2.0构建22.7s通过。CMH20/20 tests/types/lint11文件0/0/productionbuild均通过。
- 官方optional全lint实际exit1：1432文件66warning/148errors，未修未隐藏；代表性失败文件与rc51字节一致，只有原4个R3文件有fork差异，这4文件单独真实CLI lint0/0通过。未独立重跑整个pristine tree lint，不能说全lint已认证纯上游；本次未做新Chrome表单E2E，不能沿用历史浏览器证据当本轮新验证。

后端f698e266与前端e66d8c1b均未改源码，全部自管作业已收/引擎退出，忽略results保留精确日志位置。无生产/旧系统访问或付费调用。远程完整UbuntuPR CI结果须另绑定后再合main/develop，不据本地已定位失败提前称全绿。

## 挂载点验收范围

原49文件H1目录/写前/selected/预扣/长连接/plugin/source/destination/档案/poll/content/report等由原集中host+archive三库用例回归；P0 R3官方回跳桥接既有测试须在更新后生产构建下通过。H3/H7仍未实现，不称资金守恒通过；globalGate/TaskContinuations仍未启用。

旧任务独立执行权限仍需负责人拍板explicitgrant/expiry/restartrevocation/staticfirstslice，RFC不是实际operator许可。同步rc51不等于发布生产，不访问旧系统/不迁数据/不执行付费上游。
