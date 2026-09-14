<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import SideNav from './components/layout/SideNav.vue'
import { activeNavKey, bottomNavItems, navItems } from './navigation'
import { antdThemes } from './theme'
import { useTheme } from './theme/useTheme'
import { rememberNavPage, restoreLastNavPage } from './composables/useAppSettings'

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
