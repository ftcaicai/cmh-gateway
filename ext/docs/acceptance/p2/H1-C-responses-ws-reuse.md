# Responses WS 冻结 provider 上下文与发送前复核

2026-11-14；有限H1修复，无生产/付费调用，无资金协议改变。

## 修复

- session保存首次实际选中的raw channel.Other，后续create恢复冻结值；不以fresh DB值冒充旧连接配置。既有fresh-row精确校验仍拒绝配置漂移。
- 复用连接payload准备后、create写入前调用既有CheckRequestAdmission；新连接成功握手后、setTarget前同样复核，拒绝则关闭局部target，直接返回、不进入dial故障重试。
- 沿用原defer失败路径；不增加退款/结算实现。检查不覆盖cancel/control帧。

## 实际中央socket回归

真实type61 native Responses provider、精确executor审批：同一连接两次create成功，1 handshake / 2 upstream creates；原始Other或plugin config漂移后error，计数不增加。另开连接在实际握手返回101前使用channel屏障禁用DB渠道：仅增加1 handshake，0 create，客户端error，upstream观察target关闭。无sleep/NthAttempt伪屏障。

初始fixture错误使用根路径、随后缺provider identity witness，得到get_channel_failed；修正实际/v1/responses路径和真实witness后重跑，未削弱SUT。

## 验证

- `go test -p 2 ./ext/domestic -run '^TestHostAdmission' -count=1 -v`：真实SQLite3.50.4 / MySQL8.0.44 / PostgreSQL17.12全部PASS，分别8.03/7.28/6.71s，package22.331s。独立引擎正常停止，启动jobs均收集exit0。
- 实现代理定向SQLite13.724s、Responses WS定向relay5.777s、vet exit0。
- 集成相关全测relay0.447s、domestic8.943s及其余ext通过；集成vet及完整`go build -p 2 ./...`最终exit0，job已收集。

## 范围限制

这是当前渠道/策略与既有精确executor再次核验，不是任意最终JSON字节的完整授权，也不承诺原子消除所有TOCTOU。复用连接payload准备期间缺少现成真实屏障，未宣称其动态撤销测试已覆盖。每create重新批准已连接目的地需要保存实际签名握手目标，尚未由本补丁实现。无金融守恒/race/最低DB版本/完整ASVS认证。独立最终只读diff复核无阻断；复核未运行测试，不宣称完整ASVS认证。生产源码仅既有登记文件增加10行；实测累计49文件4485行，剩1515，named shared endpoint例外14/40不变。
