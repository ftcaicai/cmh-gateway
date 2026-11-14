# H1-C：当前 legacy owner 不授予 builtin 身份

## 有限结论

目录/保存/候选投影不再以 legacy owner 的 models 声明为排除条件。当前 registry 对该 channel type 有 owner，即返回未知执行器，由已启用的权威 ModelGate 拒绝。实际 legacy task 解析并不依赖声明模型，因此旧条件可能把未声明模型误标为 builtin。

这不是 legacy 支持或上线批准。默认 sora/type1 等双用途普通 chat 在真实 gate 启用后也会被保守拒绝；不能靠扩大 builtin 白名单恢复。owner 缺失、静态 platform 映射和旧 pinned generation 的完整身份仍未解决。普通未绑定 type60 无 owner 时保持 builtin；绑定路径及既有 pinned 执行守卫不变。所有全局 gate 仍 noop，Store/authorizer 仍 nil。

## 集中行为验收

现有 host 测试中新增 legacy_owner_projection：no-owner type1/type60 实际 Insert 和目录成功；注册只声明其他模型的 type1 owner 后，未声明模型从目录消失，DB/memory 候选均拒绝，实际 Insert 拒绝且无新行；type60 仍可保存/发布。

父 ordinary fixture 原先隐式依赖默认工厂。首次 focused 失败揭示默认 sora 拥有 type1，正是该保守策略的兼容损失；改为显式空 registry 并恢复，owner 场景独立验证，未弱化生产判断。没有 JS metadata 或计费字段修改。

## 验证

- `powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1`：PASS，package 21.539s；SQLite 3.50.4 7.90s / MySQL 8.0.44 6.58s / PostgreSQL 17.12 6.79s；新 case 0.05/0.04/0.01s。
- 测试引擎已正常停止；仅合成 loopback 数据，无生产或收费调用。
- `go test -p 2 ./model ./relay ./ext/... -count=1` PASS（model11.449s/relay5.587s/domestic7.976s）；`go vet -p 2 ./model ./relay ./ext/...` 与 `go build -p 2 ./...` exit0。
- 独立只读 agent 完整规则/API静态复核未发现有限补丁阻断项；明确确认默认 sora/type1 的普通 chat 兼容损失与 owner 缺失/旧 pinned 未覆盖；未重复执行长测试，不宣称专用安全工具或完整ASVS合规。
- 不借本用例宣称金融、最低数据库版本、race 或完整 failclosed 启动验收。
