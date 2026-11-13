# H1-C Operator批准清单编译基础（2026-11-14）

本批只编译明确operator批准制品，**不读取环境自动启用、不写入DB、不安装global Gate/Store**。官方区0改动，累计47/48文件、3833/6000增删行保持。

## 已实现

ext/domestic/operator_manifest.go定义version1清单：PublicModels/UpstreamModels/ChannelTypes、精确Executor制品批准及Destination关系批准。只接受EncodeOperatorManifest生成的common.Marshal wireformat，Parse roundtrip严格匹配（只允许文档首尾空白），拒绝unknown/duplicate JSON字段、重复数组成员、内部格式歧义、非1version、无效UTF8和超1MiB。

模型数组每项一个精确名称，不接受CSV片段或偷偷Trim；type正数去重；artifact只接受plugin exactkey/source/config摘要，拒非零Generation（它不是批准）；目的地仍用独立DestinationPolicy，不从BaseURL/插件发现猜批准。坏清单返回nil固定错误，不交付半套策略。空清单得到全拒绝，生成指纹绑定canonical批准字节；编译后map不共享输入slice。

该格式是受控批准制品，不是通用手写JSON兼容承诺：unknown字段/空slice与nil编码差异都可能拒，需要用提供的encoder生成。里面不放地址原文/凭据/配置，只放hash与非secret身份；这不是schema能自动保证任意模型名绝不含敏感业务信息的承诺，operator仍需审核来源。

## 验证

扩展既有policy_test.go：编译纯builtin模型及plugin/source/target批准正控；currentglobalModels/Destinations不变；改输入array不改变compiledmaps；首尾空白fingerprint一致；unknown/duplicate JSON字段、内部非canonical空白、badversion/UTF8/size、重复模型/type/artifact、CSV/Trim、坏目的地/Generation全部拒，空清单不能执行。独立只读复核bf13e101-09fc-48ae-ace6-49cea17ef6cc发现Encode先Marshal会把原始无效UTF8 Go字符串替换U+FFFD，可能批准不同模型名；主代理补所有原始model/executor/destination字符串UTF8/长度及数组count/version预检，模型及hook具体回归通过，复核重读确认原缺口关闭。Parse原始wire本就拒invalidUTF8，但不能替代Encode输入检查。

扩展全套 `go test -p2 ./ext/... -count=1`通过（domestic7.626s及其余ext），`go vet ./ext/...`及 `go build -p2 ./...`均exit0；额外originaldestinationhook定向5.753s通过。

无数据库或serializer行为改动，本批未启动三库（之前archive/模型变更三库证据保留）；不把pure批准编译称为live白名单或生产启用。

## 本轮避免的错误实现

原拟将pinned RoutingGeneration直接放入候选dto，经只读子任务7d15be7e-22c8-46ca-8360-f02e333163c1核实存在 dto→pkg/jsplugin→relay/common→dto 导入环，未修改任何源码，没有用any/反射/名称或generation编号反查逃逸。该兼容缺口仍保守拒绝，未假称修好；本轮转为独立可验批准编译继续推进。未来需要typed host overload或清晰低层接口，不为保住最后slot复制execution逻辑。

## 未完成

批准artifact还没与ldflags/env/启动loader连通，更没有原子安装/全权威入口启用。task-scoped旧grant、pinned候选/affinity/type60/legacy、密钥管理及金融同事务RFC都仍未完。明确批准清单是未来完整activation的输入，不是现在的enable按钮。
