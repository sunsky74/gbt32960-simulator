// 「规划中」占位行共享定义:徽标文案、控件宽度与占位行数据结构。
// 各分类面板以 PlannedRow[] 描述占位行,由 PlannedSettingRows 统一渲染,
// 保证 8 个分类的占位行外观与禁用状态完全一致。

// 下拉/输入类控件统一宽度,禁止被 flex 拉伸
export const CTRL_W = '180px'

// 规划中徽标文案(SettingRow 徽标与快捷键表状态列共用)
export const PLANNED = '规划中'

// 单条「规划中」占位行:整行禁用,右侧控件仅有展示值、不可交互
export interface PlannedRow {
  /** 行标题 */
  title: string
  /** 行描述 */
  description: string
  /** 右侧占位控件类型 */
  control: 'switch' | 'select' | 'button'
  /** select 展示值 / button 按钮文字(switch 不需要) */
  value?: string
}
