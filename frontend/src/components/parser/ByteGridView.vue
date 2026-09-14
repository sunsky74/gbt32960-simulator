<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export interface ByteRange {
  start: number
  end: number
}

export interface ByteHover {
  range: ByteRange
  x: number
  y: number
  issue?: string
}

// 与协议定义不符的异常字节区间(来源 parser.Result.Issues)
export interface ByteIssueRange {
  start: number
  end: number
  note: string
}

const props = defineProps<{
  normalizedHex: string
  active: ByteRange | null
  activeSource: 'byte' | 'field' | null
  activeByte: number | null
  issues?: ByteIssueRange[]
}>()

const emit = defineEmits<{
  (e: 'hover', h: ByteHover | null): void
  (e: 'pin', r: ByteRange | null): void
}>()

const gridEl = ref<HTMLElement | null>(null)

// 每行字节数随面板宽度自适应:窄面板 8 字节/行,宽面板 16 字节/行(紧凑 IDE 风格,消除横向空白)
const perRow = ref(8)

const rows = computed(() => {
  const hex = props.normalizedHex
  const total = hex.length / 2
  const n = perRow.value
  const out: Array<{ offset: number; bytes: string[] }> = []
  for (let off = 0; off < total; off += n) {
    const row: string[] = []
    for (let i = off; i < Math.min(off + n, total); i++) {
      row.push(hex.slice(i * 2, i * 2 + 2).toUpperCase())
    }
    out.push({ offset: off, bytes: row })
  }
  return out
})

// 16 字节/行所需内容宽度:offset 40 + 16×24 单元格 + 17×4 间距 + 6 分组间隔 + 20 内边距 ≈ 516px
function fitPerRow() {
  perRow.value = (gridEl.value?.clientWidth ?? 0) >= 516 ? 16 : 8
}

let ro: ResizeObserver | null = null

onMounted(() => {
  fitPerRow()
  ro = new ResizeObserver(fitPerRow)
  if (gridEl.value) ro.observe(gridEl.value)
})

onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
})

// 网格内光标所在的字节(悬停期间持续有效;离开网格后回到 activeByte/无)
const curIdx = ref<number | null>(null)

// 单元格元素缓存(按 data-idx 顺序与绝对字节索引一致),rows 重建后刷新
let cellEls: HTMLElement[] = []
watch(
  rows,
  async () => {
    await nextTick()
    cellEls = gridEl.value ? Array.from(gridEl.value.querySelectorAll<HTMLElement>('[data-idx]')) : []
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

// 异常区间索引表:idx → issue 说明(hover 卡片透传)
const issueMap = computed(() => {
  const m = new Map<number, string>()
  for (const iss of props.issues ?? []) {
    for (let i = Math.max(0, iss.start); i < iss.end; i++) m.set(i, iss.note)
  }
  return m
})

function issueOf(idx: number): string | undefined {
  return issueMap.value.get(idx)
}

function onMove(e: MouseEvent) {
  const cell = (e.target as HTMLElement).closest('[data-idx]')
  if (!cell) return
  const i = Number((cell as HTMLElement).dataset.idx)
  curIdx.value = i
  emit('hover', { range: { start: i, end: i + 1 }, x: e.clientX, y: e.clientY, issue: issueOf(i) })
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
  <div ref="gridEl" class="byte-grid" @mouseover="onMove" @mouseleave="onLeave" @click="onClick">
    <div v-for="row in rows" :key="row.offset" class="byte-row">
      <span class="byte-offset">{{ row.offset.toString(16).padStart(4, '0') }}</span>
      <span
        v-for="(b, bi) in row.bytes"
        :key="bi"
        class="byte-cell"
        :class="{ g8: bi === 8, 'byte-issue': issueOf(row.offset + bi) !== undefined }"
        :data-idx="row.offset + bi"
        >{{ b }}</span
      >
    </div>
  </div>
</template>

<style scoped>
.byte-grid {
  font-family: var(--font-mono);
  font-size: 12px;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 8px 10px;
  cursor: default;
}

/* 紧凑行:24px 单元格 + 4px 间距 + 6px 行距 → 行高约 28px(IDE / Hex Editor 风格) */
.byte-row {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 6px;
}

.byte-row:last-child {
  margin-bottom: 0;
}

.byte-offset {
  width: 40px;
  flex: none;
  color: var(--text-tertiary);
  font-size: 11px;
}

/* 24px 内容 + 4px gap = 28px 槽位,scale(1.15) 放大后不挤压相邻字节 */
.byte-cell {
  width: 24px;
  height: 22px;
  line-height: 20px;
  flex: none;
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

/* 16 字节/行时,第 9 个字节前加分组间隔(8 + 8 视觉分组) */
.byte-cell.g8 {
  margin-left: 6px;
}

/* 异常区间:与协议定义不符的单元数据微红标记(高亮态仍可覆盖其上) */
.byte-cell.byte-issue {
  background: rgba(255, 77, 79, 0.14);
  border-color: rgba(255, 77, 79, 0.4);
}

[data-theme='light'] .byte-cell.byte-issue {
  background: rgba(207, 19, 34, 0.09);
  border-color: rgba(207, 19, 34, 0.35);
}

/* 同字段区间内字节:规范联动高亮(primary-hover-bg 底 + 主色描边,双主题变量) */
.byte-hl {
  background: var(--primary-hover-bg);
  box-shadow: 0 0 0 1px var(--divider-hover);
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
  background: var(--primary-hover-bg);
  box-shadow: 0 0 0 1px var(--divider-hover);
  transform: scale(1.04);
}

/* 字段行悬停来源:区间外字节降低视觉权重 */
.byte-dim {
  opacity: 0.4;
}
</style>
