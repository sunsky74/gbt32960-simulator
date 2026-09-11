// JsonTree 组件测试:可折叠 JSON 树,用于展示 GB/T 32960 解析帧
// 约定:根层条目直接展开渲染;嵌套容器默认折叠;行点击不向宿主冒泡
import {describe, expect, it, vi} from 'vitest'
import {mount} from '@vue/test-utils'
import {defineComponent, h} from 'vue'
import JsonTree from './JsonTree.vue'

// 模拟解析帧数据:根对象含原始值条目与嵌套容器条目
const frame = {
  起始符: '##',
  命令单元: {
    命令标识: '0x02 实时信息上报',
    应答标志: '0xFE 命令',
  },
}

describe('JsonTree', () => {
  it('根层直接渲染,嵌套对象默认折叠:显示 {2 items} 且子键不在 DOM', () => {
    const wrapper = mount(JsonTree, {props: {value: frame}})
    // 根层原始值条目可见
    expect(wrapper.text()).toContain('起始符')
    expect(wrapper.text()).toContain('"##"')
    // 折叠摘要与收起指示符
    expect(wrapper.find('.jt-count').text()).toBe('{2 items}')
    expect(wrapper.find('.jt-toggle').text()).toBe('▸')
    // 嵌套子键不可见
    expect(wrapper.text()).not.toContain('命令标识')
    expect(wrapper.text()).not.toContain('应答标志')
    expect(wrapper.find('.jt-children').exists()).toBe(false)
  })

  it('点击容器行展开子键,再次点击折叠', async () => {
    const wrapper = mount(JsonTree, {props: {value: frame}})
    await wrapper.find('.jt-container').trigger('click')
    expect(wrapper.find('.jt-children').exists()).toBe(true)
    expect(wrapper.text()).toContain('命令标识')
    expect(wrapper.text()).toContain('应答标志')
    expect(wrapper.find('.jt-toggle').text()).toBe('▾')
    await wrapper.find('.jt-container').trigger('click')
    expect(wrapper.find('.jt-children').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('命令标识')
  })

  it('数组默认折叠为 [2 items],展开后为下标行', async () => {
    const wrapper = mount(JsonTree, {props: {value: {数据单元: ['a', 'b']}}})
    expect(wrapper.find('.jt-count').text()).toBe('[2 items]')
    await wrapper.find('.jt-container').trigger('click')
    expect(wrapper.text()).toContain('[0]')
    expect(wrapper.text()).toContain('[1]')
    expect(wrapper.text()).toContain('"a"')
    expect(wrapper.text()).toContain('"b"')
  })

  it('空对象渲染 {},空数组渲染 []', () => {
    const objWrapper = mount(JsonTree, {props: {value: {扩展: {}}}})
    expect(objWrapper.find('.jt-count').text()).toBe('{}')
    const arrWrapper = mount(JsonTree, {props: {value: {扩展: []}}})
    expect(arrWrapper.find('.jt-count').text()).toBe('[]')
  })

  it('原始值渲染:字符串带双引号,数字原样', () => {
    const wrapper = mount(JsonTree, {props: {value: {名称: 'abc', 数量: 318}}})
    expect(wrapper.text()).toContain('"abc"')
    expect(wrapper.text()).toContain('318')
    expect(wrapper.text()).not.toContain('"318"')
  })

  it('点击树内行不冒泡到外层宿主容器', async () => {
    const onRoot = vi.fn()
    const Host = defineComponent({
      setup: () => () =>
        h('div', {onClick: onRoot}, [h(JsonTree, {value: {嵌套: {a: 1}}})]),
    })
    const wrapper = mount(Host)
    await wrapper.find('.jt-container').trigger('click')
    expect(onRoot).not.toHaveBeenCalled()
  })

  it('深层嵌套(3+ 层)不自动展开根以下任何层级', async () => {
    const deep = {level1: {level2: {level3: {level4: 'x'}}}}
    const wrapper = mount(JsonTree, {props: {value: deep}})
    // 初始仅可见根层 level1,level2/3/4 均不在 DOM
    expect(wrapper.text()).toContain('level1')
    expect(wrapper.text()).not.toContain('level2')
    expect(wrapper.find('.jt-children').exists()).toBe(false)
    // 展开第一层后仅露出 level2,更深层仍折叠
    await wrapper.find('.jt-container').trigger('click')
    expect(wrapper.text()).toContain('level2')
    expect(wrapper.text()).not.toContain('level3')
  })
})
