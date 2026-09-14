<script setup lang="ts">
// 「规划中」占位行列表:按传入顺序批量渲染禁用的 SettingRow,
// 行数据由各分类面板提供,渲染结果与手写占位行完全一致。
import SettingRow from './SettingRow.vue'
import { CTRL_W, PLANNED, type PlannedRow } from './planned'

defineProps<{
  rows: PlannedRow[]
}>()
</script>

<template>
  <SettingRow
    v-for="row in rows"
    :key="row.title"
    :title="row.title"
    :description="row.description"
    :badge="PLANNED"
    disabled
  >
    <template #action>
      <a-switch v-if="row.control === 'switch'" size="small" disabled />
      <a-select
        v-else-if="row.control === 'select'"
        size="small"
        disabled
        :value="row.value"
        :style="{ width: CTRL_W }"
      />
      <a-button v-else size="small" disabled>{{ row.value }}</a-button>
    </template>
  </SettingRow>
</template>
