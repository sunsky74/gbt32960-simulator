// ConsolePanel 集成测试:详情行内的 JsonTree 渲染与冒泡隔离
// Mock 约定:api/backend 与 wailsjs 绑定仅保证模块可导入,测试不触发任何后端调用;
// 数据通过 store.consoleEvents 直接注入(与 console:events 事件推送后的形态一致)
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
}))

vi.mock('../../wailsjs/go/bridge/MessageService', () => ({
  GetSchema: vi.fn(async () => []),
  GetGroups: vi.fn(async () => null),
  DefaultGroups: vi.fn(async () => null),
  RespondParamQuery: vi.fn(async () => null),
  RespondAck: vi.fn(async () => null),
}))

vi.mock('../api/backend', () => ({
  ConnectionService: {
    State: vi.fn(async () => 'idle'),
    GetProfiles: vi.fn(async () => []),
    GetConfig: vi.fn(async () => null),
  },
  MessageService: {
    GetSchema: vi.fn(async () => []),
    GetGroups: vi.fn(async () => null),
    DefaultGroups: vi.fn(async () => null),
  },
  ConsoleService: {
    ExportConsole: vi.fn(async () => null),
    ClearConsole: vi.fn(async () => null),
  },
  payloadToState: vi.fn(() => ({})),
}))

import ConsolePanel from './ConsolePanel.vue'
import { store } from '../state'

// 模拟车辆登入帧:根层原始值 + 嵌套容器(与 Go 桥解码输出形态一致)
const decoded = {
  起始符: '##',
  命令单元: { 命令标识: 'x', 应答标志: 'y' },
}

// 未注册的 ant-design-vue 全局组件统一 stub,消除解析警告(测试断言不依赖它们)
const antStubs = [
  'a-button',
  'a-input',
  'a-checkbox',
  'a-modal',
  'a-select',
  'a-radio-group',
  'a-radio-button',
  'a-radio',
]

describe('ConsolePanel 详情 JsonTree', () => {
  beforeEach(() => {
    store.consoleFilter = 'all'
    store.consoleEvents = [
      {
        time: '2026-07-07T06:55:15Z',
        kind: 'tx',
        cmd: '0x01 VEHICLE_LOGIN',
        message: '车辆登入',
        hex: '##81',
        bytes: 30,
        decoded,
      },
    ]
  })

  it('点击行展开详情,JsonTree 渲染根层键与折叠摘要', async () => {
    const wrapper = mount(ConsolePanel, { global: { stubs: antStubs } })
    expect(wrapper.find('.row-detail').exists()).toBe(false)
    await wrapper.find('.console-row').trigger('click')
    expect(wrapper.find('.row-detail').exists()).toBe(true)
    // 根层键直接渲染
    const keyTexts = wrapper.findAll('.jt-key').map((k) => k.text())
    expect(keyTexts.some((t) => t.includes('起始符'))).toBe(true)
    // 嵌套容器默认折叠:{2 items}
    expect(wrapper.find('.jt-count').text()).toBe('{2 items}')
    expect(wrapper.text()).not.toContain('命令标识')
  })

  it('点击树内容器行不冒泡:详情块保留(行保持展开)且子树展开', async () => {
    const wrapper = mount(ConsolePanel, { global: { stubs: antStubs } })
    await wrapper.find('.console-row').trigger('click')
    await wrapper.find('.jt-container').trigger('click')
    // 树行点击未触发宿主行的折叠切换
    expect(wrapper.find('.row-detail').exists()).toBe(true)
    // 子键经树展开后可见
    expect(wrapper.text()).toContain('命令标识')
    expect(wrapper.text()).toContain('应答标志')
  })
})
