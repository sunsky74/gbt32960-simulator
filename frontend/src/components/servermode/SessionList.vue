<script setup lang="ts">
import { computed, ref } from 'vue'
import { fmtClock, fmtDuration, type SessionRow } from './types'

const props = defineProps<{
  sessions: SessionRow[]
  selectedVin: string
  now: number // 秒级 tick,驱动在线时长跳动
}>()

const emit = defineEmits<{
  (e: 'select', vin: string): void
}>()

// VIN / peer(IP:Port)关键字过滤(仅影响显示,不改动会话数据)
const search = ref('')
const visibleSessions = computed<SessionRow[]>(() => {
  const kw = search.value.trim().toLowerCase()
  if (!kw) return props.sessions
  return props.sessions.filter((s) => s.vin.toLowerCase().includes(kw) || s.peer.toLowerCase().includes(kw))
})

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
  <aside class="zone session-zone">
    <header class="zone-head">
      <span class="zone-title">客户端会话</span>
      <a-input-search v-model:value="search" size="small" class="zone-search" placeholder="搜索 VIN / IP" allow-clear />
      <span v-if="sessions.length" class="toolbar-count">{{ sessions.length }}</span>
    </header>
    <div class="session-list">
      <div
        v-for="s in visibleSessions"
        :key="s.vin"
        class="sess-row"
        :class="{ online: s.online, selected: s.vin === selectedVin }"
        :title="s.online ? '点击选中,报文流仅显示该 VIN;再次点击取消' : '离线会话:点击可按其 VIN 过滤报文'"
        @click="emit('select', s.vin)"
      >
        <div class="sess-top">
          <span class="sess-dot" :class="{ on: s.online }" />
          <span class="sess-vin">{{ s.vin || '(未登入)' }}</span>
          <a-tag v-if="s.platform" color="geekblue" class="sess-platform-tag">平台链路</a-tag>
        </div>
        <div class="sess-sub">{{ s.peer }}</div>
        <div class="sess-meta">
          <span>{{ s.online ? '在线' : '离线' }} {{ onlineDur(s) }}</span>
          <span>RX {{ s.rxCount }} / TX {{ s.txCount }}</span>
        </div>
        <div class="sess-meta dim">最后活跃 {{ fmtClock(s.lastSeen) }}</div>
      </div>
      <div v-if="!visibleSessions.length" class="console-empty">
        <div class="console-empty-title">{{ sessions.length ? '无匹配会话' : '暂无客户端连接' }}</div>
        <div class="console-empty-sub">
          {{ sessions.length ? '调整搜索关键字后重试' : '服务启动后,接入的客户端将显示在此' }}
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sess-platform-tag {
  margin-left: 4px;
  padding: 0 4px;
  font-size: 10px;
  line-height: 16px;
}
</style>
