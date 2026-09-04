<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  CaretRightOutlined, DownloadOutlined, ExportOutlined, StopOutlined,
} from '@ant-design/icons-vue'
import * as ServerService from '../../wailsjs/go/bridge/ServerService'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { parser } from '../../wailsjs/go/models'
import ByteGridView from '../components/parser/ByteGridView.vue'
import FieldTableView from '../components/parser/FieldTableView.vue'

interface ServerFrame {
  time: string
  vin: string
  cmd: string
  hex: string
  summary: string
  kind: 'normal' | 'unknown' | 'encrypted' | 'warn'
  unauthed?: boolean
}
interface ServerSession {
  vin: string
  peer: string
  online: boolean
  lastSeen: string
}

const cfg = reactive({ ip: '127.0.0.1', port: 32960, idleEnabled: true, idleSeconds: 60 })
const running = ref(false)
const listenAddr = ref('')
const sessions = ref<ServerSession[]>([])
const frames = ref<ServerFrame[]>([])
const detail = ref<ServerFrame | null>(null)

const RENDER_CAP = 200 // 前端渲染上限(后端环形 500,spec §5.4)

let offs: Array<() => void> = []

function subscribe() {
  offs = [
    EventsOn('server:status', (st: { running: boolean; listenAddr: string }) => {
      running.value = st.running
      listenAddr.value = st.listenAddr ?? ''
    }),
    EventsOn('server:session', (e: ServerSession) => {
      const i = sessions.value.findIndex((s) => s.vin === e.vin && s.online)
      if (e.online && i < 0) sessions.value = [e, ...sessions.value].slice(0, RENDER_CAP)
      if (!e.online && i >= 0) sessions.value.splice(i, 1)
    }),
    EventsOn('server:frame', (e: ServerFrame) => {
      frames.value = [e, ...frames.value].slice(0, RENDER_CAP)
    }),
    EventsOn('server:warn', (e: { note: string }) => {
      // 告警行独立样式(评审 m3):不冒充 unknown,避免与未知命令橙色混淆
      frames.value = [{ time: new Date().toISOString(), vin: '', cmd: 'WARN', hex: '', summary: e.note, kind: 'warn' as const }, ...frames.value].slice(0, RENDER_CAP)
    }),
  ]
}

async function loadCfg() {
  try {
    const saved = await ServerService.LoadConfig()
    Object.assign(cfg, saved)
  } catch (e) {
    message.error('读取服务端配置失败: ' + String(e))
  }
}

async function start() {
  const loopback = cfg.ip === '127.0.0.1' || cfg.ip === 'localhost'
  const doStart = async (force: boolean) => {
    try {
      const st = await ServerService.Start({ ...cfg }, force)
      if (st.running) message.success('服务已启动 ' + st.listenAddr)
    } catch (e) {
      message.error(String(e))
    }
  }
  if (loopback) return doStart(false)
  Modal.confirm({
    title: '监听地址非环回',
    content: `即将监听 ${cfg.ip}:${cfg.port},局域网内任何设备都可连接(协议无认证)。确认继续?`,
    okText: '继续监听',
    cancelText: '取消',
    onOk: () => doStart(true),
  })
}

async function stop() {
  try {
    await ServerService.Stop()
  } catch (e) {
    message.error(String(e))
  }
}

async function exportLog() {
  try {
    const path = await ServerService.ExportLog()
    if (path) message.success('已导出: ' + path)
  } catch (e) {
    message.error(String(e))
  }
}

function kindClass(kind: string) {
  if (kind === 'unknown') return 'frame-unknown'
  if (kind === 'encrypted') return 'frame-encrypted'
  if (kind === 'warn') return 'frame-warn'
  return ''
}

// ---------- 详情抽屉:扩展包下拉 + 字节/字段视图 ----------
const detailVisible = ref(false)
const parserPacks = ref<Array<{ id: string; label: string }>>([])
const selectedPackId = ref('') // '' = 不使用扩展包
const parseResult = ref<parser.Result | null>(null)

