# RFC（已批准）：官方后台侧边栏增加 OEM 控制台入口

状态：**负责人于 2026-11-27 批准**：2 个具名官方文件例外、行数上限 20；菜单文案选 A（只用英文源文案）；可见范围为管理员及以上。例外登记于 `ext/AGENTS.md` §3.2/§6.3 与 `ext/docs/HOOKS.md`（R5）。规则 PR 合入前不动官方文件。

## 问题

OEM 控制台在 `/cmh/`（`web-cmh/`，自带导航，"品牌档案"在 `/cmh/brands`）。官方后台（`web/`）里没有任何指向它的链接：运营和品牌主登录官方后台后，只能手动输入 `/cmh/` 才能进入。`ext/AGENTS.md` §6.3 已预见此情况，规定要在官方界面放链接须走 §3.4 RFC。

## 已排除的非侵入方式

- **`web-cmh/` 或 `ext/` 内实现**：入口必须出现在官方 SPA 渲染的侧边栏里，扩展区代码不在该页面运行。
- **官方侧边栏模块配置**（`SidebarModulesAdmin` / 用户 `sidebar_modules`）：只能开关 `use-sidebar-config.ts` 中 `URL_TO_CONFIG_MAP` 列出的固定模块，不能新增条目。
- **自定义页脚 HTML**（`footerHtml`）：可放 `<a href="/cmh/">`，但页脚属于公开页面布局，且对所有访客可见，不能按角色显示。
- **仪表盘"API 信息"条目**：可配置带链接的条目，但对所有登录用户可见，且不在导航里。
- **现有挂载点**：H2/H5/H8 均为服务端挂载点，不控制官方前端导航；R3 只处理登录后的回跳。

后两项配置方式可作为不批准本 RFC 时的替代，但都做不到"只对有权限的人显示的菜单项"。两项仅经阅读前端代码确认存在，未在运行实例中验证显示效果。

## 候选官方改动

共 **2 个既有官方文件**，预计合计不超过 15 增删行，不新增官方文件、路由、依赖。

1. `web/src/hooks/use-sidebar-data.ts`：在 `admin` 分组追加一个条目，`url: '/cmh/'`，`requiredRole: ROLE.ADMIN`，另加一个图标 import。约 7 行。
2. `web/src/components/layout/components/nav-group.tsx`：`SidebarMenuLink` 用 TanStack `<Link to>` 做 SPA 跳转，而官方路由没有 `/cmh`，直接点击会落到官方 404（与 R3 复现的问题同源）。需对 `/cmh` 子树使用 `reloadDocument`。复用 R3 已登记并导出的 `getAuthRedirectNavigation`（`web/src/features/auth/lib/auth-redirect.ts:82`，返回 `reloadDocument` 标志），不另写一份正则，也不改该文件。约 3–5 行。

未在 `URL_TO_CONFIG_MAP` 中登记的 URL 默认可见（`isModuleEnabled` 对无映射项返回 `true`），因此不需要改 `use-sidebar-config.ts`。

**可见性**：`requiredRole` 只控制菜单是否显示。`/api/ext/oem` 的访问仍由服务端 `cmh_oem:read` / `cmh_oem:write`（默认授予内置管理员角色）强制，本改动不改变任何鉴权。

## 未确认事项

- `web/src/hooks/__tests__/sidebar-config.test.tsx` 等既有测试是否断言菜单清单，尚未核对；若需同步修改，测试文件同样计入文件与行数。
- 菜单文案的翻译，见下方第 2 问。

## 预算与规则影响

当前门禁实测（基线 `v1.0.0-rc.51`）：官方文件 **58 / 58，已用满**；H1 行数 4641 / 6000；H8 行数 353 / 1800。

因此本改动无法使用现有额度，需要一个新的具名文件例外，流程与此前的第 49 文件例外相同：

1. 独立 `rules:` PR：`ext/AGENTS.md` §3.2 增加本例外（具名 2 文件、行数上限）、§6.1/§6.3 补充说明、修订记录加一行；`ops/ci/check-upstream-diff.sh` 的文件上限同步调整。
2. rules PR 合入后才改官方文件，并在 `ext/docs/HOOKS.md` 登记两个文件（建议编号 R5，形态"前端桥接"）及白名单。

