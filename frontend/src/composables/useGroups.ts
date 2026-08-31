import { store } from '../state'
import type { FieldSchema } from '../api/backend'

// 取(或创建)某数据组的第一行 —— 右侧状态卡/报警卡与左侧报文配置共用同一份数据,双向联动
export function ensureRow(groupKey: string): Record<string, unknown> {
  let g = store.groups[groupKey]
  if (!g) {
    g = store.groups[groupKey] = { enabled: true, rows: [] }
  }
  if (!g.rows.length) {
    g.rows.push({})
  }
  return g.rows[0]
}

// 从 schema 里找某个组的字段定义(枚举选项/位定义)
export function fieldOf(groupKey: string, fieldKey: string): FieldSchema | undefined {
  return store.schema.find((g) => g.key === groupKey)?.fields.find((f) => f.key === fieldKey)
}

export function numOf(row: Record<string, unknown>, key: string): number {
  const v = row[key]
  return typeof v === 'number' ? v : Number(v) || 0
}

export function setNum(row: Record<string, unknown>, key: string, v: unknown) {
  row[key] = typeof v === 'number' ? v : Number(v) || 0
}

export function bitsObjOf(row: Record<string, unknown>, field: FieldSchema): Record<string, boolean> {
  const v = row[field.key]
  return (v && typeof v === 'object' ? v : {}) as Record<string, boolean>
}

export function bitsArrayOf(row: Record<string, unknown>, field: FieldSchema): string[] {
  return Object.entries(bitsObjOf(row, field))
    .filter(([, on]) => on)
    .map(([k]) => k)
}

export function setBitsArray(row: Record<string, unknown>, field: FieldSchema, vals: Array<string | number | boolean>) {
  const picked = new Set(vals.map(String))
  const bits: Record<string, boolean> = {}
  for (const b of field.bits ?? []) bits[`bit${b.index}`] = picked.has(`bit${b.index}`)
  row[field.key] = bits
}
