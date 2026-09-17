// SettingsAboutPanel:版本展示 / 检查更新 / 跳过版本 / 下载与校验 / 安装并重启(UpdaterService 等绑定全 mock)
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { Modal, message } from 'ant-design-vue'

const checkUpdate = vi.fn()
const downloadUpdate = vi.fn()
const cancelDownload = vi.fn()
const applyUpdate = vi.fn()
let currentVersion = 'v0.1.0'
vi.mock('../../../wailsjs/go/bridge/UpdaterService', () => ({
  CurrentVersion: vi.fn(async () => currentVersion),
  CheckUpdate: (...args: unknown[]) => checkUpdate(...args),
  DownloadUpdate: (...args: unknown[]) => downloadUpdate(...args),
  CancelDownload: (...args: unknown[]) => cancelDownload(...args),
  ApplyUpdate: (...args: unknown[]) => applyUpdate(...args),
}))

// 运行态查询绑定:由用例按需设定组合(客户端状态 / 服务端是否运行中)
let mockedConnectionState = 'idle'
let mockedServerRunning = false
vi.mock('../../../wailsjs/go/bridge/ConnectionService', () => ({
  State: vi.fn(async () => mockedConnectionState),
}))
vi.mock('../../../wailsjs/go/bridge/ServerService', () => ({
  Status: vi.fn(async () => ({ running: mockedServerRunning, listenAddr: ':12345' })),
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
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'

const REPO_URL = 'https://github.com/sunsky74/gbt32960-simulator'

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
    applyUpdate.mockReset()
    appSettings.skippedVersion = ''
    vi.mocked(BrowserOpenURL).mockClear()
    checkUpdate.mockResolvedValue({
      current: 'v0.1.0',
      latest: 'v0.2.0',
      hasUpdate: true,
      devBuild: false,
      assetName: 'gbt32960-simulator.app.zip',
    })
  })

  it('展示当前版本;检查后展示新版本并可打开发布说明/跳过版本', async () => {
    const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('v0.1.0')

    await findBtn(wrapper, '检查更新').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('v0.2.0')

    await findBtn(wrapper, '查看发布说明').trigger('click')
    expect(BrowserOpenURL).toHaveBeenCalledWith(`${REPO_URL}/releases/tag/v0.2.0`)

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

  describe('安装并重启', () => {
    // Modal.confirm 捕获:不真正弹窗,取出配置供断言与手动触发 onOk
    type ConfirmCfg = {
      title?: string
      content?: string
      okText?: string
      cancelText?: string
      onOk?: () => Promise<void>
    }
    let captured: ConfirmCfg | null = null
    let confirmSpy: { mockRestore: () => void }

    beforeEach(() => {
      captured = null
      mockedConnectionState = 'idle'
      mockedServerRunning = false
      confirmSpy = vi.spyOn(Modal, 'confirm').mockImplementation((cfg) => {
        captured = cfg as unknown as ConfirmCfg
        return {} as never
      })
    })

    afterEach(() => confirmSpy.mockRestore())

    // 挂载 → 检查 → 下载完成,进入就绪态(安装并重启可用)
    async function mountReady() {
      const wrapper = mount(SettingsAboutPanel, { global: { stubs } })
      await flushPromises()
      await findBtn(wrapper, '检查更新').trigger('click')
      await flushPromises()
      downloadUpdate.mockResolvedValue({
        tag: 'v0.2.0',
        assetName: 'gbt32960-simulator.app.zip',
        size: 200,
        sha256: 'x',
      })
      await findBtn(wrapper, '下载更新').trigger('click')
      await flushPromises()
      return wrapper
    }

    it('确认框按运行态动态追加中断文案(四种组合精确匹配)', async () => {
      const cases = [
        { conn: 'idle', running: false, hint: '安装过程中将退出。' },
        { conn: 'online', running: false, hint: '安装过程中将断开连接并退出。' },
        { conn: 'idle', running: true, hint: '安装过程中将停止服务并退出。' },
        { conn: 'online', running: true, hint: '安装过程中将断开连接、停止服务并退出。' },
      ]
      for (const c of cases) {
        const wrapper = await mountReady()
        mockedConnectionState = c.conn
        mockedServerRunning = c.running
        await findBtn(wrapper, '安装并重启').trigger('click')
        await flushPromises()

        expect(captured?.title).toBe('安装并重启')
        expect(captured?.content).toBe(`将安装 v0.2.0。${c.hint}`)
        expect(captured?.okText).toBe('安装并重启')
        expect(captured?.cancelText).toBe('取消')
      }
    })

    it('确认后进入 applying:ApplyUpdate 恰一次且就绪按钮不再出现', async () => {
      applyUpdate.mockResolvedValue(undefined)
      const wrapper = await mountReady()
      await findBtn(wrapper, '安装并重启').trigger('click')
      await flushPromises()

      await captured?.onOk?.()
      await flushPromises()

      expect(applyUpdate).toHaveBeenCalledTimes(1)
      expect(wrapper.text()).toContain('正在安装并重启,应用将在数秒内退出…')
      expect(wrapper.findAll('button').filter((b) => b.text().includes('安装并重启'))).toHaveLength(0)
      expect(findBtn(wrapper, '检查更新').attributes('disabled')).toBeDefined()
    })

    it('取消不触发:ApplyUpdate 零调用且保持就绪态', async () => {
      const wrapper = await mountReady()
      await findBtn(wrapper, '安装并重启').trigger('click')
      await flushPromises()

      expect(applyUpdate).not.toHaveBeenCalled()
      expect(wrapper.text()).toContain('更新包已就绪:v0.2.0')
      expect(findBtn(wrapper, '安装并重启').exists()).toBe(true)
    })

    it('ApplyUpdate 拒绝:纯文案 toast 且保持可重试', async () => {
      applyUpdate.mockRejectedValue(new Error('更新包未就绪,请先下载'))
      const errSpy = vi.spyOn(message, 'error')
      const wrapper = await mountReady()
      await findBtn(wrapper, '安装并重启').trigger('click')
      await flushPromises()

      await captured?.onOk?.().catch(() => {}) // 拒绝被组件重抛(antd 依赖),测试侧吞掉
      await flushPromises()

      expect(errSpy).toHaveBeenCalledWith('更新包未就绪,请先下载')
      expect(wrapper.text()).not.toContain('正在安装并重启')
      expect(findBtn(wrapper, '安装并重启').exists()).toBe(true)
      errSpy.mockRestore()
    })
  })
})
