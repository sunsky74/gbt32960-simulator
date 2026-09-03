import { computed, ref } from 'vue'
import { THEME_STORAGE_KEY, resolveStoredTheme, type ThemeMode } from './index'

// 模块级单例:App.vue、ThemeSwitch 与设置中心共享同一份响应式状态。
// themeMode 含 'auto'(跟随系统);resolvedMode 为实际渲染的 dark/light。
const themeMode = ref<ThemeMode>('dark')
const systemDark = ref(false)

let mediaQuery: MediaQueryList | null = null

function systemPrefersDark(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches
}

// auto 模式下监听系统主题变化,实时重渲染(仅注册一次)。
function watchSystem() {
  if (mediaQuery || typeof window.matchMedia !== 'function') return
  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  mediaQuery.addEventListener('change', (e) => {
    systemDark.value = e.matches
    if (themeMode.value === 'auto') paint()
  })
}

export function useTheme() {
  return { themeMode, resolvedMode: computed(() => resolvedOf(themeMode.value)) }
}

function resolvedOf(mode: ThemeMode): 'dark' | 'light' {
  if (mode === 'auto') return systemDark.value ? 'dark' : 'light'
  return mode
}

export function toggleTheme() {
  // 侧边栏手动切换:从当前实际主题切到另一侧(auto 亦按实际值取反,退出跟随系统)
  apply(resolvedOf(themeMode.value) === 'dark' ? 'light' : 'dark')
}

export function applyTheme(mode: ThemeMode) {
  apply(mode)
}

// 应用启动时初始化:读 localStorage → 写 <html data-theme> → 同步响应式状态。
// 必须在 mount 前调用(首帧无闪烁);无存档时默认 Dark。
export function initTheme(): ThemeMode {
  watchSystem()
  systemDark.value = systemPrefersDark()
  const mode = resolveStoredTheme()
  themeMode.value = mode
  paint()
  return mode
}

function apply(mode: ThemeMode) {
  themeMode.value = mode
  localStorage.setItem(THEME_STORAGE_KEY, mode)
  paint()
}

function paint() {
  document.documentElement.dataset.theme = resolvedOf(themeMode.value)
}
