# H1收口预算确认与规则门禁验证（2026-11-13）

负责人在当前会话明确选择：**批准累计48文件/6000行收口预算（推荐）**。

范围仅H1剩余执行链、最终目的地、后台延续及已发现兼容缺口；逐文件登记、80%复核、真实本地/三库行为验证和独立审查不变。累计而非追加份额，不是费用、生产或资金H3/H7重构授权，也不是完成保证。旧32/2500额度到达后没有擅自新增官方文件。

## 规则落地

- 独立 `rules:` PR [#1](https://github.com/ftcaicai/cmh-gateway/pull/1)，提交 `f6789bd81`，合并 `eac5ebaa0`，已同步main/develop。
- ext/AGENTS v0.9、既有H1 RFC与实际CI脚本一致改为48/6000，没有目录通配豁免、依赖反转放行或关闭越界检测。
- 新增ops/ci/test-upstream-diff-gate.sh，在独立临时Git仓库调用真正门禁：0文件、48文件、6000行允许；49文件、6001行和未登记文件拒绝。临时目录先验证固定绝对路径/前缀再精确清理，不触工作仓库文件。没有只靠搜索新数字假装门禁仍硬。
- 实际命令 `bash -n ops/ci/check-upstream-diff.sh ops/ci/test-upstream-diff-gate.sh`，再运行新fixture与真实工作区门禁，均exit0；当前累计32/48文件、2463/6000增删行。

## 远程CI事实

PR创建时触发了官方完整CI（此前普通main direct push仅差异workflow）。[run43439381396](https://github.com/ftcaicai/cmh-gateway/actions/runs/43439381396) 最终全部success：Ubuntu root/relaykit/tokenkit vet/build/make test通过，官方frontend typecheck/test通过；[差异门禁43439381256](https://github.com/ftcaicai/cmh-gateway/actions/runs/43439381256) success。

没有把此前Windows全仓cleanup等历史失败删除或改断言。远程Ubuntu成功证明该环境的官方完整流程通过，不把CI embed placeholder视为正式Web发布制品，不等于资金/三库/生产TLS/ASVS全通过。

注意：执行 `gh pr merge --auto --merge` 未使用admin，仓库却在frontend test仍pending时立即完成merge，说明未证明仓库强制等待全部checks；不能声称分支保护已生效。该frontend检查之后真实成功，但不倒写为合并前已全部绿。合并事件重新触发[run43440243739](https://github.com/ftcaicai/cmh-gateway/actions/runs/43440243739)，最终也全部success；main/develop对应差异门禁43440241158、43440411482均success。后台watch一次GitHub GraphQL TLS handshake timeout退出1，随后直接run view确认success，不能把watch网络失败误报为CI失败或忽略。

业务门禁仍未安装；完整H1执行链与后台延续收口继续，不因扩大额度提前激活。
