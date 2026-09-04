<script setup lang="ts">
import { fmtClock, fmtDuration, type SessionRow } from './types'

const props = defineProps<{
  sessions: SessionRow[]
  selectedVin: string
  now: number // 秒级 tick,驱动在线时长跳动
}>()

const emit = defineEmits<{
  (e: 'select', vin: string): void
}>()

// 在线时长:loginAt 起算;离线会话停格在最后活跃时刻
function onlineDur(s: SessionRow): string {
  if (!s.loginAt) return '-'
  const from = new Date(s.loginAt).getTime()
  if (Number.isNaN(from)) return '-'
  const last = new Date(s.lastSeen).getTime()
  const to = s.online ? props.now : Number.isNaN(last) ? props.now : Math.max(from, last)
  return fmtDuration(from, to)
}
</script>

<template>
  <aside class="session-pane">
    <div class="pane-head">
      客户端会话
      <span v-if="sessions.length" class="pane-count">{{ sessions.length }}</span>
    </div>
    <div class="session-list">
      <div
        v-for="s in sessions"
        :key="s.vin"
        class="sess-row"
        :class="{ online: s.online, selected: s.vin === selectedVin }"
        :title="s.online ? '点击选中,报文流仅显示该 VIN;再次点击取消' : '离线会话:点击可按其 VIN 过滤报文'"
        @click="emit('select', s.vin)"
      >
        <div class="sess-top">
          <span class="sess-dot" :class="{ on: s.online }" />
          <span class="sess-vin">{{ s.vin || '(未登入)' }}</span>
        </div>
        <div class="sess-sub">{{ s.peer }}</div>
        <div class="sess-meta">
          <span>{{ s.online ? '在线' : '离线' }} {{ onlineDur(s) }}</span>
          <span>RX {{ s.rxCount }} / TX {{ s.txCount }}</span>
        </div>
        <div class="sess-meta dim">最后活跃 {{ fmtClock(s.lastSeen) }}</div>
      </div>
      <div v-if="!sessions.length" class="pane-empty">
        <div class="pe-title">暂无客户端连接</div>
        <div class="pe-sub">服务启动后,接入的客户端将显示在此</div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.session-pane {
  width: 240px;
  flex: none;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border-right: 1px solid var(--border-subtle);
}

.pane-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.pane-count {
  margin-left: auto;
  color: var(--text-tertiary);
  font-weight: 400;
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.session-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.sess-row {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-subtle);
  cursor: pointer;
  transition: background 0.15s;
}

.sess-row:hover {
  background: var(--row-hover-bg);
}

.sess-row.selected {
  background: var(--hl-bg);
  box-shadow: inset 2px 0 0 var(--primary);
}

/* 离线会话灰化保留,报文仍可按其 VIN 过滤 */
.sess-row:not(.online) {
  opacity: 0.55;
}

.sess-row.selected:not(.online) {
  opacity: 0.75;
}

.sess-top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.sess-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--text-tertiary);
  flex: none;
}

.sess-dot.on {
  background: var(--success);
}

.sess-vin {
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sess-sub {
  margin-top: 2px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sess-meta {
  margin-top: 3px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.sess-meta.dim {
  color: var(--text-tertiary);
}

.pane-empty {
  margin: auto;
  padding: 24px 12px;
  text-align: center;
  user-select: none;
}

.pe-title {
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 4px;
}

.pe-sub {
  color: var(--text-tertiary);
  font-size: 12px;
}
</style>
