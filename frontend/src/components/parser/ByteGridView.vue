<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'

export interface ByteRange {
  start: number
  end: number
}

export interface ByteHover {
  range: ByteRange
  x: number
  y: number
}

const props = defineProps<{
  normalizedHex: string
  active: ByteRange | null
  activeSource: 'byte' | 'field' | null
  activeByte: number | null
}>()

const emit = defineEmits<{
  (e: 'hover', h: ByteHover | null): void
  (e: 'pin', r: ByteRange | null): void
}>()

// 固定每行 8 字节;offset 标号 0000/0008/0010...
const PER_ROW = 8

const rows = computed(() => {
  const hex = props.normalizedHex
  const total = hex.length / 2
  const out: Array<{ offset: number; bytes: string[] }> = []
  for (let off = 0; off < total; off += PER_ROW) {
    const row: string[] = []
    for (let i = off; i < Math.min(off + PER_ROW, total); i++) {
      row.push(hex.slice(i * 2, i * 2 + 2).toUpperCase())
    }
    out.push({ offset: off, bytes: row })
  }
  return out
})

const gridEl = ref<HTMLElement | null>(null)
// 网格内光标所在的字节(悬停期间持续有效;离开网格后回到 activeByte/无)
const curIdx = ref<number | null>(null)

// 单元格元素缓存(按 data-idx 顺序与绝对字节索引一致),rows 重建后刷新
let cellEls: HTMLElement[] = []
watch(
  rows,
  async () => {
    await nextTick()
    cellEls = gridEl.value
      ? Array.from(gridEl.value.querySelectorAll<HTMLElement>('[data-idx]'))
      : []
    applyHl()
  },
  { immediate: true, flush: 'post' },
)

// 三态高亮:增量维护 —— 仅清除上次高亮的元素,新范围按缓存索引命中,避免全量重扫
let prevHl: Element[] = []
function applyHl() {
  for (const el of prevHl) {
    el.classList.remove('byte-hl', 'byte-cur', 'byte-field-hl', 'byte-dim')
  }
  prevHl = []
  const active = props.active
  if (!active || cellEls.length === 0) return
  const source = props.activeSource
  // 光标在网格内优先;否则(钉住字节后光标离开)用 activeByte
  const cur = curIdx.value ?? (source === 'byte' ? props.activeByte : null)
  const start = Math.max(0, active.start)
  const end = Math.min(cellEls.length, active.end)
  for (let i = start; i < end; i++) {
    const el = cellEls[i]
    if (el.dataset.idx !== String(i)) continue // 防御:缓存与索引不一致时跳过
    if (i === cur) el.classList.add('byte-cur')
    else if (source === 'field') el.classList.add('byte-field-hl')
    else el.classList.add('byte-hl')
    prevHl.push(el)
  }
  if (source === 'field') {
    // 字段行悬停来源:区间外字节降低视觉权重
    for (let i = 0; i < cellEls.length; i++) {
      if (i >= start && i < end) continue
      cellEls[i].classList.add('byte-dim')
      prevHl.push(cellEls[i])
    }
  }
}

watch([() => props.active, () => props.activeSource, () => props.activeByte, curIdx], applyHl)

function onMove(e: MouseEvent) {
  const cell = (e.target as HTMLElement).closest('[data-idx]')
  if (!cell) return
  const i = Number((cell as HTMLElement).dataset.idx)
  curIdx.value = i
  emit('hover', { range: { start: i, end: i + 1 }, x: e.clientX, y: e.clientY })
}

function onLeave() {
  curIdx.value = null
  emit('hover', null)
}

function onClick(e: MouseEvent) {
  const cell = (e.target as HTMLElement).closest('[data-idx]')
  if (!cell) {
    emit('pin', null)
    return
  }
  const i = Number((cell as HTMLElement).dataset.idx)
  emit('pin', { start: i, end: i + 1 })
}
</script>

<template>
  <div
    ref="gridEl"
    class="byte-grid"
    @mouseover="onMove"
    @mouseleave="onLeave"
    @click="onClick"
  >
    <div v-for="row in rows" :key="row.offset" class="byte-row">
      <span class="byte-offset">{{ row.offset.toString(16).padStart(4, '0') }}</span>
      <span
        v-for="(b, bi) in row.bytes"
        :key="bi"
        class="byte-cell"
        :data-idx="row.offset + bi"
      >{{ b }}</span>
    </div>
  </div>
</template>

<style scoped>
.byte-grid {
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 2.1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 6px 8px;
  cursor: default;
}

.byte-row {
  white-space: nowrap;
}

.byte-offset {
  display: inline-block;
  width: 44px;
  color: var(--text-tertiary);
}

/* 22px 内容 + 左右各 2px margin = 26px 槽位,scale(1.15) 放大后不挤压相邻字节 */
.byte-cell {
  display: inline-block;
  width: 22px;
  margin: 1px 2px;
  text-align: center;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 2px;
  transform-origin: center;
  transition:
    transform 0.12s ease,
    background-color 0.08s ease,
    box-shadow 0.08s ease,
    opacity 0.12s ease;
}

/* 同字段区间内字节:弱高亮 */
.byte-hl {
  background: var(--hl-bg);
}

/* 当前字节:强高亮(边框 + 浅背景 + 放大) */
.byte-cur {
  background: var(--hl-bg);
  box-shadow: var(--hl-shadow);
  transform: scale(1.15);
  position: relative;
  z-index: 2;
}

/* 字段行悬停来源:整个字段区间明显突出、轻微放大 */
.byte-field-hl {
  background: var(--hl-bg);
  transform: scale(1.04);
}

/* 字段行悬停来源:区间外字节降低视觉权重 */
.byte-dim {
  opacity: 0.4;
}
</style>
