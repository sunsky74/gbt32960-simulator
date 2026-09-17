// 0x80 参数应答取值校验/编码纯函数测试(规则见 docs/standard 附录 B 参数定义表)
import { describe, expect, it } from 'vitest'
import { engine } from '../../wailsjs/go/models'
import { answerHint, answerInputKind, defaultAnswerValue, encodeAnswerValue } from './paramAnswer'

function spec(partial: Partial<engine.ParamSpec>): engine.ParamSpec {
  return new engine.ParamSpec({ id: 0x01, name: '测试参数', type: 'u8', length: 1, ...partial })
}

describe('encodeAnswerValue', () => {
  it('u8 范围内 OK,固定 2 位 hex', () => {
    const s = spec({ type: 'u8', length: 1, rawMin: 0, rawMax: 250 })
    expect(encodeAnswerValue(s, '100')).toBe('64')
    expect(encodeAnswerValue(s, '0')).toBe('00')
  })

  it('u8 标记值 0xFE/0xFF OK(不受 rawMax 约束)', () => {
    const s = spec({ type: 'u8', length: 1, rawMin: 0, rawMax: 250 })
    expect(encodeAnswerValue(s, '254')).toBe('fe')
    expect(encodeAnswerValue(s, '255')).toBe('ff')
  })

  it('u8 越界/非法输入拒绝', () => {
    const s = spec({ type: 'u8', length: 1, rawMin: 0, rawMax: 250 })
    expect(() => encodeAnswerValue(s, '251')).toThrow()
    expect(() => encodeAnswerValue(s, '256')).toThrow()
    expect(() => encodeAnswerValue(s, '-1')).toThrow()
    expect(() => encodeAnswerValue(s, '')).toThrow()
  })

  it('u16 范围内 OK,固定 4 位 hex', () => {
    const s = spec({ type: 'u16', length: 2, rawMin: 0, rawMax: 60000 })
    expect(encodeAnswerValue(s, '1000')).toBe('03e8')
    expect(encodeAnswerValue(s, '7')).toBe('0007')
  })

  it('u16 标记值 0xFFFE/0xFFFF OK', () => {
    const s = spec({ type: 'u16', length: 2, rawMin: 0, rawMax: 60000 })
    expect(encodeAnswerValue(s, '65534')).toBe('fffe')
    expect(encodeAnswerValue(s, '65535')).toBe('ffff')
  })

  it('u16 越界拒绝', () => {
    const s = spec({ type: 'u16', length: 2, rawMin: 0, rawMax: 60000 })
    expect(() => encodeAnswerValue(s, '60001')).toThrow()
    expect(() => encodeAnswerValue(s, '65536')).toThrow()
  })

  it('定长 string 长度不符拒绝,恰好 length 字节则通过', () => {
    const s = spec({ type: 'string', length: 5 })
    expect(() => encodeAnswerValue(s, '00ff')).toThrow()
    expect(() => encodeAnswerValue(s, 'aabbccddeeff')).toThrow()
    expect(encodeAnswerValue(s, 'aabbccdd01')).toBe('aabbccdd01')
  })

  it('变长 bytes(length=0)接受任意整字节 hex 并归一化', () => {
    const s = spec({ type: 'bytes', length: 0 })
    expect(encodeAnswerValue(s, 'aabb')).toBe('aabb')
    expect(encodeAnswerValue(s, 'AA BB')).toBe('aabb')
    expect(() => encodeAnswerValue(s, 'abc')).toThrow()
  })

  it('options 枚举值 OK,编码为 1 字节', () => {
    const s = spec({
      type: 'u8',
      length: 1,
      options: [
        { value: 1, label: '停止' },
        { value: 2, label: '上传' },
      ],
    })
    expect(encodeAnswerValue(s, 2)).toBe('02')
    expect(encodeAnswerValue(s, '1')).toBe('01')
    expect(() => encodeAnswerValue(s, 3)).toThrow()
  })

  it('未知 id(无 spec)自由 hex', () => {
    expect(encodeAnswerValue(undefined, 'a1b2')).toBe('a1b2')
    expect(() => encodeAnswerValue(undefined, 'abc')).toThrow()
  })
})

describe('answerInputKind / defaultAnswerValue / answerHint', () => {
  it('输入形态:枚举→select,u8/u16→number,其余/未知→hex', () => {
    expect(answerInputKind(spec({ type: 'u8', options: [{ value: 1, label: 'x' }] }))).toBe('select')
    expect(answerInputKind(spec({ type: 'u8' }))).toBe('number')
    expect(answerInputKind(spec({ type: 'u16' }))).toBe('number')
    expect(answerInputKind(spec({ type: 'string', length: 5 }))).toBe('hex')
    expect(answerInputKind(spec({ type: 'bytes', length: 0 }))).toBe('hex')
    expect(answerInputKind(undefined)).toBe('hex')
  })

  it('初始值:枚举取第一项,数值取下限,hex 留空', () => {
    expect(defaultAnswerValue(spec({ options: [{ value: 2, label: 'x' }] }))).toBe('2')
    expect(defaultAnswerValue(spec({ type: 'u16', rawMin: 10 }))).toBe('10')
    expect(defaultAnswerValue(undefined)).toBe('')
  })

  it('提示:变长域名补充长度说明,未知 id 给自由 hex 提示', () => {
    expect(answerHint(spec({ type: 'bytes', length: 0 }))).toContain('0x04')
    expect(answerHint(undefined)).toContain('hex')
  })
})
