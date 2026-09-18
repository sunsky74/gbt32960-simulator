<script setup lang="ts">
// 设置项通用行:标题 + 规划中徽标同行,描述在下方,右侧控件插槽。
// 设置中心 8 个分类强制复用,禁止各分类手写行布局。
defineProps<{
  title: string
  description?: string
  badge?: string
  disabled?: boolean
  monoDesc?: boolean
}>()
</script>

<template>
  <div class="setting-row" :class="{ planned: disabled }">
    <div class="sr-left">
      <div class="sr-title-line">
        <span class="sr-title">{{ title }}</span>
        <span v-if="badge" class="sr-badge">{{ badge }}</span>
      </div>
      <div v-if="description || $slots.description" class="sr-desc" :class="{ mono: monoDesc }">
        <slot name="description">{{ description }}</slot>
      </div>
    </div>
    <div class="sr-action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.setting-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 24px;
  padding: 12px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.sr-left {
  min-width: 0;
  flex: 1;
}

.sr-title-line {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sr-title {
  font-size: var(--fs-14);
  font-weight: 500;
  color: var(--text-primary);
}

.planned .sr-title {
  color: var(--text-secondary);
}

.sr-badge {
  flex-shrink: 0;
  font-size: var(--fs-11);
  line-height: 1;
  padding: 2px 6px;
  border-radius: var(--radius-xs);
  color: var(--text-tertiary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
}

.sr-desc {
  margin-top: 4px;
  font-size: var(--fs-12);
  line-height: 1.6;
  color: var(--text-secondary);
  max-width: 70%;
  overflow-wrap: break-word;
}

.sr-desc.mono {
  font-family: var(--font-mono);
  font-size: var(--fs-11);
}

.sr-action {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
