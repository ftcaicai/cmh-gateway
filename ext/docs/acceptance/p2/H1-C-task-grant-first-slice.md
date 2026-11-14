# H1-C 有限逐任务轮询授权首批（2026-11-14）

状态：负责人明确批准的有限首批已实现并完成真实三库验证；不安装全局Gate/Store/authorizer，不包含真实task许可数据或生产授权。合同见rfc/2026-11-14-task-continuation-authority.md。

## 权限而非档案存在

仅可信已安装Store.Load认证的artifact和精确operator许可同时匹配时mint私有scope；task/user/channel/ref/purposepoll/plugin/sourceSHA/runtimeSHA全部精确，有限buildQueryRequest/parseTaskResult必需，可选signRequest/extractUsageOnComplete。Ref/RestoreBindalone、nil/zeroScope、错引用或其他用途不授予权限；未安装authorizer或无匹配时仍当前sourcepolicy。已匹配但过期/畸形拒，不能回退global绕失效。

每次真正JS调用前（包括排队取得runtime与参数转换后）复核source/export/Members空/到期/目的地policyenabled；附scope即使global也批准仍强制no utils.fetch。scope不提供凭据刷新/账号交换，auth仅空/none/api_key，任何Key行含pluginCredential或oauth2_jwt等auth在queryContext/token网络前拒；scope只可绑定per_task/poll，batch/content/report不授予。实际签名descriptor与每redirect仍globalexacttarget批准，并在派发前检查scope到期。

operator政策为启动不可变副本，显式到期，撤销需替换清单并重启所有承载worker/节点，已启动hook/network可按原timeout完成，不承诺强杀/回滚或可信时钟回拨防护。Store/authorizer/Go宿主为TCB，私有字段不抗进程攻陷。原task timeout/CAS/refund不延长；无新submit许可。

OperatorManifest增可选omitempty Continuations字段以保留旧v1无grant制品canonical字节，返回编译后的独立policy不自动安装。unknown/duplicate/malformed/oversize/invalidUTF8仍拒，许可hooks与request被复制避免调用方slice改变已批准身份。

## 最终验证

实际archived任务经core.Load→真实resolver→service DispatchPlatformUpdate/query/parse，在global拒A新调用时仅精确grant使旧taskA继续1query、PollFailures2→0/InProgress且Quota不变；actual新TaskAdaptor.ValidateRequestAndSetAction仍拒无submit。错task/user/channel/refID/refDigest/plugin/source/runtime无权限，zeroScope/RestoreBindalone/未列hook/globalallow均不能绕，pollgrant不授权batch/content。无enabled目的地policy resolver拒；denyURL直接fetch0query；单及多KeypluginCredential拒0query，OAuth合法source经Capture/Save+matchinggrant仍resolver拒，避免隐式token网络。

Engine inheritedWithFetcher被scope硬隔离，即使global重批也0fetch；late AdjustBillingOnComplete在scope下0额外JS（parse已捕获usagefacts，int-only晚回调无法安全传播到期）；显式已有adaptor到期拒。另用Concurrency1实际JS hook持slot+可观测Done admission barrier，第二hook先真实排队、等待显式ExpiresAt计时器后释放slot，验证它不能在队列跨到期后开始JS；无生产clockoverride或固定sleep。每redirect expiry-after-SSRF由代码复核，本批没有真实redirect耗时跨到期测试，不作该项动态验证声明。

`run-admission-matrix.ps1` / `go test -p2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`最终exit0：SQLite3.50.4 4.44s/MySQL8.0.44 3.74s/PostgreSQL17.12 3.72s，package12.162s，显式grant子场景含queue与adaptor到期分别2.14/2.69/2.14s。专属引擎优雅退出。最终pkg/jsplugin7.638s/relay9.831s/task8.491s/service20.694s/ext全套domestic5.406s等通过，相关vet与完整build exit0；旧manifestliteral canonicalbytes不变、新grant拷贝/重复/坏hook/expiry/SHA/purpose等policy测试通过。

独立只读e570e94c未参与实现/未执行测试，发现多行凭据可先refresh、scoped目的地拒绝误计普通失败、late usage额外unscopedJS、排队失败后过期分类等问题；主代理/owner补每line检查、scope-only目标拒Unavailable、晚回调不再JS、slot/runtime失败时复核scope，重读确认所报缺口闭环，无本有限范围新阻断。初版API错名ScopeFromContext导致半态编译失败，改为既定ContinuationScopeFromContext后重跑通过，未加兼容别名或放宽测试。非完整ASVS/抗恶意TCB/即时多节点撤销声明。

当前基线rc51，累计49/4365行，命名sharedendpoint仍14/40，余1635行/0新官方文件。

## 不作的完成声明

不是batch/content/report/refresh/dynamic签名queryURL task目标授权完成，不是affinity/type60/legacy/fullfailclosedactivation；没有50文件授权、没有生产付费调用/密钥安装，资金同事务RFC仍独立未实施。rc51新基线与完整Linux同步CI已单独验收，不借上游CI绿认证本批尚未执行测试。
