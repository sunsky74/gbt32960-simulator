<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { parser as parserNs } from '../../../wailsjs/go/models'
import type { ByteRange } from './ByteGridView.vue'

type ParsedField = parserNs.Field

const props = defineProps<{ fields: ParsedField[]; active: ByteRange | null }>()

// ---------- 分组折叠:单表树形行,分组头为特殊行(同一套表头/列宽) ----------
const COLLAPSE_KEY = 'rt-collapsed-groups'

interface GroupRow {
  key: string
  isGroup: true
  name: string
  count: number
  children: Array<{ key: string; isGroup: false; field: ParsedField }>
}

let groupSeq = 0
const groupedRows = computed<GroupRow[]>(() => {
  groupSeq = 0
  const out: GroupRow[] = []
  let header: ParsedField[] = []
  let cur: GroupRow | null = null
  // key 带自增序号保证 row-key 唯一(同名 TLV 组可重复出现);折叠按 name 持久化
  const mk = (name: string): GroupRow => ({ key: `g:${groupSeq++}:${name}`, isGroup: true, name, count: 0, children: [] })
  const push = () => {
    if (cur && cur.children.length > 0) out.push(cur)
  }
  for (const f of props.fields) {
    if (f.offset < 24) {
      header.push(f)
      continue
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
    cur.children.push({ key: String(f.offset), isGroup: false, field: f })
  }
  push()
  if (header.length > 0) {
    const h = mk('报文头')
    h.children = header.map((f) => ({ key: String(f.offset), isGroup: false as const, field: f }))
    out.unshift(h)
  }
  for (const g of out) g.count = g.children.length
  return out
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
const expandedKeys = computed(() =>
  groupedRows.value.filter((g) => !collapsed.value.has(g.name)).map((g) => g.key),
)

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

function customRow(record: GroupRow | { key: string; isGroup: false }) {
  if (!record.isGroup) return {}
  const g = record as GroupRow
  return {
    onClick: () => toggleGroup(g.name),
  }
}

function rowClassName(record: { isGroup: boolean }) {
  return record.isGroup ? 'group-row' : ''
}
const emit = defineEmits<{
  (e: 'hover', r: ByteRange | null): void
  (e: 'pin', r: ByteRange | null): void
}>()

const columns = [
  { title: 'Offset', key: 'offset', width: 66 },
  { title: 'Len', key: 'length', width: 52 },
  { title: '字段名称', key: 'name', width: 190 },
  { title: '类型', key: 'type', width: 66 },
  { title: '原始值', key: 'rawValue', width: 130 },
  { title: '偏移值', key: 'offsetVal', width: 104 },
  { title: '翻译值', key: 'translate' },
  { title: '单位', key: 'unit', width: 76 },
]

// 区间相交:字段行 ↔ 字节区间
function fieldIntersects(f: ParsedField, r: ByteRange): boolean {
  return f.offset < r.end && f.offset + f.length > r.start
}

const wrapEl = ref<HTMLElement | null>(null)
let hlRows: Element[] = []

watch(
  () => [props.active, props.fields] as const,
  ([r]) => {
    for (const el of hlRows) el.classList.remove('row-hl')
    hlRows = []
    if (!r || !wrapEl.value) return
    const rows = wrapEl.value.querySelectorAll('tr[data-row-key]')
    rows.forEach((el) => {
      const key = Number((el as HTMLElement).dataset.rowKey)
      const f = props.fields.find((x) => x.offset === key)
      if (f && fieldIntersects(f, r)) {
        el.classList.add('row-hl')
        hlRows.push(el)
      }
    })
  },
)

function rangeOfRow(tr: HTMLElement): ByteRange | null {
  const key = Number(tr.dataset.rowKey)
  const f = props.fields.find((x) => x.offset === key)
  return f ? { start: f.offset, end: f.offset + f.length } : null
}

function onMove(e: MouseEvent) {
  const tr = (e.target as HTMLElement).closest('tr[data-row-key]') as HTMLElement | null
  if (!tr) return
  emit('hover', rangeOfRow(tr))
}

function onClick(e: MouseEvent) {
  const tr = (e.target as HTMLElement).closest('tr[data-row-key]') as HTMLElement | null
  emit('pin', tr ? rangeOfRow(tr) : null)
}
</script>

<template>
  <div
    ref="wrapEl"
    class="field-table"
    @mouseover="onMove"
    @mouseleave="emit('hover', null)"
    @click="onClick"
  >
    <a-table
      :data-source="groupedRows"
      :columns="columns"
      size="small"
      :pagination="false"
      :scroll="{ x: 880, y: 460 }"
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
            <span class="group-label">{{ record.name }} <em>· {{ record.count }} 字段</em></span>
          </template>
        </template>
        <template v-else-if="column.key === 'offset'">{{ record.field.offset }}</template>
        <template v-else-if="column.key === 'length'">{{ record.field.length }}</template>
        <template v-else-if="column.key === 'name'">{{ record.field.name }}</template>
        <template v-else-if="column.key === 'type'">{{ record.field.type }}</template>
        <template v-else-if="column.key === 'rawValue'">
          <span class="mono" :title="record.field.rawHex">{{ record.field.rawValue }}</span>
        </template>
        <template v-else-if="column.key === 'offsetVal'">
          <span v-if="record.field.offsetVal !== '-'" class="phys">{{ record.field.offsetVal }}<em v-if="record.field.unit"> {{ record.field.unit }}</em></span>
          <span v-else>-</span>
        </template>
        <template v-else-if="column.key === 'translate'">{{ record.field.translate }}</template>
        <template v-else-if="column.key === 'unit'">{{ record.field.unit }}</template>
      </template>
    </a-table>
  </div>
</template>

<style scoped>
.group-row > td {
  background: var(--bg-elevated) !important;
  cursor: pointer;
  user-select: none;
}

.group-row:hover > td {
  background: var(--item-hover-bg) !important;
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
  font-family: SFMono-Regular, Consolas, Menlo, monospace;
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
