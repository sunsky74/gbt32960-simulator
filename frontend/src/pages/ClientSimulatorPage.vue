<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import RealTimePanel from '../components/RealTimePanel.vue'
import ConsolePanel from '../components/ConsolePanel.vue'
import ResizableDivider from '../components/layout/ResizableDivider.vue'
import ConnectionCard from '../components/cards/ConnectionCard.vue'
import StateCard from '../components/cards/StateCard.vue'
import AlarmCard from '../components/cards/AlarmCard.vue'
import VehicleCard from '../components/cards/VehicleCard.vue'
import TrackCard from '../components/cards/TrackCard.vue'
import {
  loadInitialData,
  loadProfiles,
  pushConsoleEvents,
  refreshState,
  reloadSchemaForVersion,
  reloadSchemaPreservingGroups,
  store,
} from '../state'
import * as ConnectionService from '../../wailsjs/go/bridge/ConnectionService'
import { onConsoleEvents } from '../api/backend'
import { initConnConfig, resetToNewProfile } from '../composables/useConnConfig'
import { connect, connectDisabled, testConnect, testing } from '../composables/useConnActions'

initConnConfig()

// ---------- 客户端模拟页专属顶栏(连接控制) ----------
let offEvents: (() => void) | null = null
let stateTimer: number | undefined
let profileTimer: number | undefined

const statusBadge = computed(() => {
  switch (store.connState) {
    case 'online':
      return { status: 'success', text: '在线' } as const
    case 'connecting':
    case 'loggingIn':
      return { status: 'processing', text: '连接中' } as const
    default:
      return { status: 'default', text: '离线' } as const
  }
})

const activeProfileName = computed(() => store.config?.name ?? '')

async function onSwitchProfile(name: string) {
  try {
    const prevVersion = store.config?.version
    const prevPack = store.config?.extensionPack
    store.config = await ConnectionService.SwitchProfile(name)
    await loadProfiles()
    if (store.config.version !== prevVersion) {
      await reloadSchemaForVersion(store.config.version)
    } else if (store.config.extensionPack !== prevPack) {
      await reloadSchemaPreservingGroups(store.config.version)
    }
  } catch (e) {
    console.error('切换档案失败', e)
  }
}

function onNewProfile() {
  resetToNewProfile(store.profiles.length + 1)
  void loadProfiles()
}

async function disconnect() {
  await ConnectionService.Disconnect()
  await refreshState()
}

// 左侧上下 Panel 可拖拽高度(像素级,窗口缩放时自动钳制)
const MIN_PANEL_PX = 160
const leftEl = ref<HTMLElement | null>(null)
const topHeight = ref(0)

// .left 的可用内容高度(扣除自身 padding 与分割条高度)
function leftContentHeight(): number {
  const el = leftEl.value
  if (!el || el.clientHeight <= 0) return 0
  const cs = getComputedStyle(el)
  const padTop = parseFloat(cs.paddingTop) || 0
  const padBottom = parseFloat(cs.paddingBottom) || 0
  const divider = el.querySelector('.resizable-divider') as HTMLElement | null
  return el.clientHeight - padTop - padBottom - (divider?.offsetHeight ?? 9)
}

function clampTopHeight() {
  const h = leftContentHeight()
  if (h <= 0) return
  const max = h - MIN_PANEL_PX
  topHeight.value = Math.min(Math.max(topHeight.value, MIN_PANEL_PX), max)
}

function onDividerDrag(px: number) {
  topHeight.value = px
}

function onWindowResize() {
  clampTopHeight()
}

onMounted(async () => {
  // 初始高度:上区 55%,下区 45%
  topHeight.value = Math.round(leftContentHeight() * 0.55)
  clampTopHeight()
  window.addEventListener('resize', onWindowResize)

  offEvents = onConsoleEvents((batch) => pushConsoleEvents(batch))
  await loadInitialData()
  stateTimer = window.setInterval(() => {
    void refreshState()
  }, 1000)
  profileTimer = window.setInterval(() => {
    void loadProfiles()
  }, 5000)
})

onUnmounted(() => {
  window.removeEventListener('resize', onWindowResize)
  offEvents?.()
  if (stateTimer) window.clearInterval(stateTimer)
  if (profileTimer) window.clearInterval(profileTimer)
})
</script>

<template>
  <div class="client-page">
    <header class="topbar">
      <a-select
        v-model:value="activeProfileName"
        class="profile-select"
        size="small"
        :options="
          store.profiles.map((p) => ({
            value: p.name,
            label: `${p.name}  (${p.host}:${p.port})`,
          }))
        "
        @change="onSwitchProfile"
      />
      <a-button size="small" @click="onNewProfile"><PlusOutlined /> 新建连接</a-button>
      <a-badge :status="statusBadge.status" :text="statusBadge.text" />
      <div class="spacer" />
      <a-button size="small" :loading="testing" @click="testConnect">测试连接</a-button>
      <a-button size="small" type="primary" :loading="store.connBusy" :disabled="connectDisabled" @click="connect">
        连接并登录
      </a-button>
      <a-button size="small" danger :disabled="store.connState === 'idle'" @click="disconnect">断开连接</a-button>
    </header>

    <main class="layout">
    <section ref="leftEl" class="left">
      <div class="left-top" :style="{ height: topHeight + 'px' }">
        <RealTimePanel />
      </div>
      <ResizableDivider :min-px="MIN_PANEL_PX" @drag="onDividerDrag" />
      <div class="left-bottom">
        <ConsolePanel />
      </div>
    </section>

    <aside class="right">
      <ConnectionCard />
      <StateCard />
      <AlarmCard />
      <VehicleCard />
      <TrackCard />
    </aside>
    </main>
  </div>
</template>

<style scoped>
.client-page {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
</style>
