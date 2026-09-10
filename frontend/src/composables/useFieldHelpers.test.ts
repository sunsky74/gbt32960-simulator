// useFieldHelpers 纯函数冒烟测试(FieldSchema 经 import type 引入,运行时无 Wails 依赖)
import {describe, expect, it} from 'vitest'
import type {FieldSchema} from '../api/backend'
import {defaultFor, hexOf, numOf, setHex, setNum} from './useFieldHelpers'

// 测试夹具仅需结构化字段;生成类的实例方法(convertValues)与测试无关,整体收窄断言
function field(partial: Partial<FieldSchema>): FieldSchema {
  return {key: 'f', label: '字段', kind: 'int', ...partial} as FieldSchema
}

describe('numOf / setNum', () => {
  it('setNum → numOf 往返,字符串与非法输入归零', () => {
    const row: Record<string, unknown> = {}
    setNum(row, 'speed', 80)
    expect(numOf(row, 'speed')).toBe(80)

    setNum(row, 'speed', '128')
    expect(numOf(row, 'speed')).toBe(128)

    setNum(row, 'speed', null)
    expect(numOf(row, 'speed')).toBe(0)
    expect(numOf(row, 'missing')).toBe(0)
  })
})

describe('hexOf / setHex', () => {
  it('合法 hex 小写化、去空白后往返', () => {
    const row: Record<string, unknown> = {}
    const f = field({kind: 'bytes', length: 4})
    setHex(row, 'vin', f, ' AB cd ')
    expect(row['vin']).toBe('abcd')
    expect(hexOf(row, 'vin')).toBe('abcd')
  })

  it('空串清空、非法字符与超长被拒', () => {
    const row: Record<string, unknown> = {vin: 'abcd'}
    const f = field({kind: 'bytes', length: 2})

    setHex(row, 'vin', f, 'zz99')
    expect(row['vin']).toBe('abcd') // 非法输入不写入

    setHex(row, 'vin', f, 'aabbcc') // 3 字节 > length 2
    expect(row['vin']).toBe('abcd')

    setHex(row, 'vin', f, '  ')
    expect(row['vin']).toBe('')
  })
})

describe('defaultFor', () => {
  it('各字段类型给出合理默认值', () => {
    expect(defaultFor(field({kind: 'enum', enum: [{value: 2, label: 'B'}]}))).toBe(2)
    expect(defaultFor(field({kind: 'bool'}))).toBe(false)
    expect(defaultFor(field({kind: 'int', min: 5}))).toBe(5)
    expect(defaultFor(field({kind: 'int'}))).toBe(0)
    expect(defaultFor(field({kind: 'bytes', length: 3}))).toBe('000000')
    expect(defaultFor(field({kind: 'array_float'}))).toEqual([])
    expect(defaultFor(field({kind: 'bitgroup', bits: [{index: 0, label: 'a'}, {index: 1, label: 'b'}]})))
      .toEqual({bit0: false, bit1: false})
  })
})
