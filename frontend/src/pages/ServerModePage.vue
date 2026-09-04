<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  CaretRightOutlined, ClearOutlined, DownloadOutlined, SettingOutlined, StopOutlined,
} from '@ant-design/icons-vue'
import * as ServerService from '../../wailsjs/go/bridge/ServerService'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import ResizableDivider from '../components/layout/ResizableDivider.vue'
import SessionList from '../components/servermode/SessionList.vue'
import PacketStream from '../components/servermode/PacketStream.vue'
import PacketDetail from '../components/servermode/PacketDetail.vue'
import { RENDER_CAP, fmtDuration, type SessionRow, type StreamRow } from '../components/servermode/types'

const cfg = reactive({ ip: '127.0.0.1', port: 32960, idleEnabled: true, idleSeconds: 60 })
const running = ref(false)
const listenAddr = ref('')
const sessions = ref<SessionRow[]>([])
const frames = ref<StreamRow[]>([])
const detailFrame = ref<StreamRow | null>(null)
const selectedVin = ref('')
const parserPacks = ref<Array<{ id: string; label: string }>>([])
const cfgOpen = ref(false)

// 每秒 tick:驱动顶栏运行时长与会话在线时长跳动
const now = ref(Date.now())
let tickTimer: number | undefined
// 后端 Status 不含启动时刻,以前端首次观测到运行的时间近似(重启应用即重置)
const startedAt = ref<number | null>(null)
const uptimeText = computed(() =>
  startedAt.value === null ? '-' : fmtDuration(startedAt.value, now.value),
)

let frameSeq = 0
let offs: Array<() => void> = []

type StreamRowInput = Partial<Omit<StreamRow, 'id' | 'time' | 'kind' | 'dir'>> & {
  time?: string
  kind: StreamRow['kind']
  dir?: StreamRow['dir']
}

function pushRow(p: StreamRowInput) {
  frames.value = [
    ...frames.value,
    {
      vin: '', cmd: '', hex: '', summary: '', dir: 'rx' as StreamRow['dir'],
      ...p,
      id: ++frameSeq,
      time: p.time ?? new Date().toISOString(),
    },
  ].slice(-RENDER_CAP)
}

function upsertSession(row: SessionRow) {
  const i = sessions.value.findIndex((s) => s.vin === row.vin)
  if (i >= 0) sessions.value.splice(i, 1, row)
  else sessions.value = [row, ...sessions.value].slice(0, RENDER_CAP)
}

