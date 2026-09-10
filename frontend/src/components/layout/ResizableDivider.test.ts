// ResizableDivider 挂载冒烟测试:仅验证无 Wails 依赖下的静态渲染
import {describe, expect, it} from 'vitest'
import {mount} from '@vue/test-utils'
import ResizableDivider from './ResizableDivider.vue'

describe('ResizableDivider', () => {
  it('渲染分隔条并携带无障碍语义', () => {
    const wrapper = mount(ResizableDivider, {props: {minPx: 100}})
    const el = wrapper.find('.resizable-divider')
    expect(el.exists()).toBe(true)
    expect(el.attributes('role')).toBe('separator')
    expect(el.attributes('aria-orientation')).toBe('horizontal')
    expect(wrapper.find('.divider-grip').exists()).toBe(true)
    expect(wrapper.find('.resizable-divider').classes()).not.toContain('dragging')
  })
})
