<script setup lang="ts">
import { computed } from 'vue'
import CollapsibleCard from '../layout/CollapsibleCard.vue'
import { bitsArrayOf, ensureRow, fieldOf, setBitsArray } from '../../composables/useGroups'

// 动态解析:store.groups 会异步重载,捕获静态引用会悬空
const row = computed(() => ensureRow('alarm'))
const bitsField = computed(() => fieldOf('alarm', 'bits'))

const options = computed(() => (bitsField.value?.bits ?? []).map((b) => ({ label: b.label, value: `bit${b.index}` })))
</script>

<template>
  <CollapsibleCard title="报警设置" :default-open="false">
    <template #extra>
      <a-tag color="red">关联实时数据</a-tag>
    </template>
    <a-checkbox-group
      v-if="bitsField"
      :value="bitsArrayOf(row, bitsField)"
      :options="options"
      class="alarm-bits"
      @change="(vals: Array<string | number | boolean>) => setBitsArray(row, bitsField!, vals)"
    />
    <a-empty v-else description="报警位定义未加载" :image="false" />
  </CollapsibleCard>
</template>
