# RFC：README 指引与 CI 越界检查（首次触碰官方区）

- 日期：2026-11-13
- 状态：已批准（负责人口头确认"在个人下搭"，P0 第一批）
- 涉及登记：`ext/docs/HOOKS.md` R1、R2

## 要解决的问题

1. 协作者和 AI 助手打开仓库先看到的是官方 `README.md` 和官方 `AGENTS.md`，不知道我们的宪法在 `ext/AGENTS.md`，会按官方规则而不是本仓库规则工作。
2. 宪法 §2.7 要求 CI 对每个 PR 执行"官方区 diff 检查"，没有这条 CI，宪法只是纸面规则。

## 为什么非侵入方式做不到

| 方式 | 结论 |
|---|---|
| 插件 | 与问题无关 |
| `ext/` 独立实现 | README 指引必须出现在仓库首页，GitHub 只渲染根目录 README；GitHub Actions 只读取 `.github/workflows/`，放 `ops/` 下不会执行 |
| 现有挂载点 | 尚无 |

## 改动内容

| 文件 | 改动 | 行数 |
|---|---|---|
| `README.md` | 文件最顶部插入一段固定文案（中英各一行），指向 `ext/AGENTS.md`；官方原文一字不动 | +6 |
| `.github/workflows/cmh-upstream-diff.yml` | 新增一个 workflow，调用 `ops/ci/check-upstream-diff.sh` | 新文件，约 30 行 |

合计约 36 行，在宪法 150 行硬上限内。

## 对同步的影响

- `README.md`：官方几乎每个 release 都会改 README，**会产生冲突**。缓解：我们的插入段放在文件第 1 行到第一个空行之间，用 HTML 注释标记开头结尾，冲突时只需保留标记段 + 采用官方版本，三行内可解决。这是本仓库已知的常驻冲突点，在 sync-log 中按常规处理。
- `.github/workflows/cmh-upstream-diff.yml`：文件名带 `cmh-` 前缀，官方不会创建同名文件，不会冲突。

## 回退

删除 README 顶部标记段、删除 workflow 文件，恢复为纯官方状态。
