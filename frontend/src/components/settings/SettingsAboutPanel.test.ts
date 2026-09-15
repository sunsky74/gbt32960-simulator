// SettingsAboutPanel:版本展示 / 检查更新 / 跳过版本(UpdaterService 全 mock)
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const checkUpdate = vi.fn()
let currentVersion = 'v0.1.0'
vi.mock('../../../wailsjs/go/bridge/UpdaterService', () => ({
  CurrentVersion: vi.fn(async () => currentVersion),
  CheckUpdate: (...args: unknown[]) => checkUpdate(...args),
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  BrowserOpenURL: vi.fn(),
}))

import SettingsAboutPanel from './SettingsAboutPanel.vue'
import { appSettings } from '../../composables/useAppSettings'

// a-button 渲染为真实按钮,便于点击与文本断言
const stubs = {
  'a-button': { template: '<button @click="$emit(\'click\')"><slot /></button>' },
}

function findBtn(wrapper: ReturnType<typeof mount>, text: string) {
  const btn = wrapper.findAll('button').find((b) => b.text().includes(text))
  if (!btn) throw new Error(`未找到按钮: ${text}`)
  return btn
}

describe('SettingsAboutPanel', () => {
  beforeEach(() => {
    currentVersion = 'v0.1.0'
    checkUpdate.mockReset()
    appSettings.skippedVersion = ''
    checkUpdate.mockResolvedValue({
      current: 'v0.1.0',
      latest: 'v0.2.0',
      hasUpdate: true,
      devBuild: false,
      notes: '本次更新说明',
      assetName: 'gbt32960-simulator.app.zip',
      assetSize: 11430000,
    })
  })

  it('展示当前版本;检查后展示新版本/说明;可跳过版本', async () => {
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('v0.1.0')

    await findBtn(wrapper, '检查更新').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('v0.2.0')
    expect(wrapper.text()).toContain('本次更新说明')
    expect(wrapper.text()).toContain('10.9 MB')

    await findBtn(wrapper, '跳过此版本').trigger('click')
    expect(appSettings.skippedVersion).toBe('v0.2.0')
  })

  it('开发构建:显示标签且检查按钮禁用', async () => {
    currentVersion = 'dev'
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('开发构建')
    expect(findBtn(wrapper, '检查更新').attributes('disabled')).toBeDefined()
  })
})
