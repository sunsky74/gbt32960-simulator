<script setup lang="ts">
import { computed } from 'vue'
import CollapsibleCard from '../layout/CollapsibleCard.vue'
import { ensureRow, fieldOf, numOf, setNum } from '../../composables/useGroups'

// 动态解析:store.groups 会异步重载,捕获静态引用会悬空
const row = computed(() => ensureRow('vehicle'))

const stateFields = [
  { key: 'operatingState', label: '车辆状态' },
  { key: 'chargingState', label: '充电状态' },
  { key: 'operationMode', label: '运行模式' },
] as const

function optionsOf(key: string) {
  const f = fieldOf('vehicle', key)
  return (f?.enum ?? []).map((e) => ({ label: e.label, value: e.value }))
}
</script>

<template>
  <CollapsibleCard title="状态设置">
    <template #extra>
      <a-tag color="blue">关联实时数据</a-tag>
    </template>
    <div class="state-grid">
      <div v-for="sf in stateFields" :key="sf.key" class="state-item">
        <span class="state-label">{{ sf.label }}</span>
        <a-select
          :value="numOf(row, sf.key)"
          size="small"
          :options="optionsOf(sf.key)"
          @change="(v: unknown) => setNum(row, sf.key, v)"
        />
      </div>
    </div>
  </CollapsibleCard>
</template>
