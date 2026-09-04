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
  <section class="stream-pane">
    <div class="stream-toolbar">
      <a-radio-group v-model:value="filter" size="small" button-style="solid">
        <a-radio-button value="all">全部</a-radio-button>
        <a-radio-button value="rx">RX</a-radio-button>
        <a-radio-button value="tx">TX</a-radio-button>
        <a-radio-button value="error">Error</a-radio-button>
      </a-radio-group>
      <a-input-search
        v-model:value="search"
        size="small"
        class="stream-search"
        placeholder="搜索 VIN / 命令 / HEX / 摘要"
        allow-clear
      />
      <label class="follow-label">
        <a-checkbox v-model:checked="autoScroll" />跟随
      </label>
      <span class="stream-count">{{ visibleRows.length }} 条</span>
    </div>

    <div v-if="emptyState" class="stream-empty">
      <template v-if="emptyState === 'off'">
        <div class="se-title">服务端未启动</div>
        <div class="se-sub">启动服务后等待客户端连接</div>
        <a-button type="primary" size="small" @click="emit('start')">启动服务</a-button>
      </template>
      <template v-else-if="emptyState === 'noClient'">
        <div class="se-title">等待客户端连接...</div>
        <div class="se-sub mono">{{ listenAddr || '未监听' }}</div>
      </template>
      <template v-else>
        <div class="se-title">等待客户端发送报文...</div>
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
          <span class="c-dir" :class="r.kind === 'link' ? 'dir-link' : r.kind === 'warn' ? 'dir-error' : r.dir">
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

<style scoped>
.stream-pane {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
}

.stream-toolbar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--border-subtle);
  flex-wrap: wrap;
}

.stream-search {
  width: 220px;
}

.follow-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 12px;
}

.stream-count {
  margin-left: auto;
  color: var(--text-tertiary);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

/* ---------- 空状态三态 ---------- */
.stream-empty {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  user-select: none;
}

.se-title {
  color: var(--text-secondary);
  font-size: 14px;
}

.se-sub {
  color: var(--text-tertiary);
  font-size: 12px;
}

.se-sub.mono {
  font-family: var(--font-mono);
}

/* ---------- IDE Network Console 风格:grid 列头 + 紧凑行 ---------- */
.stream-head,
.stream-row {
  display: grid;
  grid-template-columns: 92px 44px 52px 72px 44px 60px minmax(0, 1fr);
  gap: 8px;
  align-items: center;
  padding: 0 12px;
}

.stream-head {
  flex: none;
  height: 26px;
  font-size: 11px;
  color: var(--text-tertiary);
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-elevated);
  user-select: none;
}

.stream-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 0;
}

.stream-row {
  height: 24px;
  font-size: 12px;
  cursor: pointer;
  border-bottom: 1px solid var(--border-subtle);
  transition: background 0.12s;
  font-variant-numeric: tabular-nums;
}

.stream-row:hover {
  background: var(--row-hover-bg);
}

.stream-row.selected {
  background: var(--hl-bg);
}

.mono {
  font-family: var(--font-mono);
}

.c-time {
  font-size: 11px;
  color: var(--text-tertiary);
  white-space: nowrap;
}

/* 方向徽标:RX 绿 / TX 蓝 / Link 紫 / Error 红(与客户端模式 console 色统一) */
.c-dir {
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.c-dir.rx {
  color: var(--success);
}

.c-dir.tx {
  color: var(--primary);
}

.c-dir.dir-link {
  color: #9254de;
}

.c-dir.dir-error {
  color: var(--error);
}

.c-cmd {
  color: var(--text-primary);
  white-space: nowrap;
}

.c-name {
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.c-bytes {
  color: var(--text-tertiary);
  text-align: right;
}

.c-summary {
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 状态列:未知橙 / 加密紫 / 未登入灰 / 告警红 / OK 绿 */
.st {
  display: inline-block;
  font-size: 11px;
  line-height: 16px;
  padding: 0 6px;
  border-radius: 2px;
  border: 1px solid var(--border-subtle);
  white-space: nowrap;
}

.st-ok {
  color: var(--success);
}

.st-unknown {
  color: #d46b08;
  border-color: rgba(212, 107, 8, 0.4);
}

.st-encrypted {
  color: #9254de;
  border-color: rgba(146, 84, 222, 0.4);
}

.st-unauthed {
  color: var(--text-tertiary);
}

.st-warn {
  color: var(--error);
  border-color: rgba(255, 77, 79, 0.4);
}

/* ---------- 行级三色类(沿用旧 a-table 行类名;亮暗主题各自覆盖) ---------- */
.stream-row.frame-unknown .c-cmd,
.stream-row.frame-unknown .c-name,
.stream-row.frame-unknown .c-summary {
  color: #d46b08;
}

.stream-row.frame-encrypted .c-cmd,
.stream-row.frame-encrypted .c-name,
.stream-row.frame-encrypted .c-summary {
  color: #9254de;
}

.stream-row.frame-warn .c-cmd,
.stream-row.frame-warn .c-summary {
  color: var(--error);
}

:root[data-theme='light'] .stream-row.frame-unknown .c-cmd,
:root[data-theme='light'] .stream-row.frame-unknown .c-name,
:root[data-theme='light'] .stream-row.frame-unknown .c-summary {
  color: #ad4e00;
}

:root[data-theme='light'] .stream-row.frame-encrypted .c-cmd,
:root[data-theme='light'] .stream-row.frame-encrypted .c-name,
:root[data-theme='light'] .stream-row.frame-encrypted .c-summary {
  color: #6424c2;
}

:root[data-theme='light'] .stream-row.frame-warn .c-cmd,
:root[data-theme='light'] .stream-row.frame-warn .c-summary {
  color: #a8071a;
}

:root[data-theme='light'] .st-unknown {
  color: #ad4e00;
}

:root[data-theme='light'] .st-encrypted {
  color: #6424c2;
}

:root[data-theme='light'] .st-warn {
  color: #a8071a;
}
</style>
