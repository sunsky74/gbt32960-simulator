<script setup lang="ts">
import { computed, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  CaretRightOutlined,
  ClearOutlined,
  DownloadOutlined,
  SendOutlined,
  SettingOutlined,
  StopOutlined,
} from '@ant-design/icons-vue'
import * as ServerService from '../../wailsjs/go/bridge/ServerService'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import { onWailsEvent } from '../api/events'
import ResizableDivider from '../components/layout/ResizableDivider.vue'
import ResizableDividerCol from '../components/layout/ResizableDividerCol.vue'
import SessionList from '../components/servermode/SessionList.vue'
import PacketStream from '../components/servermode/PacketStream.vue'
import PacketDetail from '../components/servermode/PacketDetail.vue'
import ServerConfigDrawer from '../components/servermode/ServerConfigDrawer.vue'
import ExtCommandModal from '../components/servermode/ExtCommandModal.vue'
import { RENDER_CAP, fmtDuration, type SessionRow, type StreamRow } from '../components/servermode/types'
import { appSettings } from '../composables/useAppSettings'

const running = ref(false)
const listenAddr = ref('')
const sessions = ref<SessionRow[]>([])
const frames = ref<StreamRow[]>([])
const detailFrame = ref<StreamRow | null>(null)
const selectedVin = ref('')
const parserPacks = ref<Array<{ id: string; label: string }>>([])

// ---------- 子组件句柄:配置抽屉自持 cfg/启停动作,下发弹窗自持命令列表 ----------
const cfgDrawerRef = ref<InstanceType<typeof ServerConfigDrawer> | null>(null)
const extCmdModalRef = ref<InstanceType<typeof ExtCommandModal> | null>(null)

// 顶栏启停委托给配置抽屉(cfg 表单状态由抽屉自持,启停共用同一实现)
function startServer() {
  void cfgDrawerRef.value?.start()
}

function stopServer() {
  void cfgDrawerRef.value?.stop()
}

function openCfgDrawer() {
  cfgDrawerRef.value?.open()
}

function openExtCmdModal() {
  extCmdModalRef.value?.open()
}

// 每秒 tick:驱动顶栏运行时长与会话在线时长跳动
const now = ref(Date.now())
let tickTimer: number | undefined
// 后端 Status 不含启动时刻,以前端首次观测到运行的时间近似(重启应用即重置)
const startedAt = ref<number | null>(null)
const uptimeText = computed(() => (startedAt.value === null ? '-' : fmtDuration(startedAt.value, now.value)))

let frameSeq = 0
let offs: Array<() => void> = []

function unsubscribe() {
  offs.forEach((off) => off())
  offs = []
}

type StreamRowInput = Partial<Omit<StreamRow, 'id' | 'time' | 'kind' | 'dir'>> & {
  time?: string
  kind: StreamRow['kind']
  dir?: StreamRow['dir']
}

// 报文流渲染上限:设置页「外观 → 服务端报文流保留行数」可调,缺省回落 RENDER_CAP(默认行为不变)
const renderCap = computed(() => appSettings.packetStreamCap || RENDER_CAP)

function pushRow(p: StreamRowInput) {
  frames.value = [
    ...frames.value,
    {
      vin: '',
      cmd: '',
      hex: '',
      summary: '',
      dir: 'rx' as StreamRow['dir'],
      ...p,
      id: ++frameSeq,
      time: p.time ?? new Date().toISOString(),
    },
  ].slice(-renderCap.value)
}

function upsertSession(row: SessionRow) {
  const i = sessions.value.findIndex((s) => s.vin === row.vin)
  if (i >= 0) sessions.value.splice(i, 1, row)
  else sessions.value = [row, ...sessions.value].slice(0, renderCap.value)
}

