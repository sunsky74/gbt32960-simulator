// 更新领域的共享常量与纯函数(设置面板与启动检查共用;不做任何网络调用)。
import { appSettings, type AppSettings } from './useAppSettings'

// 自动检查冷却:24h(与设计文档 §5.6 一致)
export const UPDATE_CHECK_COOLDOWN_MS = 24 * 60 * 60 * 1000
// 启动自动检查延迟:避开启动高峰
export const UPDATE_CHECK_DELAY_MS = 3000

// 是否为开发构建(不参与更新检查)
export function isDevVersion(v: string): boolean {
  return v === '' || v === 'dev'
}

// 自动检查是否应触发:开关开启 且 距上次检查 ≥ 24h(首启 last=0 视为到期;时钟回拨保持抑制)
export function shouldAutoCheck(
  settings: Pick<AppSettings, 'checkUpdateOnStartup' | 'lastUpdateCheckAt'>,
  now: number,
): boolean {
  if (!settings.checkUpdateOnStartup) return false
  return now - settings.lastUpdateCheckAt >= UPDATE_CHECK_COOLDOWN_MS
}

// 发现新版本时是否提示:未被用户跳过
export function shouldPrompt(settings: Pick<AppSettings, 'skippedVersion'>, latest: string): boolean {
  return settings.skippedVersion !== latest
}

// 字节数展示(资产大小)
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

// 记录一次检查完成时间戳(成功与失败统一口径:自动检查 24h 冷却)
export function markChecked(now: number) {
  appSettings.lastUpdateCheckAt = now
}

// 跳过指定版本(自动检查对其静默;更高版本发布后自动恢复提示)
export function skipVersion(latest: string) {
  appSettings.skippedVersion = latest
}

// 上次更新失败的启动告知文案;成功/无记录返回 null(设计 §5.4)
export function lastResultToast(res: { present: boolean; ok: boolean }, currentVersion: string): string | null {
  if (!res.present || res.ok) return null
  return `上次更新未成功,已回滚在 ${currentVersion},详情见日志`
}
