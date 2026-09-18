<script setup lang="ts">
import { computed } from 'vue'
import type { parser as parserNs } from '../../../wailsjs/go/models'

type ParsedField = parserNs.Field

// 悬停信息卡片数据:source 区分字节卡与字段卡(由字节视图/字段表联动写入)
export interface HoverCardInfo {
  x: number
  y: number
  byte: number
  field: ParsedField | null
  source: 'byte' | 'field'
  issue?: string
}

const props = defineProps<{
  card: HoverCardInfo | null
  parsedHex: string
}>()

const cardStyle = computed<{ left?: string; top?: string }>(() => {
  const c = props.card
  if (!c) return {}
  const W = 250
  const x = Math.max(8, Math.min(c.x + 16, window.innerWidth - W - 16))
  const y = c.y + 24 > window.innerHeight - 170 ? Math.max(8, c.y - 170) : c.y + 24
  return { left: x + 'px', top: y + 'px' }
})

// 字段原始 HEX 可能很长(如大段数据单元),卡片内截断展示,完整值看表格
function truncHex(s: string): string {
  if (!s) return '-'
  return s.length > 48 ? s.slice(0, 48) + '…' : s
}

// 字节卡片:当前悬停字节的 HEX 与十进制
const hoverByteHex = computed(() => {
  const c = props.card
  if (!c) return ''
  return props.parsedHex.slice(c.byte * 2, c.byte * 2 + 2).toUpperCase()
})

const hoverByteDec = computed(() => {
  const h = hoverByteHex.value
  return h ? parseInt(h, 16) : ''
})
</script>

<template>
  <!-- 悬停信息卡片:字段来源显示字段卡,字节来源显示字节卡(Offset/HEX/Decimal/所属字段) -->
  <div v-if="card" class="byte-card" :style="cardStyle">
    <template v-if="card.source === 'field' && card.field">
      <div class="bc-name">{{ card.field.name }}</div>
      <div class="bc-kv">
        <span class="bc-k">Offset</span><span class="bc-v mono">{{ card.field.offset }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">Length</span><span class="bc-v mono">{{ card.field.length }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">类型</span><span class="bc-v mono">{{ card.field.type }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">HEX</span><span class="bc-v mono bc-hex">{{ truncHex(card.field.rawHex) }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">原始值</span><span class="bc-v mono">{{ card.field.rawValue }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">解析值</span>
        <span class="bc-v"
          >{{ card.field.offsetVal }}<span v-if="card.field.unit" class="bc-u"> {{ card.field.unit }}</span></span
        >
      </div>
    </template>
    <template v-else>
      <div class="bc-name">字节 #{{ card.byte }}</div>
      <div class="bc-kv">
        <span class="bc-k">Offset</span><span class="bc-v mono">{{ card.byte }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">HEX</span><span class="bc-v mono">{{ hoverByteHex }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">Decimal</span><span class="bc-v mono">{{ hoverByteDec }}</span>
      </div>
      <div class="bc-kv">
        <span class="bc-k">所属字段</span>
        <span class="bc-v">{{ card.field?.name || '-' }}</span>
      </div>
      <div v-if="card.issue" class="bc-issue">{{ card.issue }}</div>
    </template>
  </div>
</template>
