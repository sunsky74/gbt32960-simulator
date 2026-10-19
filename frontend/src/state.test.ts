// state 重载动画的状态位:重载期间 schemaLoading=true(实时数据面板据此播放动画),
// 且即使后端 IPC 秒回,也至少保持 260ms 可见,避免"闪一下看不出"。
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
}))

vi.mock('../wailsjs/go/bridge/MessageService', () => ({
  GetSchema: vi.fn(async () => []),
  GetGroups: vi.fn(async () => ({ groups: [] })),
  DefaultGroups: vi.fn(async () => ({ groups: [] })),
}))

import * as MessageService from '../wailsjs/go/bridge/MessageService'
import { reloadSchemaForVersion, reloadSchemaPreservingGroups, store } from './state'

describe('报文配置重载动画状态', () => {
  beforeEach(() => {
    store.schemaLoading = false
    vi.mocked(MessageService.GetSchema).mockResolvedValue([])
  })

  it('重载期间置位 schemaLoading,并至少可见 260ms', async () => {
    const startedAt = Date.now()
    const pending = reloadSchemaPreservingGroups('2016')
    expect(store.schemaLoading).toBe(true)

    await pending

    expect(Date.now() - startedAt).toBeGreaterThanOrEqual(250)
    expect(store.schemaLoading).toBe(false)
  })

  it('后端返回失败时也收起加载态(不卡死遮罩)', async () => {
    vi.mocked(MessageService.GetSchema).mockRejectedValueOnce(new Error('boom'))

    await expect(reloadSchemaForVersion('2016')).rejects.toThrow('boom')
    expect(store.schemaLoading).toBe(false)
  })
})
