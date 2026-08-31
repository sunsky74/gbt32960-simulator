import { theme as antdTheme } from 'ant-design-vue'

export type ThemeMode = 'dark' | 'light'

export const THEME_STORAGE_KEY = 'app-theme'

// ----------------------------------------------------------------
// Ant Design 组件主题(ConfigProvider 消费):Dark 为现有基准逐字保留。
// ----------------------------------------------------------------
export const antdThemes: Record<ThemeMode, { algorithm: unknown; token: Record<string, unknown> }> = {
  dark: {
    algorithm: antdTheme.darkAlgorithm,
    token: {
      colorPrimary: '#1677FF',
      colorInfo: '#1677FF',
      colorSuccess: '#52C41A',
      colorWarning: '#FAAD14',
      colorError: '#FF4D4F',
      colorBgLayout: '#141414',
      colorBgContainer: '#262626',
      colorBgElevated: '#2a2a2a',
      colorBorder: 'rgba(255, 255, 255, 0.12)',
      colorBorderSecondary: 'rgba(255, 255, 255, 0.08)',
      colorText: 'rgba(255, 255, 255, 0.88)',
      colorTextSecondary: 'rgba(255, 255, 255, 0.65)',
      colorTextTertiary: 'rgba(255, 255, 255, 0.45)',
      colorTextQuaternary: 'rgba(255, 255, 255, 0.35)',
      borderRadius: 8,
      fontSize: 14,
    },
  },
  light: {
    algorithm: antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: '#1677FF',
      colorInfo: '#1677FF',
      colorSuccess: '#389E0D',
      colorWarning: '#D46B08',
      colorError: '#CF1322',
      colorBgLayout: '#F0F2F5',
      colorBgContainer: '#FFFFFF',
      colorBgElevated: '#FFFFFF',
      colorBorder: 'rgba(0, 0, 0, 0.12)',
      colorBorderSecondary: 'rgba(0, 0, 0, 0.06)',
      colorText: 'rgba(0, 0, 0, 0.88)',
      colorTextSecondary: 'rgba(0, 0, 0, 0.65)',
      colorTextTertiary: 'rgba(0, 0, 0, 0.45)',
      colorTextQuaternary: 'rgba(0, 0, 0, 0.3)',
      borderRadius: 8,
      fontSize: 14,
    },
  },
}

// ----------------------------------------------------------------
// 主题类型与 Ant Design 双主题 token。
// 初始化(读 localStorage → 设置 <html data-theme> → 同步响应式状态)
// 统一走 useTheme.ts 的 initTheme(),保证模块单例与 DOM 属性一致。
// ----------------------------------------------------------------
export function resolveStoredTheme(): ThemeMode {
  const saved = localStorage.getItem(THEME_STORAGE_KEY)
  return saved === 'light' ? 'light' : 'dark'
}
