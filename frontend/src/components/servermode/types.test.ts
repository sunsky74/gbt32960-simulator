// 服务端格式化纯函数冒烟测试(types.ts 无外部依赖,直接单测)
import {describe, expect, it} from 'vitest'
import {cmdNameOf, fmtClock, fmtDuration, fmtMs} from './types'

describe('cmdNameOf', () => {
  it('识别已知命令字(大小写不敏感)', () => {
    expect(cmdNameOf('0x01')).toBe('登入')
    expect(cmdNameOf('0X07')).toBe('心跳')
  })

  it('未知/空命令字', () => {
    expect(cmdNameOf('0x7f')).toBe('未知命令')
    expect(cmdNameOf('')).toBe('')
  })
})

describe('fmtMs', () => {
  it('输出 HH:mm:ss.SSS 毫秒精度', () => {
    // 固定本地时刻 12:34:56.789
    const iso = new Date(2026, 8, 10, 12, 34, 56, 789).toISOString()
    expect(fmtMs(iso)).toBe('12:34:56.789')
  })

  it('空串与非法输入回退', () => {
    expect(fmtMs('')).toBe('')
    expect(fmtMs('not-a-date')).toBe('not-a-date')
  })
})

describe('fmtClock', () => {
  it('输出 HH:mm:ss,空串返回占位符', () => {
    const iso = new Date(2026, 8, 10, 8, 5, 3).toISOString()
    expect(fmtClock(iso)).toBe('08:05:03')
    expect(fmtClock('')).toBe('-')
  })
})

describe('fmtDuration', () => {
  it('不足 1 小时为 mm:ss', () => {
    const from = 0
    expect(fmtDuration(from, 65_000)).toBe('01:05')
  })

  it('满 1 小时进位为 h:mm:ss,负差值钳制为 0', () => {
    expect(fmtDuration(0, 3_600_000)).toBe('1:00:00')
    expect(fmtDuration(0, 3_723_000)).toBe('1:02:03')
    expect(fmtDuration(1_000, 0)).toBe('00:00')
  })
})
