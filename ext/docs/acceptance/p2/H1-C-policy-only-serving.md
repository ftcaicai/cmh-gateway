# H1 policy-only 实际合成 Serving 拒绝验收

## 制品与隔离

源码 dcba0c5f760328ddc9d677701a667ba45a649d7d；实际根二进制 SHA256 `EB78F5DD5845468565C97571AC35ACEFFF18B58B3A4C317CF879E4ACDD8A467B`，Go1.26.10 windows/amd64。全新临时 SQLite，网关实际引擎3.50.4（通过Go model DB查询）；Python辅助库3.53.1不冒充网关版本。

真实根程序 HOST=127.0.0.1，合法canonical deny-all manifest+精确rawSHA。首次启动完成迁移后停止；忽略区helper通过官方导出 model.InitDB/User.Insert/Token.Insert/Channel.Insert 准备Role1普通用户、随机短期API token与指向loopback stub的type60合成渠道；不改鉴权、不管理员绕过、不导入生产资料。再用同一二进制与manifest重启。

## HTTP可观察证据

- GET /api/status → 200：真实成功监听本机。
- 已认证 GET /v1/models → 200，data=[] / object=list / success=true。
- 已认证 POST /v1/chat/completions，model=synthetic-denied → 403，code=access_denied，message=model admission denied。
- 未认证 GET /v1/models → 401 Invalid token，作为鉴权对照；不把鉴权失败计为policy生效。
- 实际loopback upstream POST计数0。
- gateway已停止、stub shutdown/close；父代理Get-CimInstance复核无policy-startup-proof.exe进程，无后台作业。

## 验收来源与失败调查

忽略区 `ops/.cache/serving-proof.py`、`seed-serving.go`、`serve-auth-proof.py` 和安全摘要 `policy-serving-results.md` 可复现。父代理实读脚本、数据初始化器和摘要核对真实Bearer请求/断言/计数/清理；不复制包含合成token的原始环境或数据库入库。

首次helper未显式禁Redis，User.Insert后的缓存调用触发nil Redis；修复fixture为RedisEnabled=false，复用已插入合成用户后完整通过，未改生产源码。首次go version缺GOSUMDB，补显式工具链校验镜像后确认Go版本。

## 第46轮：允许模型正向发布与同偏好对照

同一上述二进制、复用既有隔离SQLite合成fixture（不是新库），使用真实 `domestic.EncodeOperatorManifest` 生成canonical制品：

```json
{"Version":1,"PublicModels":["synthetic-denied"],"UpstreamModels":["synthetic-denied"],"ChannelTypes":[60],"Executors":null,"Destinations":null}
```

rawSHA `8fa3e6a40548575cf88c5465151eef2580b1c3005cfce1246fe5210133109047`。builtin不要求插件制品审批；Destinations保持deny-all，未含Continuation。

- 精确允许制品实际重启：status200；普通Role1 Bearer models200，恰好发布synthetic-denied；未认证401；upstream POST0。
- 同用户显示偏好的deny-all制品再次重启：status200；Bearer models200且data=[]；未认证401；upstream POST0。
- 两次均未发送chat、未调用上游或资金请求；finally terminate/wait、stub shutdown/close/join，子代理CIM确认无残留。

首次允许模型仍空目录，调查发现官方controller/model.go的未计价模型显示过滤；通过导出User.GetSetting/UpdateUserSetting仅为合成用户设置AcceptUnsetRatioModel=true，不设置价格、不改资金。成功正向与deny-all对照均使用相同偏好，避免将显示偏好差异误判为策略效果。默认python过旧导致f-string SyntaxError，改用bundled Python后运行。

实际命令：bundled Python运行忽略区 `ops/.cache/positive-catalog-proof.py`，随后CATALOG_DENY_CONTROL=1运行同脚本，均exit0；内部go run -p2执行manifest编码、合成数据与显示偏好helper。父代理读取安全摘要和39行脚本，核对精确id断言、401、零调用和清理。安全摘要 `ops/.cache/policy-positive-catalog-results.md`；临时env/DB含合成凭据，不入库。

此项仅证明真实目录可按精确审批正向发布，不证明dispatch成功、全协议可用或资金守恒。

## 不能推出的结论

仅成功合成启动与认证后deny-all HTTP证明；不是全协议正向可用、生产启用、三数据库启动/迁移认证、档案密钥合同或资金守恒。请求在上游前拒绝，不宣称实际预扣/结算/退款守恒。未改数据库/迁移代码，无本轮三库兼容结论。预main可信factory编译/top-level例外仍保留。
