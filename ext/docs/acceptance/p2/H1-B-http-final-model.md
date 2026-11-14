# H1-B type60 OpenAI chat 最终正文身份

日期：2026-11-14。仅合成、本地 HTTP；不启用生产策略，不改变资金/H4/H6。

## 缺口与有限合同

普通未绑定 type60 的 chat converter 保留 OpenAI schema。模型映射先写入 RelayInfo，随后渠道参数 override 可将最终 JSON model 改成另一值，目的地准入仍使用 RelayInfo 中的 upstream。故审批身份与实际发送正文可能不同；这不是供应商接受行为证明。

既有 DoApiRequest 在最终 headers/signing 后、dispatch 前，仅对 active ModelGate + type60 NewAPI + OpenAI RelayFormat + chat completions 校验最终独立 replay。合法 JSON object 中解码后的规范顶层 model 必须恰好一份、非空字符串且精确等于权威 upstream；转义同名重复也拒绝。固定403/SkipRetry；noop及其他协议保持旧路径，不对 opaque provider 正文猜测字段。

要求 GetBody 可用、ContentLength 正且加一不溢出，读取独立 reader 至声明长度加一并关闭，拒绝 factory/read/close 错误、nil reader及长度不符。不读取或seek实际发送游标，不用入口正文替代最终正文。GetBody与实际发送等价是可信 Go host 的 replay 合同，不防恶意自定义 reader。检查增加 O(最终正文) 内存副本（磁盘正文同样如此），声明长度不是额外内存硬上限；没有新增128MiB等egress cap。

## 验收范围

中央 Host fixture 使用实际 NewAPI adaptor→DoApiRequest→本地 HTTP stub，验证正例原文字节不变、唯一转义model、实际参数override错配、缺失/数字/重复model、不可重放拒绝及noop旧行为。另覆盖真实 NewOutboundJSONBody replay 正例及 factory error/nil、短/长长度、read/close error 六种故障。所有拒绝要求不新增上游调用。

独立人工只读最终复核无阻断；未运行专用安全工具，不宣称完整ASVS或供应商认证。最终定向SQLite Host 7.713s（13子例）、相关vet/diff-check exit 0；父会话channel 5.624s、relay 9.898s完整测试及根build exit 0。初次fixture曾使用不存在的SkipRetry字段、遗漏HostSHA及误用empty Host hash；修正为真实req.URL.Host的精确审批后通过，未弱化生产准入。本次无数据库行为修改，不将SQLite测试称新三库认证。

父会话最终全扩展测试全部通过（domestic 8.817s），`go vet -p 2 ./relay/channel ./ext/...` 与diff-check exit 0。


## 第69轮：HTTP2完整回归失败调查（未修复）

失败测试与rc51字节一致（blob `4370e22b3429f2046c9c80920a563de1112f4ed9`）。其独立http2.Transport直接发送，不调用最终正文准入；raw服务器在收到END_STREAM后写GOAWAY立即Close，RST_STREAM与成功响应路径也立即关闭，可能未排空SETTINGS ACK等控制帧。证据支持关闭竞态，但没有失败抓包，不宣称确证TCP reset起源。

有限完整channel固定三次实验为PASS/同一:540 wsarecv FAIL/PASS，不能归因确定性顺序污染。HTTP2 debug单次通过并观察到GOAWAY接收/重试，日志会扰动调度。仅api_request.go换rc51的overlay单次及三次通过，不是纯净整树基线，也不足证明因果。诊断test-only overlay给首连接加有界半关闭/排空后三次GOAWAY通过，但同包CannotRetryWithoutGetBody仍出现同类reset；这不是合格最终修复。所有实验作业已收齐，tracked工作树未改。

后续若修测试，只能另行登记既有raw-server的协议感知有界收尾，保留次数/正文/缺失GetBody断言；当前49文件预算没有该测试文件空位。本轮不擅自修改它，不加生产POST reset重试、sleep、skip或放宽断言。资金/H4/H6实施拒绝与生产禁用保持不变。

## 第68轮：moderation显式模型边界