const packOptions = computed(() => [
  { label: '不使用扩展包', value: '' },
  ...parserPacks.value.map((p) => ({ label: p.label, value: p.id })),
])

async function parseDetail() {
  if (!detail.value?.hex) return
  try {
    parseResult.value = await ParserService.ParsePacket(detail.value.hex, selectedPackId.value)
  } catch (e) {
    parseResult.value = null
    message.error('解析失败: ' + String(e))
  }
}

function openDetail(row: ServerFrame) {
  detail.value = row
  detailVisible.value = true
  parseResult.value = null
  // kind≠normal(加密/未知/告警)不解析,仅展示 hex 与说明(spec §5.4)
  if (row.kind === 'normal' && row.hex) void parseDetail()
}

watch(selectedPackId, () => {
  if (detailVisible.value && detail.value?.kind === 'normal') void parseDetail()
})

// 运行中即时下发空闲断开设置(AC-10)
watch(
  () => [cfg.idleEnabled, cfg.idleSeconds] as const,
  async ([enabled, seconds]) => {
    if (!running.value) return
    try {
      await ServerService.UpdateIdle(enabled, seconds)
    } catch (e) {
      message.error('空闲断开设置下发失败: ' + String(e))
    }
  },
)

// time/lastSeen 为 RFC3339 字符串,表格展示仅保留本地时分秒
function fmtTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const sessionColumns = [
  { title: 'VIN', dataIndex: 'vin' },
  { title: 'IP:端口', dataIndex: 'peer', width: 170 },
  { title: '状态', key: 'online', width: 80 },
  { title: '最后活跃', key: 'lastSeen', width: 100 },
]

const frameColumns = [
  { title: '时间', key: 'time', width: 90 },
  { title: 'VIN', dataIndex: 'vin', width: 180 },
  { title: '命令', dataIndex: 'cmd', width: 130 },
  { title: 'HEX', key: 'hex' },
]

onMounted(async () => {
  subscribe()
  await loadCfg()
  const st = await ServerService.Status().catch(() => null)
  if (st?.running) {
    running.value = true
    listenAddr.value = st.listenAddr
  }
  // 快照行全部是活会话(注册表只存在线会话),补 online: true 供状态列渲染(评审 m3)
  const snap = (await ServerService.Sessions().catch(() => [])) ?? []
  sessions.value = snap.map((s: Omit<ServerSession, 'online'>) => ({ ...s, online: true }))
  parserPacks.value = (await ParserService.ParserPacks().catch(() => [])) ?? []
})
onUnmounted(() => offs.forEach((off) => off()))
</script>

