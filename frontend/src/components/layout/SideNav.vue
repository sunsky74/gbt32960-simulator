<script setup lang="ts">
import { computed } from 'vue'
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons-vue'
import { activeNavKey, bottomNavItems, navItems } from '../../navigation'
import ThemeSwitch from '../ThemeSwitch.vue'

defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ (e: 'update:collapsed', v: boolean): void }>()

const allItems = computed(() => [...navItems, ...bottomNavItems])

function onSelect(info: { key: string | number }) {
  activeNavKey.value = String(info.key)
}
</script>

<template>
  <aside class="sidenav" :class="{ collapsed }">
    <div class="sidenav-brand" @click="emit('update:collapsed', !collapsed)">
      <span class="brand-icon">📡</span>
      <span v-if="!collapsed" class="brand-text">32960 工具</span>
      <component
        :is="collapsed ? MenuUnfoldOutlined : MenuFoldOutlined"
        class="collapse-trigger"
      />
    </div>

    <a-menu
      mode="inline"
      :inline-collapsed="collapsed"
      :selected-keys="[activeNavKey]"
      @click="onSelect"
    >
      <a-menu-item v-for="item in allItems" :key="item.key">
        <component :is="item.icon" />
        <span>{{ item.title }}</span>
      </a-menu-item>
    </a-menu>

    <div class="sidenav-bottom">
      <ThemeSwitch />
      <span v-if="!collapsed" class="bottom-label">深 / 浅色</span>
    </div>
  </aside>
</template>

<style scoped>
.sidenav {
  width: 200px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border-right: 1px solid var(--border-subtle);
  transition: width 0.2s ease;
  overflow: hidden;
}

.sidenav.collapsed {
  width: 64px;
}

.sidenav-brand {
  height: 44px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 14px;
  cursor: pointer;
  font-weight: 600;
  font-size: 14px;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  white-space: nowrap;
}

.brand-icon {
  font-size: 16px;
}

.collapse-trigger {
  margin-left: auto;
  color: var(--text-tertiary);
  font-size: 13px;
}

.collapsed .sidenav-brand {
  padding: 0;
  justify-content: center;
}

.collapsed .collapse-trigger {
  display: none;
}

.sidenav-bottom {
  margin-top: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
  white-space: nowrap;
}

.collapsed .sidenav-bottom {
  padding: 12px 0;
  justify-content: center;
}

.bottom-label {
  color: var(--text-tertiary);
  font-size: 12px;
}

.sidenav :deep(.ant-menu) {
  background: transparent;
  border-inline-end: none !important;
  padding-top: 4px;
}
</style>
