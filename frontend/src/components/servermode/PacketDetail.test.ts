// PacketDetail 右栏视图切换测试:字段表(默认)/ JSON 解析树
// Mock 约定:ParserService.ParsePacket 返回含 tree 的解析结果(与 Go 桥输出形态一致);
// ByteGridView/FieldTableView 与未注册的 ant-design-vue 组件统一 stub,聚焦视图切换逻辑
import {describe, expect, it, vi} from 'vitest'
import {flushPromises, mount} from '@vue/test-utils'

vi.mock('../../../wailsjs/go/bridge/ParserService', () => ({
  ParsePacket: vi.fn(async () => ({
    totalBytes: 30,
    fields: [],
    tree: {
      起始符: '##',
      命令单元: {命令标识: 1, 应答标志: 0},
      数据单元长度: 12,
    },
  })),
}))

import PacketDetail from './PacketDetail.vue'

// 模拟 normal 帧:挂载时传 null(与真实点击流一致),再 setProps 触发 watch 解析
const frame = {
  id: 1,
  time: '2026-07-07T06:55:15Z',
  vin: 'LSV123',
  cmd: '0x01',
  hex: '23230101000000',
  summary: '车辆登入',
  kind: 'normal',
  dir: 'rx',
} as const

const stubs = {
  'a-select': true,
  'a-button': true,
  ByteGridView: true,
  FieldTableView: true,
}

async function mountWithFrame(f: typeof frame | null) {
  const wrapper = mount(PacketDetail, {
    props: {frame: f, parserPacks: []},
    global: {stubs},
  })
  if (f) {
    await wrapper.setProps({frame: {...f}})
    await flushPromises()
  }
  return wrapper
}

describe('PacketDetail 视图切换', () => {
  it('默认字段表视图:hint 显示字段数与联动说明,渲染字段表', async () => {
    const wrapper = await mountWithFrame(frame)
    const hint = wrapper.find('.detail-right .col-hint')
    expect(hint.text()).toContain('0 个字段')
    expect(hint.text()).toContain('悬停/点击联动字节')
    expect(wrapper.find('.detail-right field-table-view-stub').exists()).toBe(true)
    // JSON 树尚未渲染
    expect(wrapper.find('.jt').exists()).toBe(false)
  })

  it('点击 JSON 切换:渲染解析树根层键与折叠摘要,hint 同步切换', async () => {
    const wrapper = await mountWithFrame(frame)
    const toggles = wrapper.findAll('.view-toggle')
    expect(toggles).toHaveLength(2)
    await toggles[1].trigger('click')
    // 根层键直接渲染,嵌套容器默认折叠
    const keyTexts = wrapper.findAll('.jt-key').map((k) => k.text())
    expect(keyTexts.some((t) => t.includes('起始符'))).toBe(true)
    expect(wrapper.find('.jt-count').text()).toBe('{2 items}')
    expect(wrapper.find('.detail-right .col-hint').text()).toContain('JSON 解析树')
  })

  it('切换帧不重置视图:JSON 视图在 frame 变化后保持', async () => {
    const wrapper = await mountWithFrame(frame)
    await wrapper.findAll('.view-toggle')[1].trigger('click')
    // 模拟流中切换到另一帧:重新解析,视图仍是 JSON
    await wrapper.setProps({frame: {...frame, hex: '23230200'}})
    await flushPromises()
    expect(wrapper.find('.jt').exists()).toBe(true)
    expect(wrapper.find('.detail-right .col-hint').text()).toContain('JSON 解析树')
  })
})
