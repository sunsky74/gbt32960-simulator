import * as ConnectionService from '../../wailsjs/go/bridge/ConnectionService'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import * as ConsoleService from '../../wailsjs/go/bridge/ConsoleService'
import { bridge, engine, schema } from '../../wailsjs/go/models'

export { ConnectionService, MessageService, ConsoleService }

export type ConnConfig = bridge.ConnectionConfig
export type TestResult = bridge.TestResult
export type GroupSchema = schema.GroupSchema
export type FieldSchema = schema.FieldSchema
export type GroupsPayload = schema.GroupsPayload
export type ParamResponseRow = engine.ParamResponseRow
export type ExportResult = bridge.ExportResult

/** 控制台事件(与 Go internal/engine.Event 对齐,经 EventsEmit 批量推送) */
export interface ConsoleEvent {
  time: string
  kind: 'conn' | 'tx' | 'rx' | 'error'
  message?: string
  cmd?: string
  hex?: string
  decoded?: Record<string, unknown>
  bytes?: number
  downlink?: DownlinkInfo
}

/** 平台下行命令解析结果(与 Go engine.DownlinkInfo 对齐) */
export interface DownlinkInfo {
  cmd: number
  kind: 'query' | 'setup' | 'control' | 'remote'
  paramIds?: number[]
  params?: Array<{ id: number; length: number; hex: string }>
  controlHex?: string
  commandTime?: string
  serialNumber?: number
  commandCount?: number
  infoTypeFlag?: number
  headerHex?: string
  bodyHex?: string
}

/** 前端内部的组配置形态:key → {enabled, rows} */
export interface GroupState {
  enabled: boolean
  rows: Array<Record<string, unknown>>
}

export type GroupsState = Record<string, GroupState>

export function payloadToState(p: GroupsPayload): GroupsState {
  const out: GroupsState = {}
  for (const g of p.groups ?? []) {
    out[g.key] = { enabled: g.enabled, rows: (g.rows ?? []) as Array<Record<string, unknown>> }
  }
  return out
}

export function stateToPayload(groups: GroupsState, order: string[]): GroupsPayload {
  const payload = new schema.GroupsPayload()
  const list: schema.NamedGroup[] = []
  for (const key of order) {
    const g = groups[key]
    if (!g) continue
    const ng = new schema.NamedGroup()
    ng.key = key
    ng.enabled = g.enabled
    ng.rows = g.rows
    list.push(ng)
  }
  payload.groups = list
  return payload
}

/**
 * console:events 订阅(兼容导出):实现已收口到 api/events.ts,
 * 此处仅转发以维持既有 import 路径可用。
 */
export { onConsoleEvents } from './events'
