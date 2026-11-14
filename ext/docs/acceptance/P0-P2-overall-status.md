# P0→P1→P2 整体验收状态（2026-11-14）

**结论：目标未完成，不具备宣称P1/P2完成或启用全局策略的证据。** 本页为可接续工程状态，不替代各项原始验收记录。

## 已有可复核交付

- P0：独立同栈CMH控制台、官方refresh Cookie→memoryBearer会话与登录回跳桥接；原真实Chrome表单5项及独立控制台回归通过。rc51后官方235文件/2887测试、CMH20测试/type/lint/build通过，但本轮未重做Chrome E2E。
- 官方同步：已真实merge rc51 c9c6da278，无冲突，PR3完整UbuntuCI43545005397后端/前端通过后正常合入0ebd2e0e2并同步main/develop；不是同tag检查。rc51合入后完整CI43545550059也success。
- H1：实际渠道变更、候选/目录/映射、selected/预扣/重试/WS、JS源/配置、HTTP/WS与fetch最终目标、任务内容、加密档案、poll/content/report冻结接线已分批实现并有集中host真实三库及独立复核。
- 有限旧task授权：负责人批准的static per_task精确task/ref/purpose/source/runtime+finitehooks+显式expiry+全承载节点重启撤销首批已实接，不提供utils.fetch/refresh/OAuth；真实旧task在新调用被拒时可按精确许可继续query/parse，新submit仍拒；queue跨到期实测通过。
- Policy veto：源和真实目标政策拒绝对archived任务仅跳本次，不误计普通PollFailures；真实JS同文案异常仍普通错误，初始/redirect目标网络计数与privatefunding保持用例三库通过。原超时/CAS/refund未改，不是永久挂起保证。
- affinity有限兼容：当前base与最终effective双精确审批已接线，真实规则cache miss→官方merge→provider wire通过；任一撤销/篡改/未批fresh base拒绝，三库通过。不是affinity-rule来源证明；第41轮补充task有效envelope选中/捕获/真实descriptor提交/双撤销，task body未应用provider operations，不改官方语义或宣称全协议认证，见p2/H1-C-affinity-effective.md。
- P1隔离纯值原型：tenant/request/source精确绑定、预留/追加/部分消费与释放/冲突重放/边界12例通过；只是合成单资产状态转移，不是真实双式账本、同事务或Redis×BATCH证明。见p1/funding-value-contract.md；未连接任何生产资金路径。
- 启动policy-only：负责人明确批准，真实main早期DB/worker前安装四项一致策略；非法/partial/grant配置早停，valid deny-all与缺省进入故意DB sentinel，六类实际进程无SQLite文件；archive/grants禁用、无生产激活，预main factory编译例外保留。见p2/H1-C-policy-only-startup.md。第45轮同制品合成成功监听loopback、普通Bearer models200空/chat403策略拒绝/未认证401对照/upstream0，已停止；第46轮同制品精确允许模型真实目录发布1项，同显示偏好deny-all仍0项，两次401对照/上游0；不证明dispatch。见p2/H1-C-policy-only-serving.md。
- T1.6c：rc51后原始Go符号重检仍24匹配/9文件（15生产/5测试/3定义/1注释），相关9文件rc50→rc51差异空；不是唯一资金出口证明。
- cmh-param-normalize：当前rc51源码CLI valid、43/43fixtures及实际hostpatch回归通过，工厂已embed注册，request实例仍未启用。

## 仍未满足的退出条件

1. **P1资金**：H3/H7完整资金生命周期同事务接入、token/任务归属/幂等恢复与BATCH开关/Redis守恒及故障重启三库测试未实现。旧四方法handled提前返回已证伪，不能复活。资金RFC2026-11-13仍未获实施批准，不能借H1预算覆盖财务重构。H4定价/H6正式启动迁移hook仍为保留范围，不把独立archive migration称整期完成。
2. **P2权威兼容**：affinity已覆盖有限provider wire及task有效envelope，重试全协议覆盖仍未认证；普通未绑定type60为builtin，插件绑定type60仍拒绝；legacy当前by-type owner不再按模型声明回退builtin，保存/目录/DB及memory候选三库拒绝；代价是默认sora/type1双用途普通chat在active gate下也拒绝，不是完整兼容；owner缺失/旧pinned/静态platform整体身份仍待解决。启用requestplan等仍拒绝，尚无全协议安全且可用的验收；有限grant不覆盖batch/content/report/refresh或动态签名query目标task权限。
3. **启动与运维**：policy-only早期一致安装已接线、全部缺省仍noop，TaskContinuations/ContinuationGrants禁用；尚无全协议正向成功Serving/生产同制品验收、档案密钥custody/备份恢复/GCM寿命预算及全协议可用性。仅合成合同获批，不授权生产manifest启用。
4. **回归缺口**：Windows全rootcontroller/middleware日志/DB占用cleanup和累计package timeout失败保留；rc51 optional全web lint148errors/66warnings未修。Linux完整CI通过不抹掉这些平台事实；race因C编译器缺少未认证。数据库仅SQLite3.50.4/MySQL8.0.44/PG17.12，不声称最低版本全面认证。
5. **真实调用/上线**：无生产凭据、收费模型调用、真实operator taskgrant、28个供应商生产渠道配置或生产域；不访问旧系统/不迁移数据/不切流。离线本地stub通过不等于供应商live或生产可用。

## 已批准不可重复询问

R3最小官方回跳、R4精确工厂清单、H1累计48/6000与唯一命名middleware/task_plugin.go第49例外≤40增删行（累计6000不变）、有限taskgrant首批合同和policy-only启动合同均已明确批准。源码当前相对rc51官方49文件/4404增删行，命名文件14/40，余1596行且0任意新文件名额；不删测试凑预算。

## 接续顺序

先前未覆盖的redirect共享验证等待跨expiry已补真实三库（initial1/后继0/Unavailable），见p2/H1-C-grant-redirect-expiry.md；接续在已有登记范围收口affinity/type60/legacy/requestplan可用性并开展完整startup设计/验收，不能继续以小parser代替主线。金融同事务保持用户原决定，需独立实施设计批准后再动账务。遇第50文件/命名例外超40行、费用、生产凭据或架构权限改变才提新决策。

用户于2026-11-14明确将连续轮数上限由40改为80，目标已恢复active/armed；轮数不是完成标准，也不是技术阻塞。目标保持未完成；没有为了到达计数而暂停/完成/标blocked。后续接续以CURRENT_TASK与本页及具体acceptance为准，不依赖聊天中“待提交”历史文字。
