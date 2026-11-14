# 首批资金实现前工程审计（2026-11-14）

状态：只读设计审计，不是实施、硬预算、资金事务或三库验收。负责人已批准有限同步非流式纯文本chat、专属新token、客户/OEM独立账户，以及经济/恢复/正文最小保存设计依据；不重复申请这些方向。

## 已收敛的协议

- Open只读；首次绑定由已验证身份经User-first runner锁内复核后创建，不要求预先存在authority。known commit后才能封装不透明authority；DTO不是权限。
- 全部User升序→全部Token升序→ownership→request/attempt→accounts升序→reservation/event/outbox。普通及品牌Token.Insert均须与HardDelete通过User锁串行化。
- model拥有财务阶段唯一提交权；单次、禁止重入的participant仅在同步窗口使用同一事务。HardDelete收养其原事务，不另开资金事务。仍须实际故障/竞争测试，不把私有类型当作证明。
- 最终URL、Host、method、body及header override/签名完成之后，在relay/channel/api_request.go网络前强制claim。只有已知首次commit成功可创建发送reader；unknown、失败、重放不发送。正文丢失不重建。

## 源码调查的决定性发现

1. **transport**：当前共享client禁止redirect但保留共享连接池/H2；ApplyUpstreamBodyMetadata恢复GetBody。Go1.26.10 H2在某些路径即使GetBody=nil也可能重试。首批需独立全新HTTP1直连transport，无keepalive/H2/proxy继承/replay wrapper，保留TLS、目的地、context与超时。仅追求网关至多一次应用层HTTP attempt，不能保证供应商exactly-once或无TCP重传。尚未实现/测试。
2. **模型资格为空**：仓库本地tokenizer及普通预扣估算不是供应商计费用量上界；尚无具体生产model/version/endpoint得到证明。缺闭合schema、所有计费类别/最高费率/输入输出上界证据即预留前拒绝；不编造模型能力或价格。
3. **实际请求晚于初始预估变化**：转换、system prompt、override与egress可能改变正文。原始入口及最终JSON都须独立闭合校验；不能按旧body预留再静默改变经济意义。
4. **正文可在controller之前落盘**：TokenAuth先于Distribute，后者首次读取BodyStorage。必须鉴权后、c.Next前新鲜验证品牌归属并安装请求级bounded nonspill策略；common/gin.go初始/legacy/cached分支及common/body_storage.go已知/未知长度、超大拒绝前均须覆盖。事后删临时文件不是未持久化。
5. **日志与回显**：compatible_handler完整请求debug、openai响应debug、非200 error preview及report快照可能保留prompt。必须提前阻断/固定安全错误，不能先记录再清洗。应用不主动保存正文不等于保证swap、crash dump、反向代理或供应商无保留。
6. **原始usage来源**：OpenAI处理会用本地估算补齐缺失usage，并在资金结算前写响应。需在fallback前保留原始完整性/来源；缺失、超界或未报价类别进入uncertain，不以合成usage结算，不承诺撤回已发响应。
7. **caller/defer**：controller在nil-success、violation及retry前检查opaque outcome。任何Written响应不再追加第二份JSON但保留pending审计；refund defer不能依赖error非nil推断是否派发。usage intent与共同结算成功前不得产生财务成功统计/通知。可保留主要接口，仅候选PostTextConsumeQuota返回error；编译与全部caller禁入仍待验证。
8. **舍入**：官方Decimal helper使用Round(0)。候选在明确目标最小单位的非负精确金额上先Ceil，再QuotaFromDecimalStrict；币种/额度scale、换算顺序和边界尚须精确合同与测试。不改官方全局舍入。

## 候选与剩余证明

目前至少18个必需官方生产候选：model/token.go、model/quota_reserve.go、model/token_cache.go、model/utils.go、model/user.go、middleware/auth.go、service/billing_session.go、service/billing.go、service/text_quota.go、relay/request_billing.go、relay/compatible_handler.go、controller/relay.go、relay/common/relay_info.go、relay/channel/api_request.go、service/http_client.go、relay/channel/openai/relay-openai.go、common/gin.go、common/body_storage.go。

