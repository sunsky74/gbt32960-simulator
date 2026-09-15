// SettingsAboutPanel:版本展示 / 检查更新 / 跳过版本 / 下载与校验(UpdaterService 全 mock)
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { message } from 'ant-design-vue'

const checkUpdate = vi.fn()
const downloadUpdate = vi.fn()
const cancelDownload = vi.fn()
let currentVersion = 'v0.1.0'
vi.mock('../../../wailsjs/go/bridge/UpdaterService', () => ({
  CurrentVersion: vi.fn(async () => currentVersion),
  CheckUpdate: (...args: unknown[]) => checkUpdate(...args),
  DownloadUpdate: (...args: unknown[]) => downloadUpdate(...args),
  CancelDownload: (...args: unknown[]) => cancelDownload(...args),
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  BrowserOpenURL: vi.fn(),
}))

// 进度事件处理器捕获(vi.hoisted 供 mock 工厂引用)
const h = vi.hoisted(() => ({ progressHandler: null as null | ((p: unknown) => void) }))
vi.mock('../../api/events', () => ({
  onWailsEvent: vi.fn((_name: string, handler: (p: unknown) => void) => {
    h.progressHandler = handler
    return () => {
      h.progressHandler = null
    }
  }),
}))

import SettingsAboutPanel from './SettingsAboutPanel.vue'
import { appSettings } from '../../composables/useAppSettings'

// a-button 渲染为真实按钮,便于点击与文本断言;声明 emits 防止 @click 经 attrs 透传到根元素导致一次点击双触发
const stubs = {
  'a-button': { template: '<button @click="$emit(\'click\')"><slot /></button>', emits: ['click'] },
  'a-progress': { template: '<div class="stub-progress"></div>' },
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
    downloadUpdate.mockReset()
    cancelDownload.mockReset()
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

  describe('下载与校验', () => {
    it('下载:进度事件驱动文案,完成后展示就绪态', async () => {
      // 可控 promise:进度事件必须在下载挂起期间发出(否则 finally 已清理订阅与状态)
      let resolveDownload: (v: unknown) => void = () => {}
      downloadUpdate.mockImplementation(() => new Promise((res) => { resolveDownload = res }))
      const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
      await flushPromises()
      await findBtn(wrapper, '检查更新').trigger('click')
      await flushPromises()

      await findBtn(wrapper, '下载更新').trigger('click')
      await flushPromises()
      h.progressHandler?.({ phase: 'downloading', received: 512, total: 1024, percent: 50 })
      await flushPromises()
      expect(wrapper.text()).toContain('512 B / 1.0 KB')

      resolveDownload({ tag: 'v0.2.0', assetName: 'gbt32960-simulator.app.zip', size: 200, sha256: 'x' })
      await flushPromises()
      expect(wrapper.text()).toContain('更新包已就绪:v0.2.0')
      expect(downloadUpdate).toHaveBeenCalledTimes(1)
    })

    it('取消:不弹错误提示并回到可下载态', async () => {
      let rejectDownload: (e: unknown) => void = () => {}
      downloadUpdate.mockImplementation(
        () => new Promise((_, reject) => { rejectDownload = reject }),
      )
      cancelDownload.mockResolvedValue(undefined)

      const errSpy = vi.spyOn(message, 'error')
      const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
      await flushPromises()
      await findBtn(wrapper, '检查更新').trigger('click')
      await flushPromises()
      await findBtn(wrapper, '下载更新').trigger('click')
      await flushPromises()

      await findBtn(wrapper, '取消下载').trigger('click')
      rejectDownload(new Error('已取消下载')) // 生产同形:Wails 拒绝值为 Error(非裸字符串)
      await flushPromises()

      expect(cancelDownload).toHaveBeenCalledTimes(1)
      expect(errSpy).not.toHaveBeenCalled()
      expect(wrapper.text()).toContain('下载更新') // 回到可下载态
      errSpy.mockRestore()
    })

    it('下载失败:展示纯文案(无 Error 前缀)', async () => {
      downloadUpdate.mockRejectedValue(new Error('发布未附校验信息,已拒绝更新'))
      const errSpy = vi.spyOn(message, 'error')
      const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
      await flushPromises()
      await findBtn(wrapper, '检查更新').trigger('click')
      await flushPromises()
      await findBtn(wrapper, '下载更新').trigger('click')
      await flushPromises()
      expect(errSpy).toHaveBeenCalledWith('发布未附校验信息,已拒绝更新')
      errSpy.mockRestore()
    })
  })
})
