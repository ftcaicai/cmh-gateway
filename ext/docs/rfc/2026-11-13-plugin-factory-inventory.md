# RFC（回归修复提案，待批准）：自有插件的工厂目录清单

## 证据

官方 `plugins/embed.go` 使用 `//go:embed tasks`，init遍历所有子目录并RegisterFactory。因此 `plugins/tasks/cmh-param-normalize` 会被自动嵌入和注册为工厂插件，**不是之前说明中的“离线文件，不嵌入、不注册”**。请求处理实例是否生效仍由实例配置/启用控制；工厂可见不能等同已绑定付费调用。

全量回归实际失败：`plugins/builtin_plugins_test.go:168` 固定清单17项，实际18项含自有插件。这是本次引入的回归，不能归为Windows或历史基线失败。

## 最小修复候选

在既有 `requestPluginHooks` 字典明确增加 `"cmh-param-normalize": {"outbound"}` 一项；不改生产embed或registry，也不放宽“所有目录都应出现在精确清单”的断言。复用现有测试会同时验证自有工厂导出的钩子，不将全部未知cmh目录自动放行。

将既有官方测试文件 `plugins/builtin_plugins_test.go` 新增到登记清单，占第9个官方文件（当前R3后8个）；不是创建新的官方测试文件，实际约+1行。不豁免10文件/150行总预算。需要用户按官方区例外流程确认后再改；未将此文件悄悄加入白名单。

若用户要求插件绝不随制品工厂注册，则应改扩展目录规划/打包方式，把自有插件源放到官方embed范围外。那是另一项架构选择，不能靠误导文档保持现状。

## 验收

`go test -p 2 ./plugins -count=1`全通过；`go test ./ext/domestic`宿主fixture通过；不删断言、不skip、不allow-failure。修订插件说明区分“工厂随制品嵌入/可发现”与“请求实例启用/作用范围”。其他全仓Windows文件句柄/HTTP2文案失败另行复核，不与本项混淆。
