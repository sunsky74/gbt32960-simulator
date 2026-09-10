<script setup lang="ts">
import { computed } from 'vue'
import { ImportOutlined, RedoOutlined, SearchOutlined } from '@ant-design/icons-vue'
import type { PackFilterCounts, PackStatusFilter, PackVersionFilter } from './types'

const props = defineProps<{
  keyword: string
  status: PackStatusFilter
  version: PackVersionFilter
  counts: PackFilterCounts
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'update:keyword', v: string): void
  (e: 'update:status', v: PackStatusFilter): void
  (e: 'update:version', v: PackVersionFilter): void
  (e: 'refresh'): void
  (e: 'import'): void
}>()

// antd Input 的 update:value 载荷即 string,与关键字类型一致,可直接 v-model 中转
const keywordModel = computed({
  get: () => props.keyword,
  set: (v: string) => emit('update:keyword', v),
})
</script>

<template>
  <!-- 工具行:搜索 + 过滤成组居左,主操作居右 -->
  <div class="ext-toolbar">
    <a-input
      v-model:value="keywordModel"
      size="small"
      class="ext-search"
      allow-clear
      placeholder="搜索扩展包名称 / ID / 厂商"
    >
      <template #prefix><SearchOutlined /></template>
    </a-input>
    <a-select
      :value="status"
      size="small"
      class="ext-filter"
      @update:value="(v: unknown) => emit('update:status', v as PackStatusFilter)"
    >
      <a-select-option value="all">全部 ({{ counts.all }})</a-select-option>
      <a-select-option value="enabled">已启用 ({{ counts.enabled }})</a-select-option>
      <a-select-option value="disabled">未启用 ({{ counts.disabled }})</a-select-option>
      <a-select-option value="bound">已绑定 ({{ counts.bound }})</a-select-option>
      <a-select-option value="unbound">未绑定 ({{ counts.unbound }})</a-select-option>
    </a-select>
    <a-select
      :value="version"
      size="small"
      class="ext-filter-version"
      @update:value="(v: unknown) => emit('update:version', v as PackVersionFilter)"
    >
      <a-select-option value="all">全部版本</a-select-option>
      <a-select-option value="2016">2016</a-select-option>
      <a-select-option value="2025">2025</a-select-option>
    </a-select>
    <div class="ext-toolbar-actions">
      <a-button size="small" :loading="loading" @click="emit('refresh')">
        <template #icon><RedoOutlined /></template>
        重新扫描
      </a-button>
      <a-button size="small" type="primary" @click="emit('import')">
        <template #icon><ImportOutlined /></template>
        导入扩展包
      </a-button>
    </div>
  </div>
</template>

<style scoped>
/* ---------- 工具行:搜索+过滤成组,主操作居右 ---------- */
.ext-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.ext-search {
  min-width: 320px;
  max-width: 420px;
  flex: 1;
}

.ext-filter {
  width: 150px;
}

.ext-filter-version {
  width: 110px;
}

.ext-toolbar-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
</style>
