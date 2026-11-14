# H1-C：当前 legacy owner 不授予 builtin 身份

## 有限结论

目录/保存/候选投影不再以 legacy owner 的 models 声明为排除条件。当前 registry 对该 channel type 有 owner，即返回未知执行器，由已启用的权威 ModelGate 拒绝。实际 legacy task 解析并不依赖声明模型，因此旧条件可能把未声明模型误标为 builtin。

这不是 legacy 支持或上线批准。默认 sora/type1 等双用途普通 chat 在真实 gate 启用后也会被保守拒绝；不能靠扩大 builtin 白名单恢复。静态 platform 映射的current owner现已与实际resolver共用；真正无owner和旧 pinned generation 的完整身份仍未解决。普通未绑定 type60 无 owner 时保持 builtin；绑定路径及既有 pinned 执行守卫不变。所有全局 gate 仍 noop，Store/authorizer 仍 nil。

## 集中行为验收

现有 host 测试中新增 legacy_owner_projection：no-owner type1/type60 实际 Insert 和目录成功；注册只声明其他模型的 type1 owner 后，未声明模型从目录消失，DB/memory 候选均拒绝，实际 Insert 拒绝且无新行；type60 仍可保存/发布。

父 ordinary fixture 原先隐式依赖默认工厂。首次 focused 失败揭示默认 sora 拥有 type1，正是该保守策略的兼容损失；改为显式空 registry 并恢复，owner 场景独立验证，未弱化生产判断。没有 JS metadata 或计费字段修改。

## 验证

- `powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1`：PASS，package 21.539s；SQLite 3.50.4 7.90s / MySQL 8.0.44 6.58s / PostgreSQL 17.12 6.79s；新 case 0.05/0.04/0.01s。
- 测试引擎已正常停止；仅合成 loopback 数据，无生产或收费调用。
- `go test -p 2 ./model ./relay ./ext/... -count=1` PASS（model11.449s/relay5.587s/domestic7.976s）；`go vet -p 2 ./model ./relay ./ext/...` 与 `go build -p 2 ./...` exit0。
- 独立只读 agent 完整规则/API静态复核未发现有限补丁阻断项；明确确认默认 sora/type1 的普通 chat 兼容损失与 owner 缺失/旧 pinned 未覆盖；未重复执行长测试，不宣称专用安全工具或完整ASVS合规。

## 静态 resolver current owner 补充修复

共享原有静态platform映射到registry，relay公开包装委托，模型在同一generation核查by-type及真实resolver。保持静态key优先、数字fallback、nil结果与factory错误分类。sora未声明channelTypes及numeric 1不再误标builtin；空registry恢复普通渠道，type60控制保留。未放宽type61执行guard，active policy下双用途普通渠道仍可能保守拒绝；不是旧pinned或legacy正向执行认证。

真实三库中央矩阵PASS，package22.005s：SQLite3.50.4 8.41s/MySQL8.0.44 6.55s/PostgreSQL17.12 6.74s，legacy case0.06/0.04/0.01s。全部独立数据库正常停止，jobs已收。实现代理相关jsplugin/relay/model tests、vet、full build通过；首次缺strconv导入导致编译失败，补齐后完整重跑。独立只读复核无阻断，未运行测试、不宣称完整安全认证。官方累计49文件4475增删行，剩1525，named exception14/40不变。
