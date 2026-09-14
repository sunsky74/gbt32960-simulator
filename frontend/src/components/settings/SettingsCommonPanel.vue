<script setup lang="ts">
// 常用:启动行为类设置;除「恢复上次工作区」外均为规划占位。
import { appSettings } from '../../composables/useAppSettings'
import SettingRow from './SettingRow.vue'
import PlannedSettingRows from './PlannedSettingRows.vue'
import type { PlannedRow } from './planned'

// 「规划中」占位行(禁用,仅展示)
const plannedRows: PlannedRow[] = [
  { title: '启动时自动连接上次档案', description: '应用启动后自动使用上次连接档案发起连接', control: 'switch' },
  { title: '自动保存配置', description: '表单修改后自动持久化,无需手动保存', control: 'switch' },
  { title: '启动时检查更新', description: '启动后后台检查新版本并提示', control: 'switch' },
  { title: '显示欢迎页', description: '启动时显示版本说明与快速入口', control: 'switch' },
]
</script>

<template>
  <div class="row-group">
    <SettingRow
      title="启动时恢复上次工作区"
      description="启动后自动打开上次使用的页面(客户端模拟 / 报文解析等)"
    >
      <template #action>
        <a-switch v-model:checked="appSettings.restoreLastPage" size="small" />
      </template>
    </SettingRow>
    <PlannedSettingRows :rows="plannedRows" />
  </div>
</template>

<style scoped>
/* 分组:组内末行去分隔线 */
.row-group :deep(.setting-row:last-child) {
  border-bottom: none;
}
</style>
