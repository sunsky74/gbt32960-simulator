import { describe, expect, it } from 'vitest'
import { appSettings } from './useAppSettings'
import {
  UPDATE_CHECK_COOLDOWN_MS,
  formatBytes,
  isDevVersion,
  lastResultToast,
  markChecked,
  shouldAutoCheck,
  shouldPrompt,
  skipVersion,
} from './useUpdater'

describe('isDevVersion', () => {
  it('dev/空串视为开发构建', () => {
    expect(isDevVersion('dev')).toBe(true)
    expect(isDevVersion('')).toBe(true)
    expect(isDevVersion('v0.1.0')).toBe(false)
  })
})

describe('shouldAutoCheck', () => {
  const base = { checkUpdateOnStartup: true, lastUpdateCheckAt: 0 }
  it('开关关闭时不检查', () => {
    expect(shouldAutoCheck({ ...base, checkUpdateOnStartup: false }, 1e12)).toBe(false)
  })
  it('首启(从未检查)到期', () => {
    expect(shouldAutoCheck(base, 1e12)).toBe(true)
  })
  it('24h 内不重复检查;恰满 24h 到期;时钟回拨保持抑制', () => {
    const now = 1e12
    expect(shouldAutoCheck({ ...base, lastUpdateCheckAt: now - UPDATE_CHECK_COOLDOWN_MS + 1 }, now)).toBe(false)
    expect(shouldAutoCheck({ ...base, lastUpdateCheckAt: now - UPDATE_CHECK_COOLDOWN_MS }, now)).toBe(true)
    expect(shouldAutoCheck({ ...base, lastUpdateCheckAt: now + 60000 }, now)).toBe(false)
  })
})

describe('shouldPrompt', () => {
  it('被跳过的版本不提示,更高版本恢复提示', () => {
    expect(shouldPrompt({ skippedVersion: 'v0.2.0' }, 'v0.2.0')).toBe(false)
    expect(shouldPrompt({ skippedVersion: 'v0.2.0' }, 'v0.2.1')).toBe(true)
    expect(shouldPrompt({ skippedVersion: '' }, 'v0.2.0')).toBe(true)
  })
})

describe('formatBytes', () => {
  it('B/KB/MB 分档', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(11430000)).toBe('10.9 MB')
  })
})

describe('markChecked / skipVersion', () => {
  it('写入设置字段', () => {
    markChecked(123)
    expect(appSettings.lastUpdateCheckAt).toBe(123)
    skipVersion('v9.9.9')
    expect(appSettings.skippedVersion).toBe('v9.9.9')
  })
})

describe('lastResultToast', () => {
  it.each<[{ present: boolean; ok: boolean }, string, string | null]>([
    [{ present: false, ok: false }, 'v0.1.0', null],
    [{ present: true, ok: true }, 'v0.1.0', null],
    [{ present: true, ok: false }, 'v0.1.0', '上次更新未成功,已回滚在 v0.1.0,详情见日志'],
  ])('present/ok → 固定文案或静默(%#)', (res, currentVersion, expected) => {
    expect(lastResultToast(res, currentVersion)).toBe(expected)
  })
})
