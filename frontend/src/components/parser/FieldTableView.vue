<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { parser as parserNs } from '../../../wailsjs/go/models'
import type { ByteRange } from './ByteGridView.vue'

type ParsedField = parserNs.Field

const props = defineProps<{ fields: ParsedField[]; active: ByteRange | null }>()

// ---------- 分组折叠:单表树形行,分组头为特殊行(同一套表头/列宽) ----------
const COLLAPSE_KEY = 'rt-collapsed-groups'

interface FieldRow {
  key: string
  isGroup: false
  field: ParsedField
}

interface GroupRow {
  key: string
  isGroup: true
  name: string
  count: number
  children: FieldRow[]
}

// 纯 computed:组序号为局部变量,不落模块级状态(避免多次求值/并发污染)
const model = computed<{ rows: GroupRow[]; byKey: Map<string, ParsedField> }>(() => {
  let groupSeq = 0
  const byKey = new Map<string, ParsedField>()
  const out: GroupRow[] = []
  const header: Array<{ f: ParsedField; idx: number }> = []
  let cur: GroupRow | null = null
  // key 带自增序号保证 row-key 唯一(同名 TLV 组可重复出现);折叠按 name 持久化
  const mk = (name: string): GroupRow => ({
    key: `g:${groupSeq++}:${name}`,
    isGroup: true,
    name,
    count: 0,
    children: [],
  })
  const push = () => {
    if (cur && cur.children.length > 0) out.push(cur)
  }
  // 复合行键:offset + name + 全局索引,消除同 offset 字段(重叠/占位)key 冲突隐患
  const row = (f: ParsedField, idx: number): FieldRow => {
    const key = `${f.offset}-${f.name}-${idx}`
    byKey.set(key, f)
    return { key, isGroup: false, field: f }
  }
  props.fields.forEach((f, idx) => {
    if (f.offset < 24) {
      header.push({ f, idx })
      return
    }
    if (f.name === '校验码 BCC') {
      push()
      cur = mk('BCC 校验')
    } else if (f.name === '数据类型标志 (TLV)') {
      push()
      cur = mk(f.translate || '数据组')
    } else if (!cur) {
      cur = mk('数据单元')
    }
    cur.children.push(row(f, idx))
  })
  push()
  if (header.length > 0) {
    const h = mk('报文头')
    h.children = header.map(({ f, idx }) => row(f, idx))
    out.unshift(h)
  }
  for (const g of out) g.count = g.children.length
  return { rows: out, byKey }
})

function loadCollapsed(): Set<string> {
  try {
    const arr = JSON.parse(localStorage.getItem(COLLAPSE_KEY) || '[]')
    return new Set(Array.isArray(arr) ? arr : [])
  } catch {
    return new Set()
  }
}

// collapsed 存组名(跨解析稳定);expandedKeys 用唯一 key
const collapsed = ref<Set<string>>(loadCollapsed())
const expandedKeys = computed(() => model.value.rows.filter((g) => !collapsed.value.has(g.name)).map((g) => g.key))

function toggleGroup(name: string) {
  const next = new Set(collapsed.value)
  if (next.has(name)) {
    next.delete(name)
  } else {
    next.add(name)
  }
  collapsed.value = next
  localStorage.setItem(COLLAPSE_KEY, JSON.stringify([...next]))
}

function customRow(record: GroupRow | FieldRow) {
  if (!record.isGroup) return {}
  return {
    onClick: () => toggleGroup(record.name),
  }
}

function rowClassName(record: { isGroup: boolean }) {
  return record.isGroup ? 'group-row' : ''
}

const emit = defineEmits<{
  (e: 'hover', r: ByteRange | null, pos?: { x: number; y: number }): void
  (e: 'pin', r: ByteRange | null): void
}>()

const columns = [
  { title: 'Offset', key: 'offset', width: 72 },
  { title: 'Len', key: 'length', width: 56 },
  { title: '字段名称', key: 'name', width: 210 },
  { title: '类型', key: 'type', width: 64 },
  { title: '原始值', key: 'rawValue', width: 132 },
  { title: '解析值', key: 'offsetVal', width: 120 },
  { title: '翻译值', key: 'translate' },
]

// 区间相交:字段行 ↔ 字节区间
function fieldIntersects(f: ParsedField, r: ByteRange): boolean {
  return f.offset < r.end && f.offset + f.length > r.start
}

const wrapEl = ref<HTMLElement | null>(null)
let hlRows: Element[] = []

watch(
  // expandedKeys 一并依赖:分组折叠/展开会重建 DOM 行,需重新应用高亮
  () => [props.active, model.value, expandedKeys.value] as const,
  ([r]) => {
    for (const el of hlRows) el.classList.remove('row-hl')
    hlRows = []
    if (!r || !wrapEl.value) return
    const byKey = model.value.byKey
    wrapEl.value.querySelectorAll('tr[data-row-key]').forEach((el) => {
      const key = (el as HTMLElement).dataset.rowKey
      const f = key ? byKey.get(key) : undefined
      if (f && fieldIntersects(f, r)) {
        el.classList.add('row-hl')
        hlRows.push(el)
      }
    })
  },
  { flush: 'post' },
)

function rangeOfRow(tr: HTMLElement): ByteRange | null {
  const f = model.value.byKey.get(tr.dataset.rowKey || '')
  return f ? { start: f.offset, end: f.offset + f.length } : null
}

