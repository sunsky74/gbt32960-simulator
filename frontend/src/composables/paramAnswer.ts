// 0x80 参数查询应答:取值校验与编码(纯函数,无副作用)。
// 规则全部来自 docs/standard/2016.md 表B.12 / docs/standard/2025.md 表B.8 的参数定义:
// - u8/u16:十进制输入,取值 ∈ [rawMin, rawMax],或等于该宽度的标记值
//   (1 字节:0xFE 异常 / 0xFF 无效;2 字节:0xFFFE / 0xFFFF)
// - string:定长 hex,必须恰好 length 字节(如 0x07/0x08 硬件/固件版本为 5 字节)
// - bytes 且 length=0:变长 hex(任意整字节,长度由同一应答中 0x04/0x0D 行携带)
// - options:枚举下拉,编码为 1 字节
// - 无定义 id(预留 0x11~0x7F / 厂商 0x80~0xFE):自由 hex
import type { engine } from '../../wailsjs/go/models'

type ParamSpec = engine.ParamSpec

// 各宽度的异常/无效标记值
const SENTINELS: Record<number, number[]> = {
  1: [0xfe, 0xff],
  2: [0xfffe, 0xffff],
}

export type AnswerInputKind = 'number' | 'hex' | 'select'

// 行内输入形态:有枚举走下拉,u8/u16 走十进制输入,其余(定长/变长/未知)走 hex
export function answerInputKind(spec?: ParamSpec): AnswerInputKind {
  if (!spec) return 'hex'
  if (spec.options && spec.options.length > 0) return 'select'
  if (spec.type === 'u8' || spec.type === 'u16') return 'number'
  return 'hex'
}

// 行初始值:枚举取第一项,数值取范围下限(默认 0),hex 留空
export function defaultAnswerValue(spec?: ParamSpec): string {
  if (spec?.options && spec.options.length > 0) return String(spec.options[0].value)
  if (spec && (spec.type === 'u8' || spec.type === 'u16')) return String(spec.rawMin ?? 0)
  return ''
}

// 行下方的小字提示:spec.note + 变长域名的长度说明
export function answerHint(spec?: ParamSpec): string {
  if (!spec) return '标准未定义该参数(预留/厂商自定义),可填写任意整字节 hex'
  const parts: string[] = []
  if (spec.note) parts.push(spec.note)
  if (
    spec.type === 'bytes' &&
    spec.length === 0 &&
    !parts.some((p) => p.includes('0x04') || p.includes('0x0D'))
  ) {
    parts.push('长度由同一应答中 0x04/0x0D 行携带')
  }
  return parts.join(';')
}

// 校验并编码一行应答值,返回小写 hex;非法输入抛 Error(由调用方 toast 提示)
export function encodeAnswerValue(spec: ParamSpec | undefined, value: string | number): string {
  if (!spec) return freeHex(String(value))
  if (spec.options && spec.options.length > 0) {
    const n = Number(value)
    if (!spec.options.some((o) => o.value === n)) throw new Error('请选择有效选项')
    return (n & 0xff).toString(16).padStart(2, '0')
  }
  switch (spec.type) {
    case 'u8':
      return encodeUint(value, 1, spec.rawMin, spec.rawMax)
    case 'u16':
      return encodeUint(value, 2, spec.rawMin, spec.rawMax)
    case 'string':
      return fixedHex(String(value), spec.length)
    case 'bytes':
      return spec.length > 0 ? fixedHex(String(value), spec.length) : freeHex(String(value))
    default:
      return freeHex(String(value))
  }
}

// 定宽数值:十进制 → 固定宽度 hex;标记值不受 rawMin/rawMax 约束
function encodeUint(value: string | number, width: 1 | 2, rawMin?: number, rawMax?: number): string {
  const s = String(value).trim()
  if (s === '') throw new Error('请输入十进制数值')
  const n = Number(s)
  if (!Number.isInteger(n) || n < 0) throw new Error('需为非负整数')
  const cap = width === 1 ? 0xff : 0xffff
  if (n > cap) throw new Error(`超出 ${width} 字节上限 ${cap}`)
  const sentinels = SENTINELS[width] ?? []
  if (!sentinels.includes(n)) {
    const min = rawMin ?? 0
    const max = rawMax ?? cap
    if (n < min || n > max) {
      const marks = sentinels.map((v) => '0x' + v.toString(16).toUpperCase()).join('/')
      throw new Error(`有效范围 ${min}~${max},或标记值 ${marks}`)
    }
  }
  return n.toString(16).padStart(width * 2, '0')
}

function cleanHex(v: string): string {
  return v.trim().toLowerCase().replace(/\s+/g, '')
}

// 定长 hex:必须恰好 length 字节
function fixedHex(v: string, length: number): string {
  const s = cleanHex(v)
  if (!/^[0-9a-f]*$/.test(s)) throw new Error('仅允许 hex 字符(0-9/a-f)')
  if (s.length !== length * 2) throw new Error(`需为 ${length} 字节 hex,即 ${length * 2} 个 hex 字符`)
  return s
}

// 变长/自由 hex:任意整字节数
function freeHex(v: string): string {
  const s = cleanHex(v)
  if (!/^[0-9a-f]*$/.test(s)) throw new Error('仅允许 hex 字符(0-9/a-f)')
  if (s.length % 2 !== 0) throw new Error('hex 长度需为偶数(整字节)')
  return s
}
