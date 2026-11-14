# Responses WS 原成功连接目的地重新审批

2026-11-14；H1已批准目的地边界内有限修复。无生产/收费调用，无资金实现。

## 实现与可信边界

既有DoWssRequest保留wrapper签名，新增成功握手返回ConnectedTargetAdmission。所有字段私有：仅存目的地hash/模型渠道值/实际conn/有效标志，不保存原始签名URL或headers。最终signer和Host override后、阻塞dial前冻结tuple，仅成功dial后mint。没有公开DTO构造器；零值、nil、别的conn、模型渠道不一致在enabled policy下拒绝。

Responses每create在既有模型复核后，对实际原conn的冻结tuple查询当前destination policy；新连接还在握手成功后/setTarget前复核，拒绝关闭局部target且不进入dial故障重试。noop仍保持旧dial行为，capture无效不会额外拒绝noop；不因此声称支持热激活。cancel/control帧不改，原失败defer保持。

## 实际回归

中央host fixture实际signer添加固定synthetic query，配置Host override。精确签名URL与Host批准后2create/1握手；wrongHost-only和unsignedURL-only审批均拒绝且不增加create/dial，恢复原审批后同连接可继续。实际公开握手API产生有效proof可用于原conn，不能借给另一conn；nil/zero拒绝。保留前批DB渠道禁用屏障，另测目的地撤销屏障：实际握手101前撤销、释放后0create且target关闭，session有deadline/join/屏障释放；proofConn立即defer清理。未用sleep/NthAttempt。

初期测试typed-nil error使用require.NoError误判，改为正确require.Nil后完整重跑；不改生产错误语义。独立只读最终diff复核无阻断，提出保留旧屏障/地址负控制/跨conn/失败清理建议均已落实；复核未运行测试，不宣称ASVS完整认证。

## 验证结果

- 实现代理最终focused Host SQLite13.393s、vet/diffcheck exit0，所有作业已收集。
- 集成真实三库：SQLite3.50.4 7.86s / MySQL8.0.44 6.81s / PostgreSQL17.12 7.68s；package22.655s PASS。命令：`powershell.exe -NoProfile -ExecutionPolicy Bypass -File ops/.cache/db-test/run-admission-matrix.ps1`（内部`go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`）。专属合成DSN不入库；引擎已正常停止，启动jobs exit0已收。
- 集成`go test -p 2 ./relay ./relay/channel ./ext/... -count=1`：relay5.888/channel4.787/domestic8.417s等全部PASS；对应vet及完整`go build -p 2 ./...` exit0。
- 官方累计49文件4551增删行，剩1449；无新增官方文件，命名shared endpoint14/40不变。

## 不证明的内容

这是原逻辑签名URL/Host的授权，不是DNS/IP pinning、TLS peer或proxy hop授权，不是任意最终payload身份证明。不原子消除policy check之后的所有TOCTOU；复用payload准备期间动态撤销屏障仍未实测。无财务守恒、race、最低DB版本或供应商live认证。全部缺省noop及archive/grants生产禁用边界保持，资金/H4/H6实施禁令不变。
