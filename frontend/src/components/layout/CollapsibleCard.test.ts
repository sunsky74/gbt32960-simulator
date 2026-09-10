// CollapsibleCard 挂载冒烟测试:无 Wails 依赖,验证渲染与折叠交互
import {describe, expect, it} from 'vitest'
import {mount} from '@vue/test-utils'
import CollapsibleCard from './CollapsibleCard.vue'

describe('CollapsibleCard', () => {
  it('默认展开渲染标题与插槽内容', () => {
    const wrapper = mount(CollapsibleCard, {
      props: {title: '上报配置'},
      slots: {default: '<p>内容区</p>'},
    })
    expect(wrapper.find('.cc-title').text()).toBe('上报配置')
    expect(wrapper.find('.cc-content').html()).toContain('内容区')
    expect(wrapper.find('.cc-body').classes()).not.toContain('collapsed')
  })

  it('点击头部切换折叠,支持 defaultOpen=false 初始收起', async () => {
    const wrapper = mount(CollapsibleCard, {props: {title: '会话', defaultOpen: false}})
    expect(wrapper.find('.cc-body').classes()).toContain('collapsed')

    await wrapper.find('.cc-head').trigger('click')
    expect(wrapper.find('.cc-body').classes()).not.toContain('collapsed')
  })
})
