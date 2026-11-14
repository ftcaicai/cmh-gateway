# P0→P1→P2 整体验收状态（2026-11-14）

**结论：目标未完成，不具备宣称P1/P2完成或启用全局策略的证据。** 本页为可接续工程状态，不替代各项原始验收记录。

## 已有可复核交付

- P0：独立同栈CMH控制台、官方refresh Cookie→memoryBearer会话与登录回跳桥接；原真实Chrome表单5项及独立控制台回归通过。rc51后官方235文件/2887测试、CMH20测试/type/lint/build通过，但本轮未重做Chrome E2E。
- 官方同步：已真实merge rc51 c9c6da278，无冲突，PR3完整UbuntuCI43545005397后端/前端通过后正常合入0ebd2e0e2并同步main/develop；不是同tag检查。rc51合入后完整CI43545550059也success。
- H1：实际渠道变更、候选/目录/映射、selected/预扣/重试/WS、JS源/配置、HTTP/WS与fetch最终目标、任务内容、加密档案、poll/content/report冻结接线已分批实现并有集中host真实三库及独立复核。
- 有限旧task授权：负责人批准的static per_task精确task/ref/purpose/source/runtime+finitehooks+显式expiry+全承载节点重启撤销首批已实接，不提供utils.fetch/refresh/OAuth；真实旧task在新调用被拒时可按精确许可继续query/parse，新submit仍拒；queue跨到期实测通过。
- Policy veto：源和真实目标政策拒绝对archived任务仅跳本次，不误计普通PollFailures；真实JS同文案异常仍普通错误，初始/redirect目标网络计数与privatefunding保持用例三库通过。原超时/CAS/refund未改，不是永久挂起保证。
- T1.6c：rc51后原始Go符号重检仍24匹配/9文件（15生产/5测试/3定义/1注释），相关9文件rc50→rc51差异空；不是唯一资金出口证明。
- cmh-param-normalize：当前rc51源码CLI valid、43/43fixtures及实际hostpatch回归通过，工厂已embed注册，request实例仍未启用。

## 仍未满足的退出条件

1. **P1资金**：H3/H7完整资金生命周期同事务接入、token/任务归属/幂等恢复与BATCH开关/Redis守恒及故障重启三库测试未实现。旧四方法handled提前返回已证伪，不能复活。资金RFC2026-11-13仍未获实施批准，不能借H1预算覆盖财务重构。H4定价/H6正式启动迁移hook仍为保留范围，不把独立archive migration称整期完成。
2. **P2权威兼容**：affinity有效参数envelope、type60/legacy执行者及启用requestplan等仍保守未知/拒绝，尚无全协议安全且可用的验收；有限grant不覆盖batch/content/report/refresh或动态签名query目标task权限。
3. **启动与运维**：Models/PluginHooks/Executors/Destinations仍noop；TaskContinuations/ContinuationGrants仍nil。批准manifest+锚定loader虽能编译返回policy，但尚无完整启动原子安装、密钥custody/备份恢复/GCM寿命预算、全量failclosed行为与同制品验收。禁止提前安装半套策略。
4. **回归缺口**：Windows全rootcontroller/middleware日志/DB占用cleanup和累计package timeout失败保留；rc51 optional全web lint148errors/66warnings未修。Linux完整CI通过不抹掉这些平台事实；race因C编译器缺少未认证。数据库仅SQLite3.50.4/MySQL8.0.44/PG17.12，不声称最低版本全面认证。
5. **真实调用/上线**：无生产凭据、收费模型调用、真实operator taskgrant、28个供应商生产渠道配置或生产域；不访问旧系统/不迁移数据/不切流。离线本地stub通过不等于供应商live或生产可用。

## 已批准不可重复询问

R3最小官方回跳、R4精确工厂清单、H1累计48/6000与唯一命名middleware/task_plugin.go第49例外≤40增删行（累计6000不变）、有限taskgrant首批合同均已明确批准。源码当前相对rc51官方49文件/4365增删行，命名文件14/40，余1635行且0任意新文件名额；不删测试凑预算。

## 接续顺序

先前未覆盖的redirect共享验证等待跨expiry已补真实三库（initial1/后继0/Unavailable），见p2/H1-C-grant-redirect-expiry.md；接续在已有登记范围收口affinity/type60/legacy/requestplan可用性并开展完整startup设计/验收，不能继续以小parser代替主线。金融同事务保持用户原决定，需独立实施设计批准后再动账务。遇第50文件/命名例外超40行、费用、生产凭据或架构权限改变才提新决策。

自动连续轮数达到40不是完成标准，也不是技术阻塞。目标保持未完成；没有为了到达计数而暂停/完成/标blocked。后续接续以CURRENT_TASK与本页及具体acceptance为准，不依赖聊天中“待提交”历史文字。
