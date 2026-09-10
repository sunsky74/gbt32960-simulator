<script setup lang="ts">
import { computed } from 'vue'
import { CopyOutlined, DeleteOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'

// ---------- 输入态(状态 A):居中报文输入区,解析/清空/复制与扩展包选择 ----------
// 纯展示组件:状态机、校验与解析逻辑留在页面,通过事件上报。
const props = defineProps<{
  hexInput: string
  packOptions: Array<{ value: string; label: string }>
  selectedPackId: string
  canParse: boolean
  isParsing: boolean
  parseState: 'idle' | 'parsing' | 'success' | 'error'
  parseError: string
}>()

const emit = defineEmits<{
  (e: 'update:hexInput', v: string): void
  (e: 'update:selectedPackId', v: string): void
  (e: 'parse'): void
  (e: 'clear'): void
  (e: 'copy'): void
}>()

const hexModel = computed({
  get: () => props.hexInput,
  set: (v: string) => emit('update:hexInput', v),
})

// 代理:''(未选择/已清除)映射为 undefined,让 a-select 显示 placeholder 而非空白
const selectedPackProxy = computed({
  get: () => (props.selectedPackId === '' ? undefined : props.selectedPackId),
  set: (v: unknown) => {
    emit('update:selectedPackId', typeof v === 'string' ? v : '')
  },
})
</script>

<template>
  <div class="stage-input">
    <div class="input-hero">
      <h1 class="hero-title">国标报文解析器</h1>
      <p class="hero-sub">支持 GB/T 32960-2016 / 2025</p>
      <a-textarea
        v-model:value="hexModel"
        :rows="7"
        placeholder="粘贴报文,例如: 232301FE4C5356... 或 23 23 01 FE ..."
        class="hex-input"
        spellcheck="false"
      />
      <div class="hero-actions">
        <a-button type="primary" :disabled="!canParse" :loading="isParsing" @click="emit('parse')">
          <template #icon><ThunderboltOutlined /></template>
          {{ isParsing ? '解析中...' : '解析报文' }}
        </a-button>
        <a-select
          v-model:value="selectedPackProxy"
          :options="packOptions"
          :disabled="isParsing"
          class="pack-select"
          placeholder="请选择协议扩展包"
          allow-clear
        />
        <a-button @click="emit('clear')">
          <template #icon><DeleteOutlined /></template>
          清空
        </a-button>
        <a-button @click="emit('copy')">
          <template #icon><CopyOutlined /></template>
          复制
        </a-button>
      </div>
      <Transition name="alert">
        <a-alert
          v-if="parseState === 'error' && parseError"
          type="error"
          show-icon
          message="无法解析"
          :description="parseError"
          class="parse-alert"
        />
      </Transition>
      <p class="hero-hint">支持空格 / 换行 / 连续 HEX / 0x 前缀与 , : - _ 分隔 · 解析后支持字节 ↔ 字段双向联动</p>
    </div>
  </div>
</template>

<style scoped>
/* ---------- 状态 A:居中输入 ---------- */
.stage-input {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow-y: auto;
}

.input-hero {
  margin: auto; /* 两轴居中,且内容超高时顶部不被裁剪 */
  width: 100%;
  max-width: 940px;
  padding: 8px 0 24px;
}

.hero-title {
  margin: 0 0 4px;
  font-size: 22px;
  font-weight: 600;
  color: var(--text-primary);
}

.hero-sub {
  margin: 0 0 20px;
  font-size: 13px;
  color: var(--text-tertiary);
}

.hero-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}

/* 扩展包下拉:解析报文与清空之间 */
.pack-select {
  width: 220px;
}

.hero-hint {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.hex-input :deep(textarea) {
  font-family: var(--font-mono);
  letter-spacing: 0.5px;
}

.parse-alert {
  margin-top: 10px;
  /* 多告警帧(如截断报文)会产生大量告警行,约束高度防撑爆工具栏区挤出工作台 */
  max-height: 140px;
  overflow: auto;
}

/* 解析错误/告警:淡入淡出,避免突然弹出把工作台顶跳 */
.alert-enter-active {
  transition: opacity 0.18s ease;
}

.alert-leave-active {
  transition: opacity 0.12s ease;
}

.alert-enter-from,
.alert-leave-to {
  opacity: 0;
}
</style>
