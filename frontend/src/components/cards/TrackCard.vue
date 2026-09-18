<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import * as TrackService from '../../../wailsjs/go/bridge/TrackService'
import * as MessageService from '../../../wailsjs/go/bridge/MessageService'
import { store } from '../../state'
import CollapsibleCard from '../layout/CollapsibleCard.vue'

interface TrackInfo {
  name: string
  format: string
  count: number
  skipped: number
}

interface ReplayStatus {
  running: boolean
  index: number
  total: number
  loop: boolean
  lastError: string
  lng: number
  lat: number
}

const info = ref<TrackInfo | null>(null)
const importing = ref(false)
const loop = ref(false)
const starting = ref(false)
const status = reactive<ReplayStatus>({
  running: false,
  index: 0,
  total: 0,
  loop: false,
  lastError: '',
  lng: 0,
  lat: 0,
})

const online = computed(() => store.connState === 'online')
const canStart = computed(() => !!info.value && online.value && !status.running)
const percent = computed(() => (status.total > 0 ? Math.round((status.index / status.total) * 100) : 0))
const tagText = computed(() => (status.running ? '回放中' : info.value ? info.value.format.toUpperCase() : '未导入'))

let timer: number | undefined

// 同步周期上报状态:导入/回放会由后端默认开启周期上报,面板开关需跟随。
async function syncReportState() {
  try {
    const st = await MessageService.AutoReportState()
    store.autoReport = st.on
    if (st.intervalSec > 0) store.autoInterval = st.intervalSec
  } catch {
    /* 后端未就绪时静默 */
  }
}

async function importFile() {
  importing.value = true
  try {
    const path = await TrackService.PickTrackFile()
    if (!path) return
    info.value = (await TrackService.ImportTrack(path)) as TrackInfo
    const extra = info.value.skipped > 0 ? `(跳过 ${info.value.skipped} 行无效)` : ''
    message.success(`已导入「${info.value.name}」: ${info.value.count} 个轨迹点 ${extra}`)
    await syncReportState()
    if (store.autoReport) {
      message.info(`周期上报已默认开启 (每 ${store.autoInterval}s 一条,每条推进一个轨迹点)`)
    }
    await pollOnce()
  } catch (e) {
    message.error(String(e))
  } finally {
    importing.value = false
  }
}

async function clearFile() {
  try {
    await TrackService.ClearTrack()
    info.value = null
    await pollOnce()
    message.success('已清空轨迹文件')
  } catch (e) {
    message.error(String(e))
  }
}

async function start() {
  starting.value = true
  try {
    await TrackService.StartReplay(loop.value)
    await syncReportState()
    await pollOnce()
    startPolling()
    message.success(
      `轨迹回放已开始:按周期上报间隔 (每 ${store.autoInterval}s) 逐点上报 0x02${loop.value ? ',播完循环' : ''}`,
    )
  } catch (e) {
    message.error(String(e))
  } finally {
    starting.value = false
  }
}

async function stop() {
  try {
    await TrackService.StopReplay()
  } finally {
    await pollOnce()
    stopPolling()
  }
}

async function pollOnce() {
  Object.assign(status, await TrackService.ReplayStatus())
  store.trackActive = status.running
}

function startPolling() {
  stopPolling()
  timer = window.setInterval(async () => {
    await pollOnce()
    if (!status.running) stopPolling()
  }, 500)
}

function stopPolling() {
  if (timer !== undefined) {
    clearInterval(timer)
    timer = undefined
  }
}

onMounted(async () => {
  try {
    await pollOnce()
    if (status.running) startPolling()
  } catch {
    /* 后端未就绪时静默,首点轮询由操作触发 */
  }
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <CollapsibleCard title="轨迹" :default-open="false">
    <template #extra>
      <a-tag :color="status.running ? 'blue' : undefined">{{ tagText }}</a-tag>
    </template>

    <a-button size="small" block :loading="importing" @click="importFile">
      导入轨迹文件 (.gpx / .xlsx / .csv)
    </a-button>
    <div v-if="info" class="track-file">{{ info.name }} · {{ info.count }} 点</div>

    <div class="track-controls">
      <span class="label">循环</span>
      <a-switch v-model:checked="loop" size="small" :disabled="status.running" />
      <span class="label interval-hint">按周期上报间隔推进</span>
    </div>

    <div class="track-buttons">
      <a-button size="small" type="primary" :disabled="!canStart" :loading="starting" @click="start">
        开始回放
      </a-button>
      <a-button size="small" danger :disabled="!status.running" @click="stop">停止</a-button>
      <a-button size="small" :disabled="!info || status.running" @click="clearFile">清空</a-button>
    </div>

    <template v-if="status.total > 0">
      <a-progress :percent="percent" size="small" status="active" />
      <div class="track-meta">
        {{ status.index }}/{{ status.total }} 点
        <template v-if="status.index > 0"> · {{ status.lng.toFixed(6) }}, {{ status.lat.toFixed(6) }} </template>
      </div>
    </template>
    <div v-if="status.lastError" class="track-error">{{ status.lastError }}</div>

    <div class="track-hint">
      回放搭载周期上报:每个周期 0x02 携带下一个轨迹点(速度由实时数据页的周期上报间隔决定);
      期间位置数据锁定不可编辑,其余数据保持现有配置;Excel/CSV 每行一个"经度,纬度"
    </div>
  </CollapsibleCard>
</template>

<style scoped>
.track-controls {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
  font-size: var(--fs-12);
}

.track-controls .label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.track-controls .interval-hint {
  opacity: 0.55;
}

.track-buttons {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.track-buttons .ant-btn {
  flex: 1;
}

.track-meta {
  font-size: var(--fs-12);
  opacity: 0.75;
}

.track-error {
  margin-top: 4px;
  font-size: var(--fs-12);
  color: var(--error);
}

.track-hint {
  margin-top: 8px;
  font-size: var(--fs-11);
  line-height: 1.5;
  opacity: 0.55;
}
</style>
