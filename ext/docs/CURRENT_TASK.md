# 连续推进检查点

更新时间：2026-11-13；范围 P0→P1→P2；当前状态：P0 HTTP/Chrome会话契约通过，官方UI自动返回仍待验；P1资金设计复核未通过。

已推源码：`ab4c69bee`，远程 main/develop 完全一致；GitHub cmh-upstream-diff 两次 success（43367336940 / 43367330565）。不将此门禁成功解释为分支保护已启用。

## 当前目标

完成独立控制台与官方会话复用的真实验证；不再逐小节等待用户。以阶段验收推进，但不把未验证的架构当成事实。

## 最后完成证据

- `go test -p 2 ./ext/... -count=1`、`go vet ./ext/...` 通过。
- 新增 console 静态契约 12 子场景：GET/HEAD、SPA、真实脚本、缺失资源 404、目录与隐藏文件拒绝、未构建 503。
- brand Host 隔离 11 子场景通过，修复 `/pricing` 泄露主站兜底；测试按官方 API 先注册、H5 后注册的真实 Gin 顺序。
- `ops/ci/p0-http-acceptance.mjs` 在全新临时 SQLite、loopback 运行完整二进制，13 项通过：初始化、登录、HttpOnly+SameSite Cookie、Cookie-only self 401、刷新后 Bearer self、控制台资产、Host、注销与旧凭证失效。无密码/Token/Cookie 被输出。
- `ext/docs/acceptance/p1/funding-lifecycle-audit.md` 保存独立资金复核。原方案“四入口唯一出口、80行足够”的结论不成立，资金钩子未实施。

## 下一动作

1. 前端代理已完成仅 web-cmh 的会话客户端/i18n修复；父代理复跑20项测试/typecheck/lint/format与默认输出生产构建全部通过。真实Chrome验证7项通过，记录 `acceptance/p0/T0.2-browser-session.md`。
2. 实测官方登录界面跳回独立 `/cmh` 应用；HTTP API 成功和显式整页导航成功不代表官方SPA自动回跳成功。使用真实官方前端构建；不得用占位 index 当登录UI验收。
3. P1 先以 FundingSource 工厂/事务接口为候选重新写 RFC；保持同事务决策，不自动改成双账本或“替换钱包”。150 行预算未证实可行，不擅自改规则。
4. P2 可进行纯白名单实现与旁路审查；付费渠道矩阵需独立凭据/费用授权，不能搬旧系统 Key 或配置。

## 授权与禁止重复

- 用户授权持续实现、测试、提交推送新仓库；不访问旧生产，不迁客户或资金，不做付费调用。
- 不重复推测 Cookie 可直接鉴权：官方为 refresh cookie→短时内存 Bearer；官方登录路由 `/sign-in`，不是 `/login`。
- 不用假的 Redis 余额/unlimited Key 让品牌调用“通过”。
- 不把 race 未运行记为通过：当前 `go test -race` 因 CGO 未启用且 C 编译器未发现而失败。
- 不宣称 GitHub CI 成功等于已阻断合并：保护分支未核验。

## 本轮资源与作业

- 本地验收二进制 `%TEMP%/cmh-gateway-proto.exe`，官方 `web/dist/index.html` 仅本轮新增的本地 embed 占位，非生产构建；需保留直到本轮验收结束，之后按精确路径清理。
- 所有 loopback 验收服务与独立Chrome均已关闭且输出已收；HTTP三个实例和浏览器实例由 job_kill/Browser.close 退出，不是运行失败。构建任务 pwsh-9a9565ed24074527、pwsh-1423a3758b4a4d03 和前端 pwsh-2130ccbf4ce047a4 全部 exit0 已收；race pwsh-5d04b3b69e8c44bf 因CGO未执行已收。
- 临时 SQLite `cmh-p0-fixture-*.db` 是合成验收库，可能含仅测试凭据，禁止提交；不能按广泛匹配清理，先列精确绝对路径归属。
- 当前未提交内容含前一轮尚未完成的 console/web-cmh，加上本轮隔离修复与审查记录；提交前逐项检查，不 `git add -A` 扫进临时数据库。
