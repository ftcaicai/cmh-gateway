# H1-C 锚定Operator批准文件加载（2026-11-14）

本批新增显式LoadOperatorPolicies(environmentSnapshot)，**没有在Register/main调用，没有安装任何globalGate或Store**。官方区零修改，48/48文件、4063/6000行不变。

## 合同

- 只读调用方map的CMH_OPERATOR_MANIFEST_FILE/SHA256及可ldflags注入的BuildOperatorManifestSHA256，不自行读取进程环境。全无配置返回nil,nil；任一部分配置必须提供path与明确digest，不能空配置当允许全部。
- env/build摘要需canonical lowercase64hex；两者同时提供必须相等，env不能覆盖buildanchor。格式与冲突先验再接触文件，无defaultpath、last-good回退或隐藏union。
- 预先Stat拒常见非regular/超1MiB文件；Open后对实际handle再次Stat且LimitReader(max+1)防增长。对**原始完整文件字节**SHA匹配才进入已验strictManifestParser；返回Fingerprint仍为canonical语义指纹，二者有意分离。即使只多末尾换行，也需要新的rawfileanchor。
- 所有失败nil+固定配置错误，不回路径/content；Close失败也清空结果。symlink跟随且path属于trustedoperator配置，不声称目录授权或无TOCTOU race的文件沙箱。

## 测试与复核

既有policy_test集中临时文件测试：全空未启用、envanchor和buildanchor单独正确加载、两者冲突拒、正文多换行拒、缺path/hash/坏hash/缺文件/目录/超限拒、不泄漏path、core.Models/Destinations均不变。未使用真实批准文件或生产密钥。

独立只读复核2e1d3d6f-3f1a-45e8-a4c8-0f66ca3babaf未参与开发/未运行测试，指出Open在Stat之前可能遇UnixFIFO阻塞；主代理增加pre-open regular检查并保留handleStat，防普通误配置。Windows本轮未创建UnixFIFO，不声称对恶意替换race完全防护；其余限定loading范围无阻断报告。

`go test -p2 ./ext/... -count=1`全部通过（domestic7.633s/continuation0.163s，其余ext通过），`go vet ./ext/...`及 `go build -p2 ./...`均exit0。无DB/serializer变更，本批未启动三库；先前archive相关真实三库证据保留，不能把fileparse叫live准入。

## 未完成

sharedendpoint preselection在尚未登记middleware/task_plugin.go、48file硬限已满，不能擅改第49文件。task-scoped旧grant、credentialrefresh、affinity/type60/legacy和生产keycustody/备份/GCM预算/完整activation未完成。此loader只为完整启动前校验准备输入，不提供enable按钮；P1资金同事务RFC独立未实施。
