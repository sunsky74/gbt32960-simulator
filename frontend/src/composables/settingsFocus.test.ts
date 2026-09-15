import { beforeEach, describe, expect, it } from 'vitest'
import { openSettingsCategory, settingsFocusCategory } from './settingsFocus'

describe('settingsFocus', () => {
  beforeEach(() => {
    settingsFocusCategory.value = null
  })

  it('openSettingsCategory 写入目标分类', () => {
    openSettingsCategory('about')
    expect(settingsFocusCategory.value).toBe('about')
  })
})
