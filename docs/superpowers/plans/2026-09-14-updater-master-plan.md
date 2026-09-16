# 应用内更新(App Auto-Update)· Master Plan(治理与索引)

> **For agentic workers:** 推荐 Quality-LTDD 执行(superpowers:ltdd)。Master Plan 只做治理:最终意图、子计划索引、进度台账、AC 覆盖矩阵。**具体任务步骤一律在各 Phase Plan 中**,本文件不包含实现细节。

**Final Intent:** 用户可在应用内完成版本更新:检查 GitHub Releases 最新版本 → 下载并 SHA-256 校验 → 一键替换并自动重启;失败自动回滚并拉起旧版本、启动时告知;启动自动检查默认开、可跳过版本;全链路零新依赖。

**设计文档:** `docs/superpowers/specs/2026-09-14-updater-design.md`(技术契约 §5,实现不得偏离;决策记录 §7 已确认)

**决策 ADR:** `docs/adr/0001-updater-architecture.md`、`docs/adr/0002-windows-per-user-install.md`

**术语表:** `CONTEXT.md`(自动检查 / 跳过版本 / 更新产物 / 安装并重启 / helper 模式)

## 拆分决策(依据 acceptance-driven-plan §0)

触发条件:预估任务数 18~22(>10);计划总量 >700 行(用户预设规则:>700 行必须拆 Master+Phase);4 个可独立验收阶段且风险模型显著分化(只读网络 / 缓存写入 / 自身替换高危 / 发布收尾);4 层协同(Go 包 / bridge / 前端 / CI 发布链)。**已拆分**(2026-09-14 用户批准)。

Phase Plan 生成策略:各 Phase 启动时即时生成(基于上一 Phase 落地后的真实接口,避免空转计划)。

## 子计划索引与进度台账(Progress Ledger)

| Phase | Plan | Status | Current Gate | Depends On | Final Intent Coverage |
|---|---|---|---|---|---|
| P1 版本与检查 | `plans/2026-09-14-updater-phase-1-check-plan.md` | **done**(2026-09-15;Quality-LTDD:6 任务+评审×6+oracle 终审;实机验收全链路闭合[toast/跳转/跳过,skippedVersion 机器验证];commits `b3a8c41..16915bd`) | 已合并并推送双远端(完成) | 无 | AC-1/2/3 |
| P2 下载与校验 | `plans/2026-09-15-updater-phase-2-download-plan.md` | **done**(2026-09-16;Quality-LTDD:6 任务+评审×6+oracle 终审 Ready=Yes[Important 已修补 6d7dbf2];机器侧真实链验收:拒签+happy path 双 PASS[v0.1.0 已补传校验资产];UI 人工走查以等价证据替代记录于 rehearsal-checklist;commits `1082e78..6d7dbf2`) | 已合并 main(fast-forward,推送待指示) | P1 ✅ | AC-4/5;AC-12(SHA256SUMS+签名部分);AC-11(白名单修正);AC-9(update:progress);AC-15(清理) |
| P3 替换与重启 | (待生成) | not-started | — | P2 | AC-6/7/8/13/14;AC-12(installscope 部分) |
| P4 发布链收尾与终验 | (待生成) | not-started | — | P3 | AC-12(wails.json)/15;全局终验;文档随动(frontend-ui-spec "9 分类"/README/backlog) |

状态取值:not-started / in-progress / done / blocked。每完成一个 Phase 更新本表,并记录 commit 区间与验收证据位置。

## Global AC 覆盖矩阵

| Global AC | 覆盖来源 |
|---|---|
| AC-1(版本显示;系统层与应用内一致) | P1(ldflags 接线 + 关于面板)、P4(wails.json 系统层注入) |
| AC-2(手动检查与错误分类) | P1 |
| AC-3(自动检查/跳过版本/跳转/静默失败) | P1 |
| AC-4(下载进度/取消/120s 停滞) | P2 |
| AC-5(SHA256SUMS fail-closed) | P2 |
| AC-6(macOS 整体替换 + 失败回滚 + 拉起旧版) | P3 |
| AC-7(Windows per-user + rename-aside + 重试) | P3 |
| AC-8(Linux rename + 权限保留) | P3 |
| AC-9(架构/绑定面/事件契约) | P1(绑定面)、P2(update:progress)、P3(apply 扩展) |
| AC-10(既有功能逐字节不变) | 各 Phase 回归门(P1~P4) |
| AC-11(零依赖/域名白名单/SHA-256) | P1(白名单初版)、P2(校验)、各 Phase 回归 |
| AC-12(release.yml 增强) | P2(SHA256SUMS)、P3(installscope)、P4(wails.json) |
| AC-13(安装确认框中断提醒) | P3 |
| AC-14(失败闭环:启动告知) | P3 |
| AC-15(缓存启动清理/数据边界) | P2(清理)、P3(替换边界) |

规则:每个 Global AC 必须映射到至少一个非 L1 任务或 Phase AC;跨模块/端到端 AC 由 L3 任务或 Phase AC 覆盖。

## 全局约束(与设计文档 §5.7 一致,所有 Phase 隐含继承)

- `go.mod` 零变化;不新增第三方依赖(HTTP/JSON/哈希/zip 全部标准库;macOS 解压用系统 `ditto`)
- 不改变既有服务与页面行为;后端 `go vet ./...` + `go test ./... -count=1` + `go test -race ./...`;前端 `npm run lint` + `npm run typecheck` + `npm test -- --run` + `npm run build` 全绿
- 仅 HTTPS 白名单域名(`api.github.com` / `github.com` / `objects.githubusercontent.com`);SHA-256 强制校验(下载阶段起)
- Git:所有操作前缀 `GIT_MASTER=1`;Conventional Commits(中文摘要 + 要点列表);逐任务提交
- 注释/提交信息/文档全中文;每 Phase 结束时工作树干净

## 最终验收(全部 Phase 完成后执行)

按设计文档 §3 的 AC-1~AC-15 逐条执行;核心手工验收:真机升级演练(v0.1.0→v0.1.1,含回滚与跳过版本场景)、系统层版本一致性(文件属性/Finder)、失败闭环跨会话告知。证据(命令、输出、截图/日志路径)留存于执行台账,未跑过验收不得宣称完成。