**对同步的影响**：`use-sidebar-data.ts` 是官方菜单清单，上游新增菜单项时会改同一数组，冲突概率中等但冲突内容简单（保留双方条目）。`nav-group.tsx` 改动点在单个组件的一个属性上，冲突概率低。每次同步须验证：菜单项仍显示、点击后整页加载到 `/cmh/`、其他菜单项仍为 SPA 跳转。

## 必需验收

- 管理员登录官方后台，侧边栏"管理"分组出现入口，点击后加载 OEM 控制台并显示当前账号。
- 普通用户不显示该入口；直接访问 `/api/ext/oem` 仍被服务端拒绝。
- 其他侧边栏条目行为不变（仍为 SPA 跳转，不整页刷新）。
- 侧边栏折叠态与移动端抽屉下入口可用。
- 官方前端类型检查、相关单测、生产构建通过；`ops/ci/check-upstream-diff.sh` 通过；真实浏览器复验。

## 负责人决定（2026-11-27）

1. 批准这 2 个具名官方文件的例外，行数上限 20。
2. 菜单文案选 A：只用英文源文案（如 `OEM workspace`），不改官方翻译文件；中文等界面下该项显示英文。未选的 B 方案是给官方 7 个语言文件各加一条翻译。
3. 可见范围为管理员及以上（`ROLE.ADMIN`）。

## 追加：第三个具名文件（2026-11-27 同日批准）

实现时发现本 RFC 漏了一处：官方命令面板 `web/src/components/command-menu.tsx` 通过 `useSidebarView()` 与侧边栏共用同一份菜单数据，新条目会同时出现在命令面板里；该处用 `navigate({ to })` 做 SPA 跳转，选中后落到官方 404。

负责人在"追加第三个具名文件"与"接受为已知限制"之间选择了前者。例外范围改为 3 个具名文件，合计行数上限仍为 20：在 `command-menu.tsx` 的两处 `navigate` 调用上，对 `/cmh` 子树使用 document navigation，同样复用 `getAuthRedirectNavigation`，预计 3–4 行。

追加的验收项：在命令面板中选中该条目后加载 OEM 控制台；选中其他条目仍为 SPA 跳转。

## 实施与验收记录（2026-11-27）

实际改动：三个具名文件相对基线 `v1.0.0-rc.51` 合计 19 增删行（上限 20）。`command-menu.tsx` 只改了顶层条目的一处 `navigate`，子菜单条目的那处未动（没有指向 `/cmh` 的子菜单）。路由把链接地址规范为 `/cmh`（无末尾斜杠），服务端照常提供控制台。

本地检查（Bun 1.4.4，`web/` 目录）：

- `bun run typecheck`：1,494 文件无类型错误。
- `oxlint` 与 `oxfmt --check` 针对三个文件：均无问题。
- `vitest run` 既有的 `sidebar-config.test.tsx` 与 `command-menu.test.tsx`：21 个用例通过。未新增官方测试，因为测试文件不在具名范围内。
- `ops/ci/check-upstream-diff.sh`：导航 `3 / 3` 文件、`19 / 20` 行，通过。
- `NODE_ENV=production bun run build` 成功。

真实浏览器（Chrome 158 headless，本地临时 SQLite 实例，经官方登录表单登录）7 项通过：

- 管理员登录后侧边栏出现 `OEM workspace`。
- 点击其他侧边栏条目仍为 SPA 跳转（页面内存标记保留）。
- 点击入口后整页加载，`/cmh/static/` 脚本加载，控制台渲染。
- 命令面板选中其他条目仍为 SPA 跳转；选中 `OEM workspace` 后整页加载控制台。
- 390 像素宽的移动端抽屉中入口可用。
- 普通用户登录后侧边栏没有该入口。

未验证：桌面端侧边栏折叠态未单独操作；普通用户访问 `/api/ext/oem` 被服务端拒绝这一项，页面内不带令牌的 `fetch` 只得到 401，不能作为权限拒绝的证据，本次未改动鉴权代码；官方前端全量测试与全量 lint 未跑。