<template>
  <div class="page-root server-page">
    <h2 class="page-title">服务端模式</h2>

    <div class="zone">
      <div class="zone-body">
        <p class="section-label">在本机启动 TCP Server,接收客户端连接与上报报文</p>
        <div class="listen-form">
          <span class="form-label">Listen IP</span>
          <a-input v-model:value="cfg.ip" size="small" class="form-input" placeholder="127.0.0.1" />
          <span class="form-label">Listen Port</span>
          <a-input-number v-model:value="cfg.port" size="small" :min="1" :max="65535" class="form-input" />
          <span class="form-label">空闲断开</span>
          <a-switch v-model:checked="cfg.idleEnabled" size="small" />
          <span class="form-label">空闲秒数</span>
          <a-input-number
            v-model:value="cfg.idleSeconds" size="small" :min="5" :max="3600"
            class="form-input form-input-idle" :disabled="!cfg.idleEnabled"
          />
          <a-button v-if="!running" type="primary" size="small" @click="start">
            <template #icon><CaretRightOutlined /></template>
            启动服务
          </a-button>
          <a-button v-else danger size="small" @click="stop">
            <template #icon><StopOutlined /></template>
            停止服务
          </a-button>
          <a-button size="small" :disabled="!running" @click="exportLog">
            <template #icon><DownloadOutlined /></template>
            导出日志
          </a-button>
        </div>
      </div>
    </div>

    <div class="zone">
      <div class="zone-body status-bar">
        <span>服务状态:</span>
        <a-tag v-if="running" color="success">运行中 {{ listenAddr }}</a-tag>
        <a-tag v-else>未启动</a-tag>
        <span class="status-hint">空闲断开:{{ cfg.idleEnabled ? `空闲 ${cfg.idleSeconds} 秒后断开` : '已关闭' }}</span>
      </div>
    </div>

    <div class="zone">
      <div class="zone-body">
        <p class="section-label">客户端会话</p>
        <a-table
          :data-source="sessions" :columns="sessionColumns" size="small" :pagination="false"
          :row-key="(r: ServerSession) => r.vin"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'online'">
              <a-tag color="success">在线</a-tag>
            </template>
            <template v-else-if="column.key === 'lastSeen'">
              {{ fmtTime(record.lastSeen) }}
            </template>
          </template>
          <template #emptyText>
            <div class="console-empty-title">暂无连接</div>
            <div class="console-empty-sub">服务启动后,接入的客户端将显示在此</div>
          </template>
        </a-table>
      </div>
    </div>

    <div class="zone">
      <div class="zone-body">
        <p class="section-label">接收报文(点击行查看解析详情)</p>
        <a-table
          :data-source="frames" :columns="frameColumns" size="small" :pagination="false"
          :row-key="(_r: ServerFrame, i: number) => i"
          :custom-row="(r: ServerFrame) => ({ onClick: () => openDetail(r) })"
          :row-class-name="(r: ServerFrame) => kindClass(r.kind)"
          class="frames-table"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'time'">{{ fmtTime(record.time) }}</template>
            <span v-else-if="column.key === 'hex'" class="hex-cell">{{ record.hex }}</span>
            <template v-else-if="column.key === 'cmd'">
              {{ record.cmd }}
              <a-tag v-if="record.unauthed" class="unauthed-tag">未登入</a-tag>
            </template>
          </template>
          <template #emptyText>
            <div class="console-empty-title">暂无报文</div>
            <div class="console-empty-sub">收到的客户端报文将显示在此</div>
          </template>
        </a-table>
      </div>
    </div>

    <a-drawer v-model:open="detailVisible" :title="detail?.cmd" width="560" class="server-frame-drawer">
      <div class="drawer-toolbar">
        <span class="form-label">扩展包</span>
        <a-select v-model:value="selectedPackId" size="small" class="pack-select" :options="packOptions" />
      </div>
      <template v-if="detail && detail.kind === 'normal'">
        <div class="drawer-section">
          <p class="section-label">报文字节视图</p>
          <ByteGridView :normalized-hex="detail.hex" :active="null" :active-source="null" :active-byte="null" />
        </div>
        <div class="drawer-section">
          <p class="section-label">解析结果({{ parseResult?.fields.length ?? 0 }} 个字段)</p>
          <FieldTableView :fields="parseResult?.fields ?? []" :active="null" />
        </div>
      </template>
      <template v-else-if="detail">
        <p class="section-label">{{ detail.summary || '该帧不做字段解析' }}</p>
        <pre v-if="detail.hex" class="drawer-hex">{{ detail.hex }}</pre>
      </template>
    </a-drawer>
  </div>
</template>

<style scoped>
.server-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.page-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.section-label {
  color: var(--text-secondary);
  font-size: 12px;
  margin: 0 0 10px;
}

.listen-form {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.form-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.form-input {
  width: 160px;
}

.form-input-idle {
  width: 110px;
}

.status-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
}

.status-hint {
  color: var(--text-tertiary);
  font-size: 12px;
}

.frames-table :deep(tbody tr) {
  cursor: pointer;
}

.hex-cell {
  font-family: var(--font-mono);
  font-size: 12px;
  word-break: break-all;
}

.drawer-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.pack-select {
  width: 220px;
}

.drawer-section {
  margin-bottom: 16px;
}

.drawer-hex {
  font-family: var(--font-mono);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--text-primary);
  margin: 8px 0 0;
}
</style>
