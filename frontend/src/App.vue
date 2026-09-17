<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import SideNav from './components/layout/SideNav.vue'
import { activeNavKey, bottomNavItems, navItems } from './navigation'
import { antdThemes } from './theme'
import { useTheme } from './theme/useTheme'
import { message } from 'ant-design-vue'
import * as UpdaterService from '../wailsjs/go/bridge/UpdaterService'
import { appSettings, rememberNavPage, restoreLastNavPage } from './composables/useAppSettings'
import { UPDATE_CHECK_DELAY_MS, lastResultToast, markChecked, shouldAutoCheck, shouldPrompt } from './composables/useUpdater'
import { openSettingsCategory, settingsFocusCategory } from './composables/settingsFocus'

// 左侧导航展开/收起(持久化到 localStorage)
const NAV_COLLAPSED_KEY = 'app-nav-collapsed'
const navCollapsed = ref(localStorage.getItem(NAV_COLLAPSED_KEY) === '1')
function onNavCollapse(v: boolean) {
  navCollapsed.value = v
  localStorage.setItem(NAV_COLLAPSED_KEY, v ? '1' : '0')
}
const allNavItems = computed(() => [...navItems, ...bottomNavItems])

// 启动恢复上次工作区页面(设置「常用 → 启动时恢复上次页面」控制)
onMounted(() => {
  const keys = allNavItems.value.map((i) => i.key)
  const restored = restoreLastNavPage(keys, activeNavKey.value)
  if (restored !== activeNavKey.value) activeNavKey.value = restored
})
watch(activeNavKey, (k) => rememberNavPage(k))

// 跨页跳转:settingsFocusCategory 被写入时切到设置页(具体分类由 SettingsPage 消费)
watch(settingsFocusCategory, (k) => {
  if (k) activeNavKey.value = 'settings'
})

// 启动自动检查:延迟 ~3s、24h 冷却、失败静默(设置「常用 → 启动时检查更新」控制)
onMounted(() => {
  window.setTimeout(async () => {
    const now = Date.now()
    if (!shouldAutoCheck(appSettings, now)) return
    try {
      const info = await UpdaterService.CheckUpdate()
      if (!info.devBuild && info.hasUpdate && shouldPrompt(appSettings, info.latest)) {
        message.info({
          content: `发现新版本 ${info.latest},点击查看`,
          duration: 15,
          onClick: () => openSettingsCategory('about'),
        })
      }
    } catch {
      // 静默失败:不打扰;冷却期内不重试(手动检查始终可用)
    } finally {
      markChecked(now)
    }
  }, UPDATE_CHECK_DELAY_MS)
})

// 启动消费上次更新结果:失败 → 固定文案告知(原因与日志见日志文件);成功或无记录静默
onMounted(async () => {
  try {
    const [res, cur] = await Promise.all([UpdaterService.ConsumeLastResult(), UpdaterService.CurrentVersion()])
    const text = lastResultToast(res, cur)
    if (text) message.error(text)
  } catch {
    // 静默:更新结果消费失败不影响启动
  }
})

const activePage = computed(() => allNavItems.value.find((i) => i.key === activeNavKey.value) ?? allNavItems.value[0])

// 统一设计 token:页面 → 面板 → 输入框 三级背景分层,柔和边框,统一状态色。
// Dark 为现有基准(逐字保留),Light 为新增主题;auto 模式取系统偏好,随 themeMode 实时切换。
const { resolvedMode } = useTheme()
const antdTheme = computed(() => antdThemes[resolvedMode.value])
</script>

<template>
  <a-config-provider :theme="antdTheme" :locale="zhCN">
    <div class="app">
      <SideNav :collapsed="navCollapsed" @update:collapsed="onNavCollapse" />
      <main class="page-content">
        <KeepAlive>
          <component :is="activePage.component" />
        </KeepAlive>
      </main>
    </div>
  </a-config-provider>
</template>
