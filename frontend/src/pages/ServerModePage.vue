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
import ResizableDividerCol from '../components/layout/ResizableDividerCol.vue'
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

// ---------- 双向分割:左右(Session 面板宽)+ 上下(报文流↔详情) ----------
const workbenchEl = ref<HTMLElement | null>(null)
const MIN_TOP_PX = 220 // 报文流区最小高
const MIN_BOTTOM_PX = 180 // 详情区最小高
const SESS_MAX_PX = 320 // 会话面板宽度上限
const MIN_STREAM_PX = 360 // 报文流区最小宽(右侧不得挤没)
const DIVIDER_PX = 7

const mainH = ref<number | null>(null) // 报文流区高度 px;null = 初始未测量(flex 自适应)
const sessW = ref(240) // 会话面板宽度 px

// 上下拖拽:组件保证 bottom ≥ 180,此处补足 top ≥ 220
function onVSplitDrag(topPx: number) {
  mainH.value = Math.max(topPx, MIN_TOP_PX)
}

function onSessDrag(leftPx: number) {
  sessW.value = leftPx
}

// 窗口缩放后钳制,任一区域不得为 0
function clampMainH() {
  const h = workbenchEl.value?.clientHeight ?? 0
  if (h <= 0 || mainH.value === null) return
  const max = Math.max(MIN_TOP_PX, h - MIN_BOTTOM_PX - DIVIDER_PX)
  mainH.value = Math.min(Math.max(mainH.value, MIN_TOP_PX), max)
}

function clampSessW() {
  const w = workbenchEl.value?.clientWidth ?? 0
  if (w <= 0) return
  const max = Math.min(SESS_MAX_PX, w - MIN_STREAM_PX - DIVIDER_PX)
  if (max > 0) sessW.value = Math.min(sessW.value, max)
}

function onWindowResize() {
  clampMainH()
  clampSessW()
}

// 初始分割:上区 56%(800px 窗口 ≈ 420/330),先于异步数据执行避免首帧比例失衡
function initSplit() {
  const h = workbenchEl.value?.clientHeight ?? 0
  if (h > 0) mainH.value = Math.round(h * 0.56)
  clampMainH()
  clampSessW()
}

onMounted(async () => {
  tickTimer = window.setInterval(() => (now.value = Date.now()), 1000)
  // 先初始化分割与监听:纯浏览器调试(无 window.runtime)时布局仍可用
  initSplit()
  window.addEventListener('resize', onWindowResize)
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
  window.removeEventListener('resize', onWindowResize)
  offs.forEach((off) => off())
})
</script>

<template>
  <div class="server-page">
    <!-- 顶部服务控制栏:品牌位 + 状态 + 关键指标 + 操作(全局 .topbar) -->
    <header class="topbar">
      <span class="brand">服务端模式</span>
      <span class="bar-sep" />
      <span class="server-dot" :class="{ on: running }" />
      <span class="server-status" :class="{ on: running }">{{ running ? '运行中' : '已停止' }}</span>
      <span class="server-proto">TCP</span>
      <span class="server-addr">{{ listenAddr || '未监听' }}</span>
      <span class="server-stat">客户端 <b>{{ sessions.length }}</b></span>
      <span class="server-stat">报文 <b>{{ frames.length }}</b></span>
      <span class="server-stat">运行 <b>{{ uptimeText }}</b></span>
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

    <!-- 主体工作台:左会话 ↔ 右报文流(左右可拖),下方报文详情(上下可拖) -->
    <div ref="workbenchEl" class="server-workbench">
      <div
        class="server-main-row"
        :class="{ fixed: mainH !== null }"
        :style="mainH !== null ? { height: mainH + 'px' } : undefined"
      >
        <SessionList
          :sessions="sessions"
          :selected-vin="selectedVin"
          :now="now"
          :style="{ width: sessW + 'px' }"
          @select="onSelectSession"
        />
        <ResizableDividerCol
          :min-px="200"
          :max-px="SESS_MAX_PX"
          :right-min-px="MIN_STREAM_PX"
          @drag="onSessDrag"
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
      <ResizableDivider class="tight" :min-px="MIN_BOTTOM_PX" @drag="onVSplitDrag" />
      <PacketDetail :frame="detailFrame" :parser-packs="parserPacks" />
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
