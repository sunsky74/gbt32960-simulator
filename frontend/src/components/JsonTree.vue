<script setup lang="ts">
import {computed, ref} from 'vue'

// JsonTree:可折叠 JSON 树,用于展示 GB/T 32960 解析后的帧数据
// 约定:根层条目直接展开渲染(无根标签行);嵌套容器默认折叠;
// 所有行点击均阻断冒泡,避免触发宿主(如 ConsolePanel 报文行)的点击行为

// 单条目:key 为展开状态索引(对象键名/数组下标),text 为展示文本(数组下标带方括号)
interface JtEntry {
  key: string
  text: string
  value: unknown
}

const props = defineProps<{value: unknown}>()

// 当前实例内已展开条目的 key 集合;更深层级由递归子实例各自管理,天然互不影响
const expandedKeys = ref<Set<string>>(new Set())

// 是否为容器(对象/数组)
function isContainer(v: unknown): boolean {
  return typeof v === 'object' && v !== null
}

// 根层条目列表:对象取键值对,数组取下标条目
const entries = computed<JtEntry[]>(() => {
  const v = props.value
  if (!isContainer(v)) return []
  if (Array.isArray(v)) {
    return v.map((item, i) => ({key: String(i), text: `[${i}]`, value: item}))
  }
  return Object.entries(v as Record<string, unknown>).map(([k, val]) => ({
    key: k,
    text: k,
    value: val,
  }))
})

function isOpen(key: string): boolean {
  return expandedKeys.value.has(key)
}

// 切换展开/折叠(行点击配合 @click.stop 使用)
function toggle(key: string): void {
  const next = new Set(expandedKeys.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  expandedKeys.value = next
}

// 折叠摘要:数组 [N items] / 对象 {N items};空容器渲染 [] / {}
function countText(v: unknown): string {
  if (Array.isArray(v)) return v.length === 0 ? '[]' : `[${v.length} items]`
  const n = Object.keys(v as Record<string, unknown>).length
  return n === 0 ? '{}' : `{${n} items}`
}

// 原始值展示文本:字符串加双引号,其余(数字/布尔/null)原样
function primText(v: unknown): string {
  return typeof v === 'string' ? JSON.stringify(v) : String(v)
}

// 原始值着色类:字符串绿 / 数字蓝 / 布尔橙 / null 灰(均取自全局设计 token)
function primClass(v: unknown): string {
  switch (typeof v) {
    case 'string':
      return 'jt-str'
    case 'number':
      return 'jt-num'
    case 'boolean':
      return 'jt-bool'
    default:
      return 'jt-null'
  }
}
</script>

<template>
  <div class="jt">
    <!-- 根值为原始值时直接渲染原始行(防御性;正常根应为对象/数组) -->
    <div v-if="entries.length === 0 && !isContainer(value)" class="jt-row" @click.stop>
      <span class="jt-val" :class="primClass(value)">{{ primText(value) }}</span>
    </div>
    <template v-for="entry in entries" :key="entry.key">
      <!-- 容器条目行:整行可点击切换;stop 防止冒泡触发宿主行 -->
      <div
        v-if="isContainer(entry.value)"
        class="jt-row jt-container"
        role="button"
        @click.stop="toggle(entry.key)"
      >
        <span class="jt-toggle">{{ isOpen(entry.key) ? '▾' : '▸' }}</span>
        <span class="jt-key">{{ entry.text }}</span>
        <span class="jt-count">{{ countText(entry.value) }}</span>
      </div>
      <!-- 展开后的子树:递归渲染,每层缩进 14px -->
      <div v-if="isContainer(entry.value) && isOpen(entry.key)" class="jt-children" @click.stop>
        <JsonTree :value="entry.value" />
      </div>
      <!-- 原始值条目行:键 + 着色值 -->
      <div v-if="!isContainer(entry.value)" class="jt-row" @click.stop>
        <span class="jt-key">{{ entry.text }}</span>
        <span class="jt-val" :class="primClass(entry.value)">{{ primText(entry.value) }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.jt {
  font-family: var(--font-mono);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--text-primary);
}

/* 行:pre-wrap + break-all,长 hex/VIN 折行不溢出 */
.jt-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  padding: 1px 4px;
  white-space: pre-wrap;
  word-break: break-all;
}

/* 容器行可点击,hover 反馈与全局 item-hover 一致 */
.jt-row.jt-container {
  cursor: pointer;
  border-radius: 3px;
}

.jt-row.jt-container:hover {
  background: var(--item-hover-bg);
}

/* 折叠指示符:▸ 收起 / ▾ 展开 */
.jt-toggle {
  flex: none;
  width: 12px;
  color: var(--text-tertiary);
}

.jt-key {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.jt-count {
  color: var(--text-tertiary);
}

.jt-val {
  min-width: 0;
}

.jt-val.jt-str {
  color: var(--success);
}

.jt-val.jt-num {
  color: var(--primary);
}

.jt-val.jt-bool {
  color: var(--warning);
}

.jt-val.jt-null {
  color: var(--text-tertiary);
}

/* 子层缩进:每层 14px,左侧细分隔线作层级参考线 */
.jt-children {
  padding-left: 14px;
  border-left: 1px solid var(--border-subtle);
}
</style>
