<script setup lang="ts">
import { CopyOutlined } from '@ant-design/icons-vue'

defineProps<{
  html: string
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'copy'): void
}>()
</script>

<template>
  <!-- ===== JSON 配置:高亮代码块 + 复制 ===== -->
  <div class="dd-body">
    <div class="dd-json-head">
      <span class="dd-json-label">JSON</span>
      <a-button size="small" type="text" class="dd-copy" :disabled="!html" @click="emit('copy')">
        <template #icon><CopyOutlined /></template>
        复制
      </a-button>
    </div>
    <a-spin v-if="loading" class="dd-json-loading" />
    <a-alert
      v-else-if="error"
      type="error"
      show-icon
      message="配置读取失败"
      :description="error"
    />
    <!-- eslint-disable-next-line vue/no-v-html -- html 由本地 highlightJson(先 escapeHtml 再 token 着色)生成,内容已转义且来源为本地包文件 -->
    <pre v-else-if="html" class="dd-json" v-html="html"></pre>
    <p v-else class="dd-hint">暂无配置内容</p>
  </div>
</template>

<style scoped>
.dd-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dd-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-tertiary);
}

/* JSON 页签:代码块头部(标签 + 复制) */
.dd-json-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.dd-json-label {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.5px;
  font-family: var(--font-mono);
  color: var(--text-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 1px 5px;
}

.dd-copy {
  font-size: 12px;
  color: var(--text-secondary);
}

.dd-json-loading {
  display: flex;
  justify-content: center;
  padding: 40px 0;
}

/* 代码块:深色常驻底 + 语法高亮,自适应滚动 */
.dd-json {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.65;
  color: #d4d4d4;
  background: #0f0f0f;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 12px 14px;
  max-height: 360px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.dd-json :deep(.j-key) {
  color: #7fb3ff;
}

.dd-json :deep(.j-str) {
  color: #9ece6a;
}

.dd-json :deep(.j-num) {
  color: #e0af68;
}

.dd-json :deep(.j-kw) {
  color: #bb9af7;
}
</style>
