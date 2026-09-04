// 服务端工作台共享类型与格式化工具(纯函数,无副作用)

export type FrameKind = 'normal' | 'unknown' | 'encrypted' | 'warn' | 'link'
export type FrameDir = 'rx' | 'tx' | 'link' // link = 非 rx/tx 的伪方向(链路事件/告警)

// 报文流统一行:server:frame + server:session(合成紫色 Link 行)+ server:warn(红色 Error 行)
export interface StreamRow {
  id: number
  time: string // ISO;link/warn 行取前端接收时刻
  vin: string
  cmd: string // '0x01' 形式;link/warn 行为空
  hex: string
  summary: string
  kind: FrameKind
  dir: FrameDir
  unauthed?: boolean
  peer?: string // link 行:对端地址
}

export interface SessionRow {
  vin: string
  peer: string
  online: boolean
  loginAt: string
  lastSeen: string
  rxCount: number
  txCount: number
}

// 前端渲染上限(后端环形 500,spec §5.4)
export const RENDER_CAP = 200

// 服务端已知命令字名称(与 internal/servermode/conn.go 处理器矩阵一致)
const GB32960_CMD_NAMES: Record<string, string> = {
  '0X01': '登入',
  '0X02': '实时上报',
  '0X03': '补发上报',
  '0X04': '登出',
  '0X07': '心跳',
  '0X08': '校时',
}

// 命令名称:后端 cmd 为 '0x%02X' 字符串,未知命令字由 kind=unknown 橙色标注
export function cmdNameOf(cmd: string): string {
  const key = (cmd || '').toUpperCase()
  if (!key) return ''
  return GB32960_CMD_NAMES[key] ?? '未知命令'
}

function pad(n: number, w = 2): string {
  return String(n).padStart(w, '0')
}

// 报文流时间列:HH:mm:ss.SSS(毫秒精度)
export function fmtMs(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
}

// 会话最后活跃:HH:mm:ss
export function fmtClock(iso: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

// 时长:mm:ss(满 1h 进位为 h:mm:ss);每秒跳动由父级 now 驱动
export function fmtDuration(fromMs: number, nowMs: number): string {
  const s = Math.max(0, Math.floor((nowMs - fromMs) / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = s % 60
  return h > 0 ? `${h}:${pad(m)}:${pad(ss)}` : `${pad(m)}:${pad(ss)}`
}
