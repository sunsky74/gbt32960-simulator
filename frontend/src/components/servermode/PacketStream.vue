<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { cmdNameOf, fmtMs, type StreamRow } from './types'

const props = defineProps<{
  frames: StreamRow[]
  running: boolean
  listenAddr: string
  sessionCount: number
  selectedId: number | null
}>()

const emit = defineEmits<{
  (e: 'select', row: StreamRow): void
  (e: 'start'): void
}>()

type FilterKey = 'all' | 'rx' | 'tx' | 'error'
const filter = ref<FilterKey>('all')
const search = ref('')
const autoScroll = ref(true)
const listEl = ref<HTMLElement | null>(null)

// 过滤 tab + 关键字(VIN/命令/hex/摘要),与 ConsolePanel 搜索惯例一致
const visibleRows = computed<StreamRow[]>(() => {
  let list = props.frames
  if (filter.value === 'rx') list = list.filter((r) => r.dir === 'rx')
  else if (filter.value === 'tx') list = list.filter((r) => r.dir === 'tx')
  else if (filter.value === 'error') list = list.filter((r) => r.kind === 'warn')
  const kw = search.value.trim().toLowerCase()
  if (kw) {
    list = list.filter(
      (r) =>
        r.vin.toLowerCase().includes(kw) ||
        r.cmd.toLowerCase().includes(kw) ||
        r.hex.toLowerCase().includes(kw) ||
        r.summary.toLowerCase().includes(kw),
    )
  }
  return list
})

// 新帧置底 + 自动滚动到底(跟随开关关闭时静止阅读)
watch(
  () => [props.frames.length, filter.value, search.value] as const,
  async () => {
    if (!autoScroll.value) return
    await nextTick()
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  },
)

// 空状态三态:服务未启动 / 已启动无客户端 / 有客户端无报文(或过滤后空)
const emptyState = computed<'off' | 'noClient' | 'noFrame' | null>(() => {
  if (visibleRows.value.length > 0) return null
  if (!props.running) return 'off'
  if (props.sessionCount === 0) return 'noClient'
  return 'noFrame'
})

function dirText(r: StreamRow): string {
  if (r.kind === 'link') return 'Link'
  if (r.kind === 'warn') return 'Error'
  return r.dir === 'tx' ? 'TX' : 'RX'
}

function nameOf(r: StreamRow): string {
  if (r.kind === 'link') return '链路事件'
  if (r.kind === 'warn') return '告警'
  return cmdNameOf(r.cmd) || '-'
}

function statusOf(r: StreamRow): { text: string; cls: string } {
  if (r.kind === 'warn') return { text: '告警', cls: 'st-warn' }
  if (r.kind === 'unknown') return { text: '未知', cls: 'st-unknown' }
  if (r.kind === 'encrypted') return { text: '加密', cls: 'st-encrypted' }
  if (r.unauthed) return { text: '未登入', cls: 'st-unauthed' }
  return { text: 'OK', cls: 'st-ok' }
}

// 行级配色沿用既有三色类名(未知橙/加密紫/告警红,亮暗主题各自覆盖)
function rowClass(r: StreamRow): string {
  if (r.kind === 'unknown') return 'frame-unknown'
  if (r.kind === 'encrypted') return 'frame-encrypted'
  if (r.kind === 'warn') return 'frame-warn'
  return ''
}

function bytesOf(r: StreamRow): string {
  return r.hex ? String(r.hex.length / 2) : '-'
}
</script>

<template>
  <section class="zone">
    <header class="zone-head">
      <span class="zone-title">实时通信</span>
      <a-radio-group v-model:value="filter" size="small" button-style="solid">
        <a-radio-button value="all">全部</a-radio-button>
        <a-radio-button value="rx">RX</a-radio-button>
        <a-radio-button value="tx">TX</a-radio-button>
        <a-radio-button value="error">Error</a-radio-button>
      </a-radio-group>
      <a-input-search
        v-model:value="search"
        size="small"
        class="search-input"
        placeholder="搜索 VIN / 命令 / HEX / 摘要"
        allow-clear
      />
      <a-checkbox v-model:checked="autoScroll">跟随</a-checkbox>
      <span class="toolbar-count">{{ visibleRows.length }} 条</span>
    </header>

    <div v-if="emptyState" class="zone-empty">
      <template v-if="emptyState === 'off'">
        <div class="console-empty-title">服务端未启动</div>
        <div class="console-empty-sub mono">启动服务后等待客户端连接</div>
        <a-button type="primary" size="small" @click="emit('start')">启动服务</a-button>
      </template>
      <template v-else-if="emptyState === 'noClient'">
        <div class="console-empty-title">等待客户端连接...</div>
        <div class="console-empty-sub mono">{{ listenAddr || '未监听' }}</div>
      </template>
      <template v-else>
        <div class="console-empty-title">等待客户端发送报文...</div>
      </template>
    </div>

    <template v-else>
      <div class="stream-head">
        <span>时间</span>
        <span>方向</span>
        <span>命令</span>
        <span>名称</span>
        <span>字节</span>
        <span>状态</span>
        <span>摘要</span>
      </div>
      <div ref="listEl" class="stream-list">
        <div
          v-for="r in visibleRows"
          :key="r.id"
          class="stream-row"
          :class="[rowClass(r), { selected: r.id === selectedId }]"
          @click="emit('select', r)"
        >
          <span class="c-time mono">{{ fmtMs(r.time) }}</span>
          <span class="dir-badge" :class="r.kind === 'link' ? 'dir-link' : r.kind === 'warn' ? 'dir-error' : r.dir">
            {{ dirText(r) }}
          </span>
          <span class="c-cmd mono">{{ r.cmd || '-' }}</span>
          <span class="c-name">{{ nameOf(r) }}</span>
          <span class="c-bytes mono">{{ bytesOf(r) }}</span>
          <span class="c-status">
            <span class="st" :class="statusOf(r).cls">{{ statusOf(r).text }}</span>
          </span>
          <span class="c-summary" :title="r.summary">{{ r.summary || '-' }}</span>
        </div>
      </div>
    </template>
  </section>
</template>
