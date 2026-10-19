// 回归:实时数据页每组的启停开关,点击只切换 enabled,不冒泡到 collapse header 把整组收起。
// 背景:antd Switch 未声明 emits,`@click` 被当作 props.onClick 吞掉,组件级 @click.stop 静默失效;
// 修法是原生 <span> 承载 @click.stop。此用例在换回组件级 @click.stop 时应当失败。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
}))

vi.mock('../../wailsjs/go/bridge/MessageService', () => ({
  Send: vi.fn(async () => null),
  SendReissue: vi.fn(async () => null),
  Preview: vi.fn(async () => ''),
  SetAutoReport: vi.fn(async () => null),
  SaveGroups: vi.fn(async () => null),
}))

vi.mock('../../wailsjs/go/bridge/ExtService', () => ({ ListPacks: vi.fn(async () => []) }))

import RealTimePanel from './RealTimePanel.vue'
import type { GroupSchema } from '../api/backend'
import { store } from '../state'

function vehicleGroup(): GroupSchema {
  return {
    key: 'vehicle',
    title: '整车数据',
    enabled: true,
    multiple: false,
    maxRows: 0,
    fields: [],
    source: '',
  } as unknown as GroupSchema
}

describe('RealTimePanel 分组启停开关', () => {
  beforeEach(() => {
    store.connState = 'idle'
    store.schema = [vehicleGroup()]
    store.groups = { vehicle: { enabled: true, rows: [{}] } }
  })

  it('默认收起;点开关只改 enabled,展开态不变', async () => {
    const wrapper = mount(RealTimePanel, { global: { plugins: [Antd] } })
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.ant-collapse-item').classes()).not.toContain('ant-collapse-item-active')

    // 先手动展开该组,再点开关:开关不应把展开态改回去
    await wrapper.find('.ant-collapse-header').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.ant-collapse-item').classes()).toContain('ant-collapse-item-active')

    await wrapper.find('.ant-collapse-extra .ant-switch').trigger('click')
    await wrapper.vm.$nextTick()

    expect(store.groups.vehicle.enabled).toBe(false)
    expect(wrapper.find('.ant-collapse-item').classes()).toContain('ant-collapse-item-active')
  })
})
