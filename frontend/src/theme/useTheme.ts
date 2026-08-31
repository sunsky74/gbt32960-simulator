import { ref } from 'vue'
import { THEME_STORAGE_KEY, resolveStoredTheme, type ThemeMode } from './index'

// 模块级单例:App.vue 与 ThemeSwitch 共享同一份响应式状态。
const themeMode = ref<ThemeMode>('dark')

export function useTheme() {
  return { themeMode }
}

export function toggleTheme() {
  apply(themeMode.value === 'dark' ? 'light' : 'dark')
}

export function applyTheme(mode: ThemeMode) {
  apply(mode)
}

// 应用启动时初始化:读 localStorage → 写 <html data-theme> → 同步响应式状态。
// 必须在 mount 前调用(首帧无闪烁);无存档时默认 Dark。
export function initTheme(): ThemeMode {
  const mode = resolveStoredTheme()
  themeMode.value = mode
  document.documentElement.dataset.theme = mode
  return mode
}

function apply(mode: ThemeMode) {
  themeMode.value = mode
  document.documentElement.dataset.theme = mode
  localStorage.setItem(THEME_STORAGE_KEY, mode)
}
