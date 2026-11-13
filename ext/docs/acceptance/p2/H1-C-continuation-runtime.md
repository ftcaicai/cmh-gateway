# H1-C 显式冻结runtime与未发布旧源恢复（2026-11-14）

本批提供显式capture/restore方法与真实query验证；**尚未接提交前archiveSave、TaskPrivate引用或生产poll resolver，不是跨进程完整任务恢复完成**。全局source/model/target门禁仍默认noop，恢复方法也不豁免已安装检查。

## 变更

- pkg/jsplugin/registry.go LoadedPlugin留存编译时精确private source string，SourceBytes返回独立copy；实际Engine/sourceSHA一致仍单独验证。不是从当前Registry或version标签重建源码。
- TaskAdaptor.CaptureContinuationArtifact(info)只接受自身Init的actual info、正channelID/type61、绑定key匹配、actual Engine/Loaded/sourcebytes hash一致、taskpart。host v1运行控制包含BaseURL/settings/OtherSettings/effectiveHeaders/ParamOverride/effectivePluginConfig及finalpublic/upstream；不读freshDB。
- Artifact不复制ApiKey/token/credentialblob/requestpayload字段；必要header/config本身仍可能敏感，必须进入前批加密archive，不应公开或日志输出。ChannelMeta没有全mapping表，本方法只存最终public/upstream，不虚构mappingfreeze。
- RestoreContinuationArtifact验证sourcehash/boundsUTF8、exact hostv1struct编码/类型/binding/transport/base/configobject，在哈希验证之后使用CompilePlugin**未发布**编译，不能Register或改变全局routes。runtime严格byte重编码，未知/缺失/非原host编码拒绝，不是通用JSON兼容承诺。
- restored adaptor有private frozen状态，Init不能覆盖，shared submitguard在ModelAdmission noop之前拒绝新提交，cached body路径也不能绕过。FetchTask/Batch base/proxy不符在credential/builder前拒；queryContext与signRequest用同一frozenConfig不回当前CacheGetChannel。

## 行为证据

扩展既有ext/domestic/plugin_test.go TestFrozenTaskRuntimeRestoration：sourceA/configA/effective headerA捕获后，原info改configB/baseB/headerB，当前Registry替sourceB（同key/version）；显式restore后实际loopback FetchTask发送A revision、A signer config、A override，Registry仍指B。SourceBytes返回copy篡改不改变留存source。

Init(infoB)不能重置冻结；base/proxy替换拒绝不新增请求；Validate/BuildRequestBody/DoRequest直接入口均拒newsubmit（包括默认noop）；开启空targetpolicy后同restored查询也被拒0网络增量，证明没有以“已存档”为由绕过activegate。损坏源hash、wrongPluginKey、unknownruntimeversion拒绝。

只读独立复核804189ad-48cf-4066-b144-90493092f343（未开发/未测试/无生产收费调用）核对实际capture/restore/sourcecopy、hash前置、Init与cachedsubmit拒绝时机及frozen query/signer，未发现本未接线基础范围阻断问题。未认证当前source撤销后旧任务仍获批准：activegate仍会拒。

## 验证

定向actual restored query+sourceidentity测试5.736s通过。`go test -p 2 ./pkg/jsplugin ./plugins ./relay/channel/task/jsplugin ./relay/channel ./relay ./service ./ext/... -count=1` exit0：jsplugin7.594s/plugins8.659s/task3.628s/channel5.604s/relay9.820s/service21.215s/domestic1.938s，其余ext通过；同范围vet和 `go build -p 2 ./...`均exit0。无DB行为/serializer/migration修改；archive SQLite用原fixture普通回归，未启动MySQL/PG，不伪称又完成真三库持久task恢复。没有生产key/DSN或付费模型；statickey仅fixture调用参数，未复制进RuntimeJSON。

累计41/48官方文件、3158/6000增删行，全部复用已登记文件；source/task函数登记同步。

## 未完成的重要接口

1. Capture方法当前由测试显式调用，不是已在成功submit上的冻结点；默认noop普通submit仍可能从当前cache读取config，未来必须在真正已准入actualruntime路径、联网前耐久保存，而不是把info投影当历史执行证明。
2. archiveBinding/taskRef、model identities、用途/目标grant与restoredtask自身逐项核对尚未接；不能把恢复函数作为任意task执行能力入口。
3. Restore.info无ApiKey且Init不能附新key，content恢复需明确account-bound credential注入合同；本批仅statickey Query使用参数key，不声称contentcredential路径已通。
4. poll分批/缺档hold分类及timeout/财务恢复未接。旧源unpublished恢复不替代operator source/config/model/target授权，不会自动通过现有全局gate。H3/H7同事务选择仍独立，不能为收口放宽新提交或把缺档转成退款。

鉴权相关语义参考 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 与 [Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)，本批不改会话协议，不宣称ASVS整体认证。
