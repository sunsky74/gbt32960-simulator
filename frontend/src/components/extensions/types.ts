// 扩展包页共享类型:工具栏(PackToolbar)与页面(ExtensionsPage)之间的一致性契约
export type PackStatusFilter = 'all' | 'enabled' | 'disabled' | 'bound' | 'unbound'

export type PackVersionFilter = 'all' | '2016' | '2025'

export interface PackFilterCounts {
  all: number
  enabled: number
  disabled: number
  bound: number
  unbound: number
}