function subscribe() {
  // 先清理旧订阅,保证 activate 循环中重复调用也不会双重订阅
  unsubscribe()
  offs = [
    onWailsEvent('server:status', (st) => {
      running.value = st.running
      listenAddr.value = st.listenAddr ?? ''
      if (st.running && startedAt.value === null) startedAt.value = Date.now()
      if (!st.running) startedAt.value = null
    }),
    // 上下线事件合成紫色 Link 行插入报文流(VIN + peer)
    onWailsEvent('server:session', (e) => {
      const nowIso = new Date().toISOString()
      const i = sessions.value.findIndex((s) => s.vin === e.vin)
      if (e.online) {
        // 新连接:重置计数,loginAt 取事件到达时刻(Sessions 快照才有真实 loginAt)
        upsertSession({
          vin: e.vin,
          peer: e.peer,
          online: true,
          platform: e.platform,
          loginAt: nowIso,
          lastSeen: nowIso,
          rxCount: 0,
          txCount: 0,
        })
        pushRow({
          kind: 'link',
          dir: 'link',
          vin: e.vin,
          peer: e.peer,
          summary: `${e.platform ? '平台' : '客户端'}上线 ${e.peer}`,
          platform: e.platform,
        })
      } else {
        // 离线会话保留显示(灰化),报文仍可按其 VIN 过滤
        if (i >= 0) {
          const s = sessions.value[i]
          sessions.value.splice(i, 1, { ...s, online: false, lastSeen: nowIso })
        }
        pushRow({ kind: 'link', dir: 'link', vin: e.vin, peer: e.peer, summary: `客户端离线 ${e.peer}` })
      }
    }),
    onWailsEvent('server:frame', (e) => {
      const dir: StreamRow['dir'] = e.dir === 'tx' ? 'tx' : 'rx'
      pushRow({
        time: e.time,
        vin: e.vin,
        cmd: e.cmd,
        hex: e.hex,
        summary: e.summary,
        kind: e.kind,
        dir,
        unauthed: e.unauthed,
        platform: e.platform,
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
    }),
    // server:warn → 红色 Error 行
    onWailsEvent('server:warn', (e) => {
      pushRow({ kind: 'warn', dir: 'link', hex: e.hex, summary: e.note })
    }),
  ]
}

function startTick() {
  stopTick()
  tickTimer = window.setInterval(() => (now.value = Date.now()), 1000)
}

function stopTick() {
  if (tickTimer !== undefined) {
    window.clearInterval(tickTimer)
    tickTimer = undefined
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
const MIN_TOP_PX = 260 // 报文流区最小高
const MIN_BOTTOM_PX = 180 // 详情区最小高(另受"下方≤60%"约束)
const SESS_MAX_PX = 400 // 会话面板宽度上限
const MIN_STREAM_PX = 360 // 报文流区最小宽(右侧不得挤没)
const DIVIDER_PX = 12 // 与统一分割条 12px 判定区同步(ui-design-reference §8)

const mainH = ref<number | null>(null) // 报文流区高度 px;null = 初始未测量(flex 自适应)
const sessW = ref(240) // 会话面板宽度 px

// 上下拖拽:组件保证 bottom ≥ 180,此处补足 top ≥ 260 与 top ≥ 40%(即下方 ≤ 60%)
function onVSplitDrag(topPx: number) {
  const h = workbenchEl.value?.clientHeight ?? 0
  const min = h > 0 ? Math.max(MIN_TOP_PX, Math.round(h * 0.4)) : MIN_TOP_PX
  mainH.value = Math.max(topPx, min)
}

function onSessDrag(leftPx: number) {
  sessW.value = leftPx
}

// 窗口缩放后钳制,任一区域不得为 0
function clampMainH() {
  const h = workbenchEl.value?.clientHeight ?? 0
  if (h <= 0 || mainH.value === null) return
  // 上限 = 详情区保底(下方 ≤ 60% 由下限侧共同保证)
  const max = Math.max(MIN_TOP_PX, h - MIN_BOTTOM_PX - DIVIDER_PX)
  // 下限 = 报文流保底 与 40%(下方不得超过 60%) 取大
  const min = Math.max(MIN_TOP_PX, Math.round((h - DIVIDER_PX) * 0.4))
  mainH.value = Math.min(Math.max(mainH.value, min), Math.max(min, max))
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

// 初始分割:上区 60% / 下区 40%,先于异步数据执行避免首帧比例失衡
function initSplit() {
  const h = workbenchEl.value?.clientHeight ?? 0
  if (h > 0) mainH.value = Math.round((h - DIVIDER_PX) * 0.6)
  clampMainH()
  clampSessW()
}

// ---------- KeepAlive 生命周期 ----------
// App.vue 用的是无 include 的 <KeepAlive>:切页只触发 onDeactivated/onActivated,
// onUnmounted 不会执行。因此事件订阅与 tickTimer 全部收口到 onActivated 这一条
// 路径(onMounted 不订阅),onDeactivated 时暂停,onUnmounted 仅兜底清理;
// activate 前先清理旧订阅/旧定时器,保证多次切换不泄漏、不重复。
// 取舍:停用期间到达的 server:frame 等事件不在前端缓存(高频帧缓存无意义),
// Go 侧仍持有 500 行导出环形缓冲与 Sessions 快照,重新激活时以快照补齐,
// 停用窗口内的报文流会有缺口。

async function resyncSnapshots() {
  const st = await ServerService.Status().catch(() => null)
  if (st) {
    running.value = st.running
    listenAddr.value = st.running ? st.listenAddr : ''
    if (st.running && startedAt.value === null) startedAt.value = Date.now()
    if (!st.running) startedAt.value = null
  }
  // 快照行全部是活会话(注册表只存在线会话),补 online: true(评审 m3)
  const snap = (await ServerService.Sessions().catch(() => [])) ?? []
  sessions.value = snap.map((s: Omit<SessionRow, 'online'>) => ({ ...s, online: true }))
}

onMounted(async () => {
  // 先初始化分割与监听:纯浏览器调试(无 window.runtime)时布局仍可用
  initSplit()
  window.addEventListener('resize', onWindowResize)
  // 已保存服务配置由 ServerConfigDrawer 挂载时自行加载
  parserPacks.value = (await ParserService.ParserPacks().catch(() => [])) ?? []
})

onActivated(() => {
  // KeepAlive 首次挂载也会触发 onActivated:订阅只在此建立,天然只有一份
  startTick()
  subscribe()
  void resyncSnapshots()
})

onDeactivated(() => {
  stopTick()
  unsubscribe()
})

onUnmounted(() => {
  stopTick()
  unsubscribe()
  window.removeEventListener('resize', onWindowResize)
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
      <span class="server-stat"
        >客户端 <b>{{ sessions.length }}</b></span
      >
      <span class="server-stat"
        >报文 <b>{{ frames.length }}</b></span
      >
      <span class="server-stat"
        >运行 <b>{{ uptimeText }}</b></span
      >
      <div class="spacer" />
      <a-button v-if="!running" type="primary" size="small" @click="startServer">
        <template #icon><CaretRightOutlined /></template>
        启动服务
      </a-button>
      <a-button v-else danger size="small" @click="stopServer">
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
      <a-button size="small" @click="openCfgDrawer">
        <template #icon><SettingOutlined /></template>
        服务配置
      </a-button>
      <a-button size="small" :disabled="!running || !selectedVin" @click="openExtCmdModal">
        <template #icon><SendOutlined /></template>
        下发命令
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
        <ResizableDividerCol :min-px="200" :max-px="SESS_MAX_PX" :right-min-px="MIN_STREAM_PX" @drag="onSessDrag" />
        <PacketStream
          :frames="streamFrames"
          :running="running"
          :listen-addr="listenAddr"
          :session-count="sessions.length"
          :selected-id="detailFrame?.id ?? null"
          @select="onSelectFrame"
          @start="startServer"
        />
      </div>
      <ResizableDivider :min-px="MIN_BOTTOM_PX" @drag="onVSplitDrag" />
      <PacketDetail :frame="detailFrame" :parser-packs="parserPacks" />
    </div>

    <!-- 平台下发:扩展包 down 命令模板 -->
    <ExtCommandModal ref="extCmdModalRef" :selected-vin="selectedVin" />

    <!-- 服务配置抽屉:Listen/空闲设置不再常驻顶栏 -->
    <ServerConfigDrawer ref="cfgDrawerRef" :running="running" />
  </div>
</template>
