import { reactive } from 'vue'
import { message } from 'ant-design-vue'
import {
  ConnectionService,
  MessageService,
  payloadToState,
  type ConnConfig,
  type ConsoleEvent,
  type GroupSchema,
  type GroupsState,
} from './api/backend'
import type { bridge } from '../wailsjs/go/models'

export const store = reactive({
  connState: 'idle',
  connBusy: false,
  config: null as ConnConfig | null,
  profiles: [] as bridge.ProfileSummary[],
  schema: [] as GroupSchema[],
  groups: {} as GroupsState,
  consoleEvents: [] as ConsoleEvent[],
  consolePaused: false,
  consoleFilter: 'all' as 'all' | 'tx' | 'rx' | 'conn' | 'error',
})

const MAX_CONSOLE_EVENTS = 10000

export function pushConsoleEvents(batch: ConsoleEvent[]) {
  if (store.consolePaused) return
  store.consoleEvents.push(...batch)
  if (store.consoleEvents.length > MAX_CONSOLE_EVENTS) {
    store.consoleEvents.splice(0, store.consoleEvents.length - MAX_CONSOLE_EVENTS)
  }
}

export function showToast(msg: string) {
  message.error(msg)
}

export async function refreshState() {
  try {
    store.connState = await ConnectionService.State()
  } catch {
    store.connState = 'idle'
  }
}

export async function loadProfiles() {
  try {
    store.profiles = await ConnectionService.GetProfiles()
  } catch {
    store.profiles = []
  }
}

export async function loadInitialData() {
  try {
    store.config = await ConnectionService.GetConfig()
    store.schema = await MessageService.GetSchema(store.config?.version ?? '2016')
    const payload = await MessageService.GetGroups()
    store.groups = payloadToState(payload)
    await loadProfiles()
  } catch (e) {
    showToast('初始化失败: ' + String(e))
  }
}

export async function reloadSchemaForVersion(version: string) {
  store.schema = await MessageService.GetSchema(version)
  const payload = await MessageService.DefaultGroups(version)
  store.groups = payloadToState(payload)
}

export function filteredConsole(): ConsoleEvent[] {
  if (store.consoleFilter === 'all') return store.consoleEvents
  return store.consoleEvents.filter((e) => e.kind === store.consoleFilter)
}

export function connStateText(s: string): string {
  switch (s) {
    case 'online':
      return '在线'
    case 'connecting':
      return '连接中'
    case 'loggingIn':
      return '登录中'
    default:
      return '离线'
  }
}
