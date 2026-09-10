// clampNum 纯函数冒烟测试(非法回落默认 / 越界钳回区间;localStorage 侧效应由 jsdom 提供)
import {describe, expect, it} from 'vitest'
import {clampNum} from './useAppSettings'

describe('clampNum', () => {
  it('区间内的值原样返回', () => {
    expect(clampNum(200, 100, 2000, 200)).toBe(200)
    expect(clampNum(100, 100, 2000, 200)).toBe(100)
    expect(clampNum(2000, 100, 2000, 200)).toBe(2000)
  })

  it('越界值钳回区间边界', () => {
    expect(clampNum(50, 100, 2000, 200)).toBe(100)
    expect(clampNum(99999, 100, 2000, 200)).toBe(2000)
  })

  it('非法输入回落默认值', () => {
    expect(clampNum(undefined, 100, 2000, 200)).toBe(200)
    expect(clampNum(null, 100, 2000, 200)).toBe(200)
    expect(clampNum('abc', 1000, 50000, 10000)).toBe(10000)
    expect(clampNum(NaN, 1000, 50000, 10000)).toBe(10000)
  })
})
