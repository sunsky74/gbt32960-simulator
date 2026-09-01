import type { FieldSchema } from '../api/backend'

export function defaultFor(f: FieldSchema): unknown {
  switch (f.kind) {
    case 'enum':
      return f.enum?.[0]?.value ?? 1
    case 'bool':
      return false
    case 'bitgroup': {
      const bits: Record<string, boolean> = {}
      for (const b of f.bits ?? []) bits[`bit${b.index}`] = false
      return bits
    }
    case 'array_float':
      return []
    case 'bytes':
      return f.length ? '00'.repeat(f.length) : ''
    case 'int':
      return f.min ?? 0
    default:
      return f.min ?? 0
  }
}

export function numOf(row: Record<string, unknown>, key: string): number {
  const v = row[key]
  return typeof v === 'number' ? v : Number(v) || 0
}

export function setNum(row: Record<string, unknown>, key: string, v: number | string | null | undefined) {
  row[key] = typeof v === 'number' ? v : Number(v) || 0
}

export function boolOf(row: Record<string, unknown>, key: string): boolean {
  return row[key] === true
}

export function setBool(row: Record<string, unknown>, key: string, v: unknown) {
  row[key] = v === true
}

export function setEnum(row: Record<string, unknown>, key: string, v: unknown) {
  row[key] = typeof v === 'number' ? v : Number(v) || 0
}

export function arrayValues(row: Record<string, unknown>, key: string): number[] {
  const v = row[key]
  return Array.isArray(v) ? v.map(Number) : []
}

export function setArrayValue(row: Record<string, unknown>, key: string, idx: number, v: number | string | null | undefined) {
  const arr = arrayValues(row, key).slice()
  arr[idx] = typeof v === 'number' ? v : Number(v) || 0
  row[key] = arr
}

export function addArrayItem(row: Record<string, unknown>, key: string) {
  const arr = arrayValues(row, key)
  arr.push(0)
  row[key] = arr
}

export function hexOf(row: Record<string, unknown>, key: string): string {
  const v = row[key]
  return typeof v === 'string' ? v : ''
}

export function setHex(row: Record<string, unknown>, key: string, f: FieldSchema, v: string) {
  const s = v.trim().toLowerCase().replace(/\s+/g, '')
  if (s === '') {
    row[key] = ''
    return
  }
  if (!/^[0-9a-f]*$/.test(s)) return
  if (f.length && s.length > f.length * 2) return
  row[key] = s
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

export function bitOptions(field: FieldSchema) {
  return (field.bits ?? []).map((b) => ({ label: b.label, value: `bit${b.index}` }))
}