原条件候选controller/token.go、service/quota.go、service/funding_source.go保留；正文审计另标relay/common/outbound_body.go、service/error.go、relay/requestplugin/report.go为条件项：只有现有必需调用点可证明提前阻断时才可不改。测试、可能启动接线另计。清单不是实测diff或预算；H1现有额度不授权财务用途。

剩余：具体DTO与schema/模型上界证据、全部raw/bulk/Redis/batch mutation覆盖、participant生命周期与多用户竞争、实际typed caller兼容、一次派发本地故障测试、三库升级/幂等/未知commit/Redis×BATCH守恒、元数据/usage/tombstone保留期限及历史删除。以上未验证，不能批准无条件端到端或生产启用；但可以先形成有退出门槛的synthetic-only实施申请，以实现获取这些工程证据所需的代码。合成fixture不能授予真实模型资格，实施仍须独立范围/预算授权及rules PR。

## 阶段与故障验收规格（设计，未执行）

负责人已明确选择继续设计、不批准实施。本节只固定未来可审查断言，不创建仓储、事务桥、测试代码或rules PR。

- 分离持久事实与调用者观察：已提交的Reserved、DispatchClaimed、usage intent、Settled/Cancelled由数据库事实证明；CancelPending、SettlementPending、Uncertain、UsageIntentPending可能仅是内存观察。写入失败不能声称已持久保存pending或usage。
- 合成oracle仅用于规格：客户初始1000、OEM初始2000，各自独立资产；预留售价100/成本60后分别为available/reserved＝900/100及1940/60，token remain900。实际售价70/成本40结算后分别为930/0/consumed70及1960/0/consumed40，token remain930/used70；30/20释放与终态、分录、token同事务。这里1:1额度scale不是产品配置，consumed桶也不是对手科目或真实双重分录证明。
- 预留任一写入后确定回滚：全部财务事实保持初始值，可保留先前独立提交的bootstrap绑定，网络0。commit回执丢失则数据库只能是完整前态或完整后态，调用者CommitUnknown；必须从独立连接按精确phase key核验，不能猜测回滚或重复释放。
- claim确定回滚、unknown、重放均发送0次。即使unknown后来查到claim已提交，也不补发。只有活跃首次已知commit成功者可发送一次；claim/cancel竞争只有一个赢家，不得出现已释放预留与有效发送权并存。
- claim后EOF、超时、取消、3xx、异常SSE、缺失或不可信usage：保持预留与claim，不自动refund/redrive/追加收费；实际网关应用层attempt最多1、redirect下一跳0。已写HTTP响应不追加第二份正文，pending审计不能冒充可撤回响应。
- usage intent确定回滚：数据库仍只有claim，重启不能从丢失的内存usage结算。已有intent但结算回滚保持预留及原intent；结算unknown先查精确phase结果。相同终态重放零额外token/资金/通知；实际等于预留的零delta结算仍须终态事件。outbox投递失败或回执丢失不逆转已提交资金。
- 最终正文不匹配若在预留前发现，资金不变；若在已知预留后发现，应为取消成功或保持Reserved并观察CancelPending，不能笼统声称零资金触碰。缺失usage不等于合法零usage；首批可靠全上界合同没有已定义Append触发点，不自行添加claim后Append。

仍未闭合的有限设计项：预留阶段token.used是冻结计入还是仅结算计入；各资产精确对手科目及方向；phase-key/payload与各阶段outbox含义；完整零usage的合法性。不得用上述示例替代这些定义。未来验证须独立SQL连接及重启读取，覆盖每写入点回滚、真正lost-ack、三库fresh/upgrade/重复迁移与Redis×BATCH四组合；当前没有运行这些测试，纯值原型不能替代事务证据。

只读调查原稿位于本地ignored缓存，未提交敏感正文或凭据；本页保存可接续结论，不将源码阅读包装成测试通过。