中央实际NewAPI outbound验证显式mapped正例、缺失model的原始字节保持、转义重复/数字/空串拒绝、实际override错配零新增发送和noop旧行为；未调用完整moderation helper或资金结算。完整Host SQLite13.313s、定向vet/diff-check通过；独立只读复核无阻断。

父会话两次完整集成中relay与全部ext通过（domestic8.470s/8.049s），但channel均在既有TestUpstreamGetBody_HTTP2RetryAfterGracefulGoAway_PassThrough:540失败：`wsarecv: An existing connection was forcibly closed by the remote host.` 同一用例单独运行0.146s通过。该测试直接构建HTTP2 client/send，并未调用本轮DoApiRequest守卫；仍保留完整包exit1，不称全channel绿色，不改或弱化该测试。明确排除此单例的其余channel0.166s、相关vet/rootbuild/diff-check exit0；门禁49/49文件4639/6000行，余1361，named14/40。无数据库行为修改，不称新三库认证。

仅active type60＋Moderations模式＋最终OpenAI格式：最终canonical model缺失保持旧行为，存在则必须唯一、非空字符串并精确等于权威upstream。其他既有必填模型格式仍要求恰好一份。tools正文model并非路由名，已有官方兼容测试允许不同值，本轮不纳入；moderation三处默认值stable/latest/omni不一致仍未解决，缺失model不是供应商默认目标获批的证明。不会注入model、改默认值或泛化multipart。

## 第67轮：Alpha Search具名最终格式

Alpha Search入口要求非空model，实际helper先映射、后参数override；因此同样可能出现审批upstream与最终正文不一致。仅追加AlphaSearch模式＋OpenAIAlphaSearch最终格式，沿用既有type60/active gate、独立replay及唯一canonical model合同；不套到正文无需model的tools、moderation缺省模型或图片multipart。

中央fixture调用真实AlphaSearchHelper及本地HTTP stub：映射后的模型和未知嵌套字段到达stub；实际override错配403/SkipRetry且无新增发送；noop保留原override发送。stub固定返回合成400，使helper在usage/settlement之前结束，probe reserve/settle/refund全零；这是发送身份验收，不是完整成功响应或资金守恒证明。最终完整Host SQLite14.150s、相关vet通过；父会话channel0.147s/relay0.399s/ext全tests（domestic9.005s）、相关vet/rootbuild/diff-check exit0。独立只读复核无阻断，未执行专用安全工具。首次fixture误用BillingSession字段编译失败，修正为真实Billing后通过，未改生产合同。官方门禁49/49文件、4638/6000行，余1362，named14/40；无数据库行为修改，不宣称新增三库认证。

## 第66轮：有限已知JSON端点收口


原chat-only判定还漏掉completions、embedding、Responses HTTP，以及临时改mode的chat→Responses转换。本轮仍在同一DoApiRequest挂载点，使用GetFinalRequestRelayFormat（显式final、转换链末项、入口格式的既有优先级），仅纳入：

- OpenAI最终格式 + chat/completions模式；
- Embedding最终格式 + embeddings模式；
- Responses最终格式 + Responses或ResponsesCompact模式；
- Compaction最终格式 + ResponsesCompact模式。

compact准备链会将DTO投影为Responses并追加转换链，所以不能只接受入口Compaction；真实PrepareResponsesRequest正反例验证这一点。converted chat使用真实service.ConvertRequest→adaptor.Convert→Append链，入口OpenAI保持而最终为Responses。六组实际NewAPI HTTP正例字节保持、实际override错配无新增dispatch；Claude最终格式对照仍保留旧行为。普通表格正文仅为身份envelope，不代表每端点完整语义请求认证。图片JSON/multipart、moderation默认模型、其他provider/额外格式不混入本轮。

原replay故障与duplicate检查全部保留。独立只读源码及中央测试复核无阻断。最终定向SQLite完整Host 13.250s、相关vet/diff-check exit0；父会话channel0.160s、relay5.806s、全部扩展tests（domestic8.842s）、相关vet及rootbuild通过。无DB行为修改，无新增三库认证或生产启用。最终门禁49/49文件、4636/6000增删行，余1364；named例外14/40。首次父会话diff-check发现文档EOF额外空行，已修正后重跑，不将后续shell成功覆盖该失败。
