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

剩余：具体DTO与schema/模型上界证据、全部raw/bulk/Redis/batch mutation覆盖、participant生命周期与多用户竞争、实际typed caller兼容、一次派发本地故障测试、三库升级/幂等/未知commit/Redis×BATCH守恒、元数据/usage/tombstone保留期限及历史删除。未运行这些验证，不具备实施申请条件。

只读调查原稿位于本地ignored缓存，未提交敏感正文或凭据；本页保存可接续结论，不将源码阅读包装成测试通过。
