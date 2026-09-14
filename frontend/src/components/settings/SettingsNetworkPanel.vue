<script setup lang="ts">
// 网络:软件级默认行为预留配置,当前全部为规划占位。
import PlannedSettingRows from './PlannedSettingRows.vue'
import type { PlannedRow } from './planned'

// 「规划中」占位行(禁用,仅展示)
const plannedRows: PlannedRow[] = [
  { title: '连接测试超时', description: '「测试连接」的默认超时时间', control: 'select', value: '3 秒' },
  { title: '默认连接超时', description: '新建档案的初始连接超时', control: 'select', value: '10 秒' },
  { title: '自动重连', description: '断线后默认是否自动重连(档案级开关的默认值)', control: 'switch' },
  { title: '重连次数 / 间隔', description: '自动重连的尝试次数与间隔', control: 'select', value: '5 次 · 10 秒' },
  { title: 'TLS 默认行为', description: '新建档案的 TLS 初始配置(跳过证书验证等)', control: 'select', value: '标准校验' },
]
</script>

<template>
  <p class="cat-hint">此处配置软件级默认行为;具体连接的地址 / 端口 / VIN / 心跳等按档案配置,见「客户端模拟 → 连接配置」。</p>
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
