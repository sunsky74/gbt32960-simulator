# 插件包扩展机制 · Master Plan(治理与索引)

> **For agentic workers:** 推荐 Quality-LTDD 执行(superpowers:ltdd)。Master Plan 只做治理:最终意图、子计划索引、进度台账、AC 覆盖矩阵。**具体任务步骤一律在各 Phase Plan 中**,本文件不包含实现细节。

**Final Intent:** 实施人员在 APP 中导入 JSON 扩展包(零代码)即可让模拟器在 0x02 实时报文尾部追加私有数据单元、发送私有命令帧(0x09~0x7F);未绑定包时现有行为逐字节不变。

**设计文档:** `docs/superpowers/specs/2026-08-31-extpack-design.md`(技术契约 §4,实现不得偏离)

## 拆分决策(依据 acceptance-driven-plan §0)

触发条件:预估任务数 > 10、计划总量远超 700 行(用户预设规则:>700 行必须拆 Master+Phase)、4 个可独立验收的业务阶段。**已拆分。**

Phase Plan 生成策略:各 Phase 启动时即时生成(基于上一 Phase 落地后的真实接口,避免空转计划);当前仅 Phase 1 Plan 已生成。

## 子计划索引与进度台账(Progress Ledger)

| Phase | Plan | Status | Current Gate | Depends On | Final Intent Coverage |
|---|---|---|---|---|---|
| P1 扩展包核心引擎 | `plans/2026-08-31-extpack-phase-1-core-plan.md` | **done**(2026-09-01,7 commits 575dff7..8c281e5,SDD 执行 + 任务评审×6 + oracle 终审 + 终审修复) | — | 无 | AC-4;AC-1/2 字节级地基 |
| P2 实时数据扩展端到端 | `plans/2026-09-01-extpack-phase-2-realtime-plan.md` | **done**(2026-09-01,6 commits 6fa0420..6f46f4a,SDD 执行 + 任务评审×6 + oracle 终审 Ready to merge;PAC-1~4 全达成) | — | P1 | AC-1, AC-3, AC-5 |
| P3 扩展命令字 | `plans/2026-09-01-extpack-phase-3-commands-plan.md` | not-started(计划已生成,含里程碑/关联性/重要性;待 oracle 执行前评审) | — | P2 | AC-2, AC-3 |
| P4 管理与交付面 | (启动时生成) | not-started | — | P2 | AC-6;AC-4 UI 面 |

状态取值:not-started / in-progress / done / blocked。每完成一个 Phase 更新本表。

## Global AC 覆盖矩阵

| Global AC | 覆盖来源 |
|---|---|
| AC-1(0x02 追加 TLV + 表单 + golden) | P2(端到端)、P1(编码字节级) |
| AC-2(私有命令帧 + 周期) | P3 |
| AC-3(未绑包行为逐字节不变) | P2(回归门)、P3(回归门) |
| AC-4(非法包路径化报错、不落盘) | P1(校验器)、P4(导入 UI 面) |
| AC-5(换绑/解绑、配置不丢) | P2 |
| AC-6(示例包 + 指南) | P4 |

规则:每个 Global AC 必须映射到至少一个非 L1 任务或 Phase AC;跨模块/端到端 AC 由 L3 任务或 Phase AC 覆盖。

## 全局约束(与设计文档 §4.6 一致,所有 Phase 隐含继承)

- 协议库 gb32960-go 零改动
- 标准路径现有 golden/单测保持全绿(逐字节)
- 技术栈不变(Wails v2 / Go / Vue 3 / AntD);表单继续由 GroupSchema 驱动
- 字段 DSL 最小集与数值变换契约按设计文档 §4.2 冻结
- 错误信息带 JSON 路径;一次仅激活一个包

## 最终验收(全部 Phase 完成后执行)

按设计文档 §2 的 AC-1~AC-6 逐条执行:AC-1/2/5 走 `wails dev` 手动流程 + golden hex 对比;AC-3 跑 `go test ./...` 全绿 + 未绑包 UI 截图对比;AC-4 构造 4 类非法包逐一导入验证报错与不落盘;AC-6 按指南实操计时。
