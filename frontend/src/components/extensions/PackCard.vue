<script setup lang="ts">
import { computed } from 'vue'
import type { bridge } from '../../../wailsjs/go/models'

const props = defineProps<{
  pack: bridge.PackInfo
  boundId?: string
  switching: boolean
}>()

const emit = defineEmits<{
  (e: 'open'): void
  (e: 'toggle', enabled: boolean): void
}>()

const bound = computed(() => props.pack.id === props.boundId)

// 首字母徽标:按包 id 稳定散列出色相,深浅主题下均以低饱和底 + 同色相文字呈现
function hueOf(id: string): number {
  let h = 0
  for (const ch of id) h = (h * 31 + (ch.codePointAt(0) ?? 0)) % 360
  return h
}

function initialOf(label: string): string {
  return (label.trim()[0] ?? '?').toUpperCase()
}

// 应用范围次要文本:2016 · 客户端 · 解析
function scopeTextOf(scope?: string[]): string {
  const parts: string[] = []
  if (!scope || scope.includes('client')) parts.push('客户端')
  if (scope?.includes('parser')) parts.push('解析')
  return parts.join(' · ')
}
</script>

<template>
  <div class="ext-card" :style="{ '--pk-h': hueOf(pack.id) }" @click="emit('open')">
    <div class="ec-head">
      <div class="ec-icon">{{ initialOf(pack.label) }}</div>
      <div class="ec-title-wrap">
        <div class="ec-title" :title="pack.label">{{ pack.label }}</div>
        <div class="ec-id mono" :title="pack.id">{{ pack.id }}</div>
      </div>
      <!-- 原生 span 阻断冒泡:antd Switch emit('click') 首参是布尔值,
           组件级 @click.stop 拿不到事件对象,stopPropagation 静默失效 -->
      <span class="ec-switch" @click.stop>
        <a-switch
          size="small"
          :checked="pack.enabled"
          :loading="switching"
          @change="(v: unknown) => emit('toggle', v === true)"
        />
      </span>
    </div>

    <div class="ec-lines">
      <div class="ec-line">
        <span class="ec-k">适用</span>
        <span class="ec-v">GB/T 32960-{{ pack.baseVersion }} · {{ scopeTextOf(pack.scope) }}</span>
      </div>
      <div class="ec-line">
        <span class="ec-k">厂商</span>
        <span class="ec-v ec-vendor">{{ pack.vendor || '-' }}</span>
        <span class="ec-bound" :class="{ on: bound }">{{ bound ? '已绑定' : '未绑定' }}</span>
      </div>
    </div>

    <div class="ec-stats">
      <div class="ec-stat">
        <span class="ec-stat-num">{{ pack.unitCount }}</span>
        <span class="ec-stat-label">个实时单元</span>
      </div>
      <div class="ec-stat">
        <span class="ec-stat-num">{{ pack.commandCount }}</span>
        <span class="ec-stat-label">条命令</span>
      </div>
      <a class="ec-detail" @click.stop="emit('open')">详情 →</a>
    </div>
  </div>
</template>

<style scoped>
/* ---------- 扩展包 Card(IDE 插件中心风格:图标重心 + 弱标签 + 唯一状态源) ---------- */
.ext-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px;
  background: var(--bg-panel);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease,
    box-shadow 0.15s ease;
}

.ext-card:hover {
  border-color: var(--primary);
  background: var(--bg-elevated);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.18);
}

.ec-head {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 首字母徽标:40x40 视觉重心,色相按包 id 稳定散列 */
.ec-icon {
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  font-size: 17px;
  font-weight: 700;
  font-family: var(--font-mono);
  color: hsl(var(--pk-h, 210) 55% 52%);
  background: hsl(var(--pk-h, 210) 55% 52% / 0.14);
}

.ec-title-wrap {
  min-width: 0;
  flex: 1;
}

.ec-switch {
  flex-shrink: 0;
  display: inline-flex;
}

.ec-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ec-id {
  font-size: 11px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-top: 2px;
}

.mono {
  font-family: var(--font-mono);
}

/* 元数据行:次要文本,无药丸边框(降噪) */
.ec-lines {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ec-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ec-k {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.ec-v {
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ec-vendor {
  min-width: 0;
}

/* 绑定状态:唯一保留的轻量徽标,紧跟厂商名 */
.ec-bound {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  border-radius: 3px;
  color: var(--text-tertiary);
  background: var(--bg-elevated);
}

.ec-bound.on {
  color: var(--success);
  background: rgba(82, 196, 26, 0.12);
}

[data-theme='light'] .ec-bound.on {
  background: rgba(56, 158, 13, 0.12);
}

/* 统计行:数字 + 单位 + 详情入口 */
.ec-stats {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}

.ec-stat {
  display: flex;
  align-items: baseline;
  gap: 5px;
}

.ec-stat-num {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.ec-stat-label {
  font-size: 11px;
  color: var(--text-tertiary);
}

.ec-detail {
  margin-left: auto;
  font-size: 12px;
  color: var(--primary);
  transition: opacity 0.12s ease;
}

.ec-detail:hover {
  opacity: 0.75;
}
</style>
