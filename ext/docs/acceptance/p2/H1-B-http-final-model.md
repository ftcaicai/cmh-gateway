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

官方门禁相对rc51：49/49文件、4625/6000增删行，余1375行；named middleware/task_plugin.go例外仍14/40。80%范围复核保留，不增加官方文件。