function subscribe() {
  offs = [
    EventsOn('server:status', (st: { running: boolean; listenAddr: string }) => {
      running.value = st.running
      listenAddr.value = st.listenAddr ?? ''
      if (st.running && startedAt.value === null) startedAt.value = Date.now()
      if (!st.running) startedAt.value = null
    }),
    // 上下线事件合成紫色 Link 行插入报文流(VIN + peer)
    EventsOn('server:session', (e: { vin: string; peer: string; online: boolean }) => {
      const nowIso = new Date().toISOString()
      const i = sessions.value.findIndex((s) => s.vin === e.vin)
      if (e.online) {
        // 新连接:重置计数,loginAt 取事件到达时刻(Sessions 快照才有真实 loginAt)
        upsertSession({
          vin: e.vin, peer: e.peer, online: true,
          loginAt: nowIso, lastSeen: nowIso, rxCount: 0, txCount: 0,
        })
        pushRow({ kind: 'link', dir: 'link', vin: e.vin, peer: e.peer, summary: `客户端上线 ${e.peer}` })
      } else {
        // 离线会话保留显示(灰化),报文仍可按其 VIN 过滤
        if (i >= 0) {
          const s = sessions.value[i]
          sessions.value.splice(i, 1, { ...s, online: false, lastSeen: nowIso })
        }
        pushRow({ kind: 'link', dir: 'link', vin: e.vin, peer: e.peer, summary: `客户端离线 ${e.peer}` })
      }
    }),
    EventsOn(
      'server:frame',
      (e: {
        time: string; vin: string; cmd: string; hex: string; summary: string
        kind: StreamRow['kind']; unauthed?: boolean; dir?: string
      }) => {
        const dir: StreamRow['dir'] = e.dir === 'tx' ? 'tx' : 'rx'
        pushRow({
          time: e.time, vin: e.vin, cmd: e.cmd, hex: e.hex, summary: e.summary,
          kind: e.kind, dir, unauthed: e.unauthed,
        })
        // 会话 RX/TX 计数与最后活跃联动
        if (e.vin) {
          const i = sessions.value.findIndex((s) => s.vin === e.vin)
          if (i >= 0) {
            const s = sessions.value[i]
            sessions.value.splice(i, 1, {
              ...s,
              lastSeen: e.time,
              rxCount: s.rxCount + (dir === 'rx' ? 1 : 0),
              txCount: s.txCount + (dir === 'tx' ? 1 : 0),
            })
          }
        }
      },
    ),
    // server:warn → 红色 Error 行
    EventsOn('server:warn', (e: { note: string; hex?: string }) => {
      pushRow({ kind: 'warn', dir: 'link', hex: e.hex ?? '', summary: e.note })
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

async function start(): Promise<boolean> {
  const loopback = cfg.ip === '127.0.0.1' || cfg.ip === 'localhost'
  const doStart = async (force: boolean): Promise<boolean> => {
    try {
      const st = await ServerService.Start({ ...cfg }, force)
      if (st.running) {
        message.success('服务已启动 ' + st.listenAddr)
        return true
      }
      return false
    } catch (e) {
      message.error(String(e))
      return false
    }
  }
  if (loopback) return doStart(false)
  return new Promise((resolve) => {
    Modal.confirm({
      title: '监听地址非环回',
      content: `即将监听 ${cfg.ip}:${cfg.port},局域网内任何设备都可连接(协议无认证)。确认继续?`,
      okText: '继续监听',
      cancelText: '取消',
      onOk: async () => resolve(await doStart(true)),
      onCancel: () => resolve(false),
    })
  })
}

async function startFromDrawer() {
  if (await start()) cfgOpen.value = false
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

// 清空:前端缓冲 + 后端导出环形缓冲;后端失败忽略(spec §6)
async function clearLog() {
  frames.value = []
  detailFrame.value = null
  try {
    await ServerService.ClearLog()
  } catch {
    /* 忽略 */
  }
}

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

// ---------- 选中与会话过滤 ----------
function onSelectSession(vin: string) {
  selectedVin.value = selectedVin.value === vin ? '' : vin
}

const streamFrames = computed<StreamRow[]>(() =>
  selectedVin.value ? frames.value.filter((f) => f.vin === selectedVin.value) : frames.value,
)

function onSelectFrame(r: StreamRow) {
  detailFrame.value = detailFrame.value?.id === r.id ? null : r
}

// ---------- 上下拖拽:报文流 ↔ 报文详情(ResizableDivider,ClientSimulatorPage 同款) ----------
const mainH = ref<number | null>(null) // null = 报文流自适应,详情区默认高
function onDividerDrag(topPx: number) {
  mainH.value = topPx
}

onMounted(async () => {
  tickTimer = window.setInterval(() => (now.value = Date.now()), 1000)
  subscribe()
  await loadCfg()
  const st = await ServerService.Status().catch(() => null)
  if (st?.running) {
    running.value = true
    listenAddr.value = st.listenAddr
    startedAt.value = Date.now()
  }
  // 快照行全部是活会话(注册表只存在线会话),补 online: true(评审 m3)
  const snap = (await ServerService.Sessions().catch(() => [])) ?? []
  sessions.value = snap.map((s: Omit<SessionRow, 'online'>) => ({ ...s, online: true }))
  parserPacks.value = (await ParserService.ParserPacks().catch(() => [])) ?? []
})

onUnmounted(() => {
  if (tickTimer !== undefined) clearInterval(tickTimer)
  offs.forEach((off) => off())
})
</script>

<template>
  <div class="server-page">
    <!-- 顶部紧凑服务控制栏:状态 + 关键指标 + 操作 -->
    <header class="ctrl-bar">
      <span class="dot" :class="{ on: running }" />
      <span class="st-text" :class="{ on: running }">{{ running ? '运行中' : '已停止' }}</span>
      <span class="proto-tag">TCP</span>
      <span class="listen mono">{{ listenAddr || '未监听' }}</span>
      <span class="stat">客户端 <b>{{ sessions.length }}</b></span>
      <span class="stat">报文 <b>{{ frames.length }}</b></span>
      <span class="stat">运行 <b>{{ uptimeText }}</b></span>
      <div class="spacer" />
      <a-button v-if="!running" type="primary" size="small" @click="start">
        <template #icon><CaretRightOutlined /></template>
        启动服务
      </a-button>
      <a-button v-else danger size="small" @click="stop">
        <template #icon><StopOutlined /></template>
        停止服务
      </a-button>
      <a-button size="small" @click="clearLog">
        <template #icon><ClearOutlined /></template>
        清空日志
      </a-button>
      <a-button size="small" @click="exportLog">
        <template #icon><DownloadOutlined /></template>
        导出日志
      </a-button>
      <a-button size="small" @click="cfgOpen = true">
        <template #icon><SettingOutlined /></template>
        服务配置
      </a-button>
    </header>

    <!-- 主体:左会话 / 中报文流,底部报文详情(上下可拖) -->
    <div class="workbench">
      <div
        class="main-row"
        :class="{ fixed: mainH !== null }"
        :style="mainH !== null ? { height: mainH + 'px' } : undefined"
      >
        <SessionList
          :sessions="sessions"
          :selected-vin="selectedVin"
          :now="now"
          @select="onSelectSession"
        />
        <PacketStream
          :frames="streamFrames"
          :running="running"
          :listen-addr="listenAddr"
          :session-count="sessions.length"
          :selected-id="detailFrame?.id ?? null"
          @select="onSelectFrame"
          @start="start"
        />
      </div>
      <ResizableDivider :min-px="150" @drag="onDividerDrag" />
      <div class="detail-row" :class="{ grown: mainH !== null }">
        <PacketDetail :frame="detailFrame" :parser-packs="parserPacks" />
      </div>
    </div>

    <!-- 服务配置抽屉:Listen/空闲设置不再常驻顶栏 -->
    <a-drawer v-model:open="cfgOpen" title="服务配置" placement="right" :width="360">
      <div class="cfg-form">
        <div class="cfg-item">
          <span class="form-label">Listen IP</span>
          <a-input v-model:value="cfg.ip" size="small" placeholder="127.0.0.1" />
        </div>
        <div class="cfg-item">
          <span class="form-label">Listen Port</span>
          <a-input-number v-model:value="cfg.port" size="small" :min="1" :max="65535" class="cfg-num" />
        </div>
        <div class="cfg-item">
          <span class="form-label">空闲断开</span>
          <a-switch v-model:checked="cfg.idleEnabled" size="small" />
        </div>
        <div class="cfg-item">
          <span class="form-label">空闲秒数</span>
          <a-input-number
            v-model:value="cfg.idleSeconds" size="small" :min="5" :max="3600"
            class="cfg-num" :disabled="!cfg.idleEnabled"
          />
        </div>
      </div>
      <a-alert
        v-if="running"
        type="info"
        show-icon
        message="服务运行中:修改空闲断开设置将立即下发生效"
        class="cfg-hint"
      />
      <a-alert
        v-else
        type="info"
        show-icon
        message="启动后客户端可连接此地址上报报文"
        class="cfg-hint"
      />
      <div class="cfg-actions">
        <a-button v-if="!running" type="primary" size="small" @click="startFromDrawer">
          <template #icon><CaretRightOutlined /></template>
          启动服务
        </a-button>
        <a-button v-else danger size="small" @click="stop">
          <template #icon><StopOutlined /></template>
          停止服务
        </a-button>
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.server-page {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden; /* 满高工作台:顶栏 + 主体,主体内部各自滚动 */
}

.mono {
  font-family: var(--font-mono);
}

/* ---------- 服务控制栏 ---------- */
.ctrl-bar {
  flex: none;
  height: 42px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 14px;
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border-subtle);
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-tertiary);
  flex: none;
}

.dot.on {
  background: var(--success);
  animation: dot-pulse 2s ease-out infinite;
}

@keyframes dot-pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(82, 194, 26, 0.4);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(82, 194, 26, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(82, 194, 26, 0);
  }
}

.st-text {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
}

.st-text.on {
  color: var(--success);
}

.proto-tag {
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 16px;
  padding: 0 6px;
  border: 1px solid var(--border-strong);
  border-radius: 2px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.listen {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.stat {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
}

.stat b {
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.spacer {
  flex: 1;
}

/* ---------- 工作台骨架 ---------- */
.workbench {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.main-row {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.main-row.fixed {
  flex: none; /* 拖拽后高度完全由 px 决定 */
}

.detail-row {
  flex: 0 0 232px; /* 默认高度:容纳字节网格与字段表 */
  min-height: 0;
  display: flex;
  border-top: 1px solid var(--border-subtle);
}

.detail-row.grown {
  flex: 1 1 auto; /* 拖拽后详情区吃剩余空间 */
}

/* ---------- 配置抽屉 ---------- */
.cfg-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-bottom: 16px;
}

.cfg-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.form-label {
  width: 80px;
  flex: none;
  color: var(--text-secondary);
  font-size: 13px;
}

.cfg-item :deep(.ant-input),
.cfg-item :deep(.ant-input-number) {
  flex: 1;
}

.cfg-num {
  width: 100%;
}

.cfg-hint {
  margin-bottom: 16px;
}

.cfg-actions {
  display: flex;
  justify-content: flex-end;
}
</style>
