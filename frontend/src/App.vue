<script setup lang="ts">
import { computed, ref } from 'vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import SideNav from './components/layout/SideNav.vue'
import { activeNavKey, bottomNavItems, navItems } from './navigation'
import { antdThemes } from './theme'
import { useTheme } from './theme/useTheme'

// 左侧导航展开/收起(持久化到 localStorage)
const NAV_COLLAPSED_KEY = 'app-nav-collapsed'
const navCollapsed = ref(localStorage.getItem(NAV_COLLAPSED_KEY) === '1')
function onNavCollapse(v: boolean) {
  navCollapsed.value = v
  localStorage.setItem(NAV_COLLAPSED_KEY, v ? '1' : '0')
}
const allNavItems = computed(() => [...navItems, ...bottomNavItems])
const activePage = computed(() => allNavItems.value.find((i) => i.key === activeNavKey.value) ?? allNavItems.value[0])

// 统一设计 token:页面 → 面板 → 输入框 三级背景分层,柔和边框,统一状态色。
// Dark 为现有基准(逐字保留),Light 为新增主题;随 themeMode 实时切换。
const { themeMode } = useTheme()
const antdTheme = computed(() => antdThemes[themeMode.value])

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

<style src="./style.css"></style>
