import { reactive, watch } from 'vue'
import { applyTheme, useTheme } from '../theme/useTheme'

// 应用设置(IDE 设置中心的数据模型):localStorage 持久化,不动后端配置。
// 只收录真实生效的项;尚未实现的能力在设置页标记「规划中」,不落入此模型。
export interface AppSettings {
  themeMode: 'dark' | 'light' | 'auto'
  animations: boolean
  restoreLastPage: boolean
  // 前端本地性能上限:服务端报文流保留行数(渲染/内存开销)
  packetStreamCap: number
  // 前端本地性能上限:客户端控制台保留事件条数(内存开销)
  consoleEventCap: number
}

const SETTINGS_KEY = 'app-settings'
const LAST_PAGE_KEY = 'app-last-page'

const DEFAULTS: AppSettings = {
  themeMode: 'dark',
  animations: true,
  restoreLastPage: true,
  packetStreamCap: 200,
  consoleEventCap: 10000,
}

// 数值档位边界(与设置页下拉选项一致;旧数据越界时钳回区间)
const PACKET_STREAM_CAP_RANGE = { min: 100, max: 2000 } as const
const CONSOLE_EVENT_CAP_RANGE = { min: 1000, max: 50000 } as const

// 数值钳制:非法值回落默认,越界值压回允许区间(读写两路共用,坏值不落盘)。
export function clampNum(value: unknown, min: number, max: number, fallback: number): number {
  const n = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(n) || n <= 0) return fallback
  return Math.min(max, Math.max(min, n))
}

function clampCaps(v: AppSettings): AppSettings {
  v.packetStreamCap = clampNum(
    v.packetStreamCap,
    PACKET_STREAM_CAP_RANGE.min,
    PACKET_STREAM_CAP_RANGE.max,
    DEFAULTS.packetStreamCap,
  )
  v.consoleEventCap = clampNum(
    v.consoleEventCap,
    CONSOLE_EVENT_CAP_RANGE.min,
    CONSOLE_EVENT_CAP_RANGE.max,
    DEFAULTS.consoleEventCap,
  )
  return v
}

function load(): AppSettings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY)
    if (!raw) return { ...DEFAULTS }
    const saved = JSON.parse(raw) as Partial<AppSettings>
    return clampCaps({ ...DEFAULTS, ...saved })
  } catch {
    return { ...DEFAULTS }
  }
}

export const appSettings = reactive<AppSettings>(load())

function persist() {
  // 持久化前兜底钳制:任何路径写入的坏值都不落盘
  clampCaps(appSettings)
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(appSettings))
}

// 主题:与既有 THEME_STORAGE_KEY 同源(单一事实),设置页与侧边栏开关共享。
const { themeMode } = useTheme()

watch(
  () => appSettings.themeMode,
  (m) => {
    if (themeMode.value !== m) applyTheme(m)
  },
)

// 反向同步:侧边栏手动切换 → 设置页显示跟随。
watch(themeMode, (m) => {
  if (appSettings.themeMode !== m) appSettings.themeMode = m
})

// 界面动画:关闭时给 <html> 打标,全局抑制过渡/动画(见 style.css)。
function applyAnimations(on: boolean) {
  document.documentElement.toggleAttribute('data-no-anim', !on)
}

watch(
  () => appSettings.animations,
  (on) => {
    applyAnimations(on)
    persist()
  },
  { immediate: true },
)

watch(
  () => appSettings.themeMode,
  () => persist(),
)

watch(
  () => appSettings.restoreLastPage,
  () => persist(),
)

// 性能上限档位:变更即持久化(与上方各设置项同一模式)
watch(
  () => [appSettings.packetStreamCap, appSettings.consoleEventCap] as const,
  () => persist(),
)

// 启动时恢复上次工作区页面(注册于 App 挂载前,见 App.vue)。
export function restoreLastNavPage(availableKeys: string[], fallback: string): string {
  if (!appSettings.restoreLastPage) return fallback
  const saved = localStorage.getItem(LAST_PAGE_KEY)
  return saved && availableKeys.includes(saved) ? saved : fallback
}

export function rememberNavPage(key: string) {
  if (appSettings.restoreLastPage) localStorage.setItem(LAST_PAGE_KEY, key)
}

// 恢复默认设置:清空本工具自管的 localStorage 键并重载(连接档案/扩展包等后端数据不受影响)。
export function resetAppSettings() {
  for (const key of [SETTINGS_KEY, LAST_PAGE_KEY, 'app-theme', 'app-nav-collapsed']) {
    localStorage.removeItem(key)
  }
  location.reload()
}
