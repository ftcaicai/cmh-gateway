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

## 不能推出的结论

仅成功合成启动与认证后deny-all HTTP证明；不是全协议正向可用、生产启用、三数据库启动/迁移认证、档案密钥合同或资金守恒。请求在上游前拒绝，不宣称实际预扣/结算/退款守恒。未改数据库/迁移代码，无本轮三库兼容结论。预main可信factory编译/top-level例外仍保留。
