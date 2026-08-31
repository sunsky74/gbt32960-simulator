<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { parser as parserNs } from '../../../wailsjs/go/models'

export interface ByteRange {
  start: number
  end: number
}

const props = defineProps<{ normalizedHex: string; active: ByteRange | null }>()
const emit = defineEmits<{
  (e: 'hover', r: ByteRange | null): void
  (e: 'pin', r: ByteRange | null): void
}>()

// 单元格/地址列宽度常量,必须与下方 CSS 的 .byte-cell/.byte-offset 宽度一致
const CELL_W = 26
const OFFSET_W = 44
const MIN_PER_ROW = 6

// 每行字节数响应式:由容器实际宽度推导,容器变化(拖拽/缩放)时实时重排
const perRow = ref(16)
const rows = ref<Array<{ offset: number; bytes: string[] }>>([])

watch(
  [() => props.normalizedHex, perRow],
  ([hex, n]) => {
    const total = hex.length / 2
    const out: Array<{ offset: number; bytes: string[] }> = []
    for (let off = 0; off < total; off += n) {
      const row: string[] = []
      for (let i = off; i < Math.min(off + n, total); i++) {
        row.push(hex.slice(i * 2, i * 2 + 2).toUpperCase())
      }
      out.push({ offset: off, bytes: row })
    }
    rows.value = out
  },
  { immediate: true },
)

let ro: ResizeObserver | null = null

onMounted(() => {
  if (!gridEl.value) return
  ro = new ResizeObserver((entries) => {
    const w = (entries[0].target as HTMLElement).clientWidth
    perRow.value = Math.max(MIN_PER_ROW, Math.floor((w - OFFSET_W - 2) / CELL_W))
  })
  ro.observe(gridEl.value)
})

onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
})

const gridEl = ref<HTMLElement | null>(null)

// 高亮只切 DOM class,不进模板响应式(避免整片重渲染)
let hlNodes: Element[] = []
watch(
  () => props.active,
  (r) => {
    for (const el of hlNodes) el.classList.remove('byte-hl')
    hlNodes = []
    if (!r || !gridEl.value) return
    const cells = gridEl.value.querySelectorAll('[data-idx]')
    cells.forEach((el) => {
      const i = Number((el as HTMLElement).dataset.idx)
      if (i >= r.start && i < r.end) {
        el.classList.add('byte-hl')
        hlNodes.push(el)
      }
    })
  },
)

function onMove(e: MouseEvent) {
  const cell = (e.target as HTMLElement).closest('[data-idx]')
  if (!cell) return
  const i = Number((cell as HTMLElement).dataset.idx)
  emit('hover', { start: i, end: i + 1 })
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
    @mouseleave="emit('hover', null)"
    @click="onClick"
  >
    <div v-for="row in rows" :key="row.offset" class="byte-row">
      <span class="byte-offset">{{ row.offset.toString().padStart(4, '0') }}</span>
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
  font-family: SFMono-Regular, Consolas, Menlo, monospace;
  font-size: 12px;
  line-height: 2;
  overflow-y: auto;
  overflow-x: hidden;
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

.byte-cell {
  display: inline-block;
  width: 26px;
  text-align: center;
  color: var(--text-primary);
  border-radius: 3px;
  transition: background-color 0.06s ease, box-shadow 0.06s ease;
}

.byte-hl {
  background: var(--hl-bg);
  box-shadow: var(--hl-shadow);
}
</style>
