import { beforeEach, describe, expect, it, vi } from 'vitest'

const { eventsOnMock, runtimeOff } = vi.hoisted(() => ({
  eventsOnMock: vi.fn(),
  runtimeOff: vi.fn(),
}))

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: eventsOnMock,
}))

import { onConsoleEvents, onWailsEvent } from './events'

describe('onWailsEvent', () => {
  beforeEach(() => {
    eventsOnMock.mockReset()
    runtimeOff.mockReset()
    eventsOnMock.mockImplementation(() => runtimeOff)
  })

  it('以事件名注册回调,并把 Wails 回调的首参作为载荷递给 handler', () => {
    const handler = vi.fn()
    onWailsEvent('server:status', handler)

    expect(eventsOnMock).toHaveBeenCalledOnce()
    const [name, cb] = eventsOnMock.mock.calls[0] as [string, (...args: unknown[]) => void]
    expect(name).toBe('server:status')

    const payload = { running: true, listenAddr: '127.0.0.1:32960' }
    cb(payload, '多余参数应被丢弃')
    expect(handler).toHaveBeenCalledExactlyOnceWith(payload)
  })

  it('注销函数幂等:重复调用只注销底层一次', () => {
    const off = onWailsEvent('server:warn', () => {})
    off()
    off()
    off()
    expect(runtimeOff).toHaveBeenCalledOnce()
  })

  it('onConsoleEvents 映射到 console:events,批量数组原样透传', () => {
    const handler = vi.fn()
    onConsoleEvents(handler)

    const [, cb] = eventsOnMock.mock.calls[0] as [string, (...args: unknown[]) => void]
    const batch = [{ time: 't', kind: 'tx' as const, hex: 'AA' }]
    cb(batch)
    expect(handler).toHaveBeenCalledExactlyOnceWith(batch)
  })
})
