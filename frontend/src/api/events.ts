// Wails 运行时事件订阅的统一类型化出口。
// 全前端唯一的 EventsOn 调用点:页面一律经由 onWailsEvent / onConsoleEvents 订阅,
// 事件名与载荷类型在此集中登记,事件名拼写错误在编译期即被发现。
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { ConsoleEvent } from './backend'

// ---------- 事件载荷类型(镜像 internal/servermode/types.go 的 json tag) ----------

export type ServerFrameKind = 'normal' | 'unknown' | 'encrypted'

/** server:status —— Go servermode.Status */
export interface ServerStatusEvent {
  running: boolean
  listenAddr: string
}

/** server:session —— Go servermode.SessionEvent */
export interface ServerSessionEvent {
  vin: string
  peer: string
  online: boolean
  lastSeen: string // Go time.Time 序列化为 RFC3339 字符串
  platform?: boolean
}

/** server:frame —— Go servermode.FrameEvent */
export interface ServerFrameEvent {
  time: string
  vin: string
  cmd: string
  hex: string
  summary: string
  kind: ServerFrameKind
  unauthed?: boolean
  dir?: 'rx' | 'tx' // Go 端 omitempty,缺省 rx(向后兼容)
  platform?: boolean
}

/** server:warn —— Go servermode.WarnEvent */
export interface ServerWarnEvent {
  note: string
  hex: string
}

/** update:progress —— bridge.UpdaterService 下载进度(设计文档 §5.5;Go 端 progressDTO) */
export interface UpdateProgressEvent {
  phase: 'downloading' | 'verifying'
  received: number
  total: number
  percent: number
}

// ---------- 事件名 → 载荷 映射 ----------

export interface WailsEventMap {
  'console:events': ConsoleEvent[]
  'server:status': ServerStatusEvent
  'server:session': ServerSessionEvent
  'server:frame': ServerFrameEvent
  'server:warn': ServerWarnEvent
  'update:progress': UpdateProgressEvent
}

/**
 * 订阅 Wails 事件;返回幂等的注销函数(重复调用安全,只注销一次)。
 * Wails 回调形参为 (...data: any),Go 端单载荷 emit,故取首参并收窄为映射类型。
 */
export function onWailsEvent<K extends keyof WailsEventMap>(
  name: K,
  handler: (payload: WailsEventMap[K]) => void,
): () => void {
  const off = EventsOn(name, (...data: unknown[]) => {
    handler(data[0] as WailsEventMap[K])
  })
  let disposed = false
  return () => {
    if (disposed) return
    disposed = true
    off()
  }
}

/** console:events(客户端引擎控制台批量事件)——实现收口在本模块,backend.ts 兼容转发到此 */
export function onConsoleEvents(handler: (batch: ConsoleEvent[]) => void): () => void {
  return onWailsEvent('console:events', handler)
}
