# 同步官方记录

每次同步官方 release 在此追加一行。规则见 `ext/AGENTS.md` §7。

| 日期 | 官方版本 | 操作 | 冲突文件 | 处理方式 | 验证结果 | 执行人 |
|---|---|---|---|---|---|---|
| 2026-11-13 | v1.0.0-rc.50（`f2aa91232`） | **建立基线**：fork 自 `QuantumNous/new-api`，`main`/`develop` 均指向该 tag | 无 | — | 官方源码未改动 | ftcaicai |
| 2026-11-14 | v1.0.0-rc.51（`c9c6da278`） | sync/20261114-rc51真实merge release | 无 | 官方34文件原样合并，自有HOOKS保留 | 本地vet/build/相关tests+独立modules+admission/archive三库+官方235/2887前端测试/productionbuild通过；Windows全量controller/middleware cleanup及全web lint失败保留，远程完整CI待验，见acceptance/p1/upstream-rc51-sync.md | ftcaicai |