function onMove(e: MouseEvent) {
  const tr = (e.target as HTMLElement).closest('tr[data-row-key]') as HTMLElement | null
  if (!tr) return
  // 携带鼠标坐标:父级据此定位/跟随悬停信息卡片
  emit('hover', rangeOfRow(tr), { x: e.clientX, y: e.clientY })
}

function onClick(e: MouseEvent) {
  const tr = (e.target as HTMLElement).closest('tr[data-row-key]') as HTMLElement | null
  if (!tr) {
    emit('pin', null)
    return
  }
  const key = tr.dataset.rowKey || ''
  // 分组行:折叠行为由 customRow 接管,不清除钉住选中
  if (!model.value.byKey.has(key)) {
    if (key.startsWith('g:')) return
    emit('pin', null)
    return
  }
  emit('pin', rangeOfRow(tr))
}

// 固定表头 + 内容独立滚动:y 实测 = 容器高 - 上方 label(含 margin)- 表头 - 2px 余量
const scrollY = ref(420)
const labelEl = ref<HTMLElement | null>(null)
let ro: ResizeObserver | null = null

function refreshScroll() {
  const wrap = wrapEl.value
  if (!wrap) return
  const wrapTop = wrap.getBoundingClientRect().top
  const headerEl = wrap.querySelector('.ant-table-header')
  if (!labelEl.value) {
    // label 缺失时回退:按常量 61px 开销估算
    scrollY.value = Math.max(100, wrap.clientHeight - 61)
    return
  }
  const labelH = wrapTop - labelEl.value.getBoundingClientRect().top
  const headerH = headerEl ? headerEl.getBoundingClientRect().height : 39
  scrollY.value = Math.max(100, wrap.clientHeight - labelH - headerH - 2)
}

onMounted(() => {
  // label 是父组件中紧邻本组件的兄弟元素
  const sib = wrapEl.value?.previousElementSibling as HTMLElement | null
  if (sib && sib.classList.contains('section-label')) labelEl.value = sib
  refreshScroll()
  ro = new ResizeObserver(refreshScroll)
  if (wrapEl.value) ro.observe(wrapEl.value)
  if (labelEl.value) ro.observe(labelEl.value)
})

onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
})
</script>

<template>
  <div ref="wrapEl" class="field-table" @mousemove="onMove" @mouseleave="emit('hover', null)" @click="onClick">
    <a-table
      :data-source="model.rows"
      :columns="columns"
      size="small"
      :pagination="false"
      :scroll="{ x: 820, y: scrollY }"
      row-key="key"
      :indent-size="0"
      :expanded-row-keys="expandedKeys"
      :custom-row="customRow"
      :row-class-name="rowClassName"
      bordered
    >
      <template #bodyCell="{ column, record }">
        <template v-if="record.isGroup">
          <template v-if="column.key === 'name'">
            <span class="group-label"
              >{{ record.name }} <em>· {{ record.count }} 字段</em></span
            >
          </template>
        </template>
        <template v-else-if="column.key === 'offset'">
          <span class="mono">{{ record.field.offset }}</span>
        </template>
        <template v-else-if="column.key === 'length'">
          <span class="mono">{{ record.field.length }}</span>
        </template>
        <template v-else-if="column.key === 'name'">
          <span class="ellipsis-cell" :title="record.field.name">{{ record.field.name }}</span>
        </template>
        <template v-else-if="column.key === 'type'">
          <span class="mono">{{ record.field.type }}</span>
        </template>
        <template v-else-if="column.key === 'rawValue'">
          <span class="mono ellipsis-cell" :title="record.field.rawHex">{{ record.field.rawValue }}</span>
        </template>
        <template v-else-if="column.key === 'offsetVal'">
          <span
            v-if="record.field.offsetVal !== '-'"
            class="phys ellipsis-cell"
            :title="record.field.offsetVal + (record.field.unit ? ' ' + record.field.unit : '')"
            >{{ record.field.offsetVal }}<em v-if="record.field.unit"> {{ record.field.unit }}</em></span
          >
          <span v-else>-</span>
        </template>
        <template v-else-if="column.key === 'translate'">
          <span class="ellipsis-cell" :title="record.field.translate">{{ record.field.translate }}</span>
        </template>
      </template>
    </a-table>
  </div>
</template>

<style scoped>
.field-table {
  height: 100%;
}

:deep(tr.group-row) > td {
  background: var(--bg-elevated) !important;
  cursor: pointer;
}

:deep(tr.group-row:hover) > td {
  background: var(--item-hover-bg) !important;
}

:deep(.ant-table-row-expand-icon) {
  margin-inline-end: 4px;
}

.group-label {
  font-weight: 600;
  color: var(--text-primary);
}

.group-label em {
  font-style: normal;
  color: var(--text-tertiary);
  font-weight: 400;
  font-size: 12px;
}

.mono {
  font-family: var(--font-mono);
}

/* 长文本截断,hover 显示完整 */
.ellipsis-cell {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.phys {
  color: var(--primary);
}

.phys em {
  font-style: normal;
  color: var(--text-tertiary);
  font-size: 12px;
}

.field-table :deep(tr.row-hl) > td {
  background: var(--hl-bg) !important;
}
</style>
