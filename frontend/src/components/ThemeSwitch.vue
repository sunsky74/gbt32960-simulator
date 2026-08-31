<script setup lang="ts">
import { toggleTheme, useTheme } from '../theme/useTheme'

const { themeMode } = useTheme()
</script>

<template>
  <a-tooltip :title="themeMode === 'dark' ? '深色主题(点击切换浅色)' : '浅色主题(点击切换深色)'">
    <a-switch
      size="small"
      class="theme-switch"
      :checked="themeMode === 'dark'"
      @change="toggleTheme"
    >
      <template #checkedChildren>🌙</template>
      <template #unCheckedChildren>☀️</template>
    </a-switch>
  </a-tooltip>
</template>

<style scoped>
.theme-switch {
  flex-shrink: 0;
}
</style>
