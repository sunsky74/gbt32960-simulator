<script setup lang="ts">
// 报文解析:解析页默认行为预留配置,当前全部为规划占位。
import PlannedSettingRows from './PlannedSettingRows.vue'
import type { PlannedRow } from './planned'

// 「规划中」占位行(禁用,仅展示)
const plannedRows: PlannedRow[] = [
  { title: '默认协议版本', description: '版本自动识别失败时按此版本解析', control: 'select', value: 'GB/T 32960-2016' },
  { title: '输入自动格式化', description: '粘贴报文后自动按字节分组排版', control: 'switch' },
  { title: '解析后自动定位第一个字段', description: '解析成功后自动选中并联动字节视图', control: 'switch' },
  { title: '默认显示字节视图', description: '解析工作台的字节视图默认可见性', control: 'switch' },
  {
    title: '默认展开解析结果分组',
    description: '字段表分组(报文头 / 数据单元 / BCC)的初始展开状态',
    control: 'switch',
  },
]
</script>

<template>
  <p class="cat-hint">
    报文解析页当前自动识别报文版本并完整容忍空格 / 换行 / 0x 前缀;以下默认行为开关将在后续版本接入解析页。
  </p>
  <div class="row-group">
    <PlannedSettingRows :rows="plannedRows" />
  </div>
</template>

<style scoped>
/* 分类说明 + 组内末行去分隔线 */
.cat-hint {
  margin: 10px 0 6px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-tertiary);
}

.row-group :deep(.setting-row:last-child) {
  border-bottom: none;
}
</style>
