<script setup lang="ts">
import { computed, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { CloseOutlined, PlusOutlined } from '@ant-design/icons-vue'
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
  savedBinding,
  store,
  syncSavedBinding,
} from '../state'
import * as ConnectionService from '../../wailsjs/go/bridge/ConnectionService'
import { onConsoleEvents } from '../api/events'
import { initConnConfig, resetToNewProfile } from '../composables/useConnConfig'
import { connect, connectDisabled, testConnect, testing } from '../composables/useConnActions'

initConnConfig()

// ---------- 客户端模拟页专属顶栏(连接控制) ----------
let offEvents: (() => void) | null = null
let stateTimer: number | undefined
let profileTimer: number | undefined

function subscribeEvents() {
  // 先清理旧订阅,保证 activate 循环中重复调用也不会双重订阅
  unsubscribeEvents()
  offEvents = onConsoleEvents((batch) => pushConsoleEvents(batch))
}

function unsubscribeEvents() {
  offEvents?.()
  offEvents = null
}

function startTimers() {
  stopTimers()
  stateTimer = window.setInterval(() => {
    void refreshState()
  }, 1000)
  profileTimer = window.setInterval(() => {
    void loadProfiles()
  }, 5000)
}

function stopTimers() {
  if (stateTimer !== undefined) {
    window.clearInterval(stateTimer)
    stateTimer = undefined
  }
  if (profileTimer !== undefined) {
    window.clearInterval(profileTimer)
    profileTimer = undefined
  }
}

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

// 下拉框数据驱动:闭框时选中项显示 label(纯文本);展开项由 #option 插槽
// 自定义渲染(文案 + 删除按钮),× 不会出现在收起状态。
const profileOptions = computed(() =>
  store.profiles.map((p) => ({
    value: p.name,
    label: `${p.name}  (${p.host}:${p.port})`,
    name: p.name,
    host: p.host,
    port: p.port,
  })),
)

async function onSwitchProfile(name: string) {
  if (store.connBusy) return
  try {
    store.config = await ConnectionService.SwitchProfile(name)
    await loadProfiles()
    if (store.config.version !== savedBinding.version) {
      await reloadSchemaForVersion(store.config.version)
    } else if (store.config.extensionPack !== savedBinding.pack) {
      await reloadSchemaPreservingGroups(store.config.version)
    }
    syncSavedBinding(store.config)
  } catch (e) {
    console.error('切换档案失败', e)
  }
}

function onNewProfile() {
  resetToNewProfile(store.profiles.length + 1)
  void loadProfiles()
}

// 下拉框选项内的 ×:弹确认框,确认后删除该档案(删激活档案须先断开)。
function confirmDeleteProfile(name: string) {
  if ((store.connBusy || store.connState !== 'idle') && name === activeProfileName.value) {
    message.warning('该档案正在连接中,请先断开连接再删除')
    return
  }
  Modal.confirm({
    title: '删除连接档案',
    content: `将删除「${name}」及其全部配置,不可恢复`,
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    onOk: () => doDeleteProfile(name),
  })
}

async function doDeleteProfile(name: string) {
  try {
    await ConnectionService.DeleteProfile(name)
    // 删除的是激活档案:配置切到剩余第一条(表单/版本/扩展包联动重载);
    // 删的是其他档案:仅刷新列表,当前配置不动。
    if (store.config?.name === name) {
      store.config = await ConnectionService.GetConfig()
      if (store.config.version !== savedBinding.version) {
        await reloadSchemaForVersion(store.config.version)
      } else if (store.config.extensionPack !== savedBinding.pack) {
        await reloadSchemaPreservingGroups(store.config.version)
      }
      syncSavedBinding(store.config)
    } else {
      await loadProfiles()
    }
    message.success(`已删除「${name}」`)
  } catch (e) {
    message.error('删除失败: ' + String(e))
  }
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
  return el.clientHeight - padTop - padBottom - (divider?.offsetHeight ?? 12)
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

// ---------- KeepAlive 生命周期 ----------
// App.vue 用的是无 include 的 <KeepAlive>:切页只触发 onDeactivated/onActivated,
// onUnmounted 不会执行。因此控制台订阅与两个定时器全部收口到 onActivated 这一条
// 路径(onMounted 不订阅),onDeactivated 时暂停,onUnmounted 仅兜底清理;
// activate 前先清理旧订阅/旧定时器,保证多次切换不泄漏、不重复。
// 取舍:停用期间到达的 console:events 不做前端缓冲,Go 侧仍保留 500 行导出
// 环形缓冲,重新激活时以 refreshState/loadProfiles 快照补齐连接与档案状态。

onMounted(async () => {
  // 初始高度:上区 55%,下区 45%
  topHeight.value = Math.round(leftContentHeight() * 0.55)
  clampTopHeight()
  window.addEventListener('resize', onWindowResize)
  await loadInitialData()
})

onActivated(() => {
  // KeepAlive 首次挂载也会触发 onActivated:订阅只在此建立,天然只有一份
  subscribeEvents()
  startTimers()
  void refreshState()
  void loadProfiles()
})

onDeactivated(() => {
  unsubscribeEvents()
  stopTimers()
})

onUnmounted(() => {
  window.removeEventListener('resize', onWindowResize)
  unsubscribeEvents()
  stopTimers()
})
</script>

<template>
  <div class="client-page">
    <header class="topbar">
      <a-select
        v-model:value="activeProfileName"
        class="profile-select"
        size="small"
        :disabled="store.connBusy"
        :options="profileOptions"
        @change="onSwitchProfile"
      >
        <template #option="{ name, host, port }">
          <span class="profile-option">
            <span class="profile-option-label">{{ name }} ({{ host }}:{{ port }})</span>
            <a-button
              type="text"
              size="small"
              shape="circle"
              class="profile-del-btn"
              title="删除该档案"
              @click.stop="confirmDeleteProfile(name)"
            >
              <CloseOutlined />
            </a-button>
          </span>
        </template>
      </a-select>
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

.profile-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.profile-option-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-del-btn.profile-del-btn {
  flex-shrink: 0;
  margin-right: -4px;
  opacity: 0.35; /* 默认淡色弱化,hover 才点亮淡红(双类名提升优先级压过 antd 按钮样式) */
}

.profile-del-btn.profile-del-btn:hover {
  opacity: 1;
  color: #ffa39e; /* 淡红:antd red-3,比原 red-4 更浅 */
}
</style>
