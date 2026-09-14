<script setup lang="ts">
import { computed } from 'vue'
import { CopyOutlined, DeleteOutlined, EditOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'
import type { parser as parserNs } from '../../../wailsjs/go/models'

type ParseResult = parserNs.Result

// ---------- 报文概览(工作台顶部信息区):标题/操作行 + 原始报文 + Metadata + 折叠编辑区 + 错误/告警 ----------
// 纯展示组件:编辑开关、重新解析、清空、复制HEX 均上报页面处理。
const props = defineProps<{
  hexInput: string
  result: ParseResult
  parsedHex: string
  parsedPackId: string
  packLabel: string
  editing: boolean
  canParse: boolean
  isParsing: boolean
  parseState: 'idle' | 'parsing' | 'success' | 'error'
  parseError: string
}>()

const emit = defineEmits<{
  (e: 'update:hexInput', v: string): void
  (e: 'update:editing', v: boolean): void
  (e: 'parse'): void
  (e: 'clear'): void
  (e: 'copyHex'): void
}>()

const hexModel = computed({
  get: () => props.hexInput,
  set: (v: string) => emit('update:hexInput', v),
})

// 原始报文悬浮完整预览:两字符一组、每行 16 字节;超长截断(完整内容可点报文编辑 / 复制HEX)
const tooltipHex = computed(() => {
  const s = props.parsedHex.toUpperCase()
  if (!s) return ''
  const bytes = s.match(/../g) ?? []
  const MAX_LINES = 20
  const lines: string[] = []
  for (let i = 0; i < bytes.length; i += 16) lines.push(bytes.slice(i, i + 16).join(' '))
  if (lines.length > MAX_LINES) {
    return lines.slice(0, MAX_LINES).join('\n') + `\n…(已截断,共 ${bytes.length} 字节;点击报文可编辑,复制HEX 可取全文)`
  }
  return lines.join('\n')
})
</script>

<template>
  <!-- 报文概览:标题 + 操作独立成行,Metadata 信息块单独一行,不与按钮互相挤压 -->
  <div class="ov-header">
    <div class="ov-title"><span class="ov-bar"></span>报文概览</div>
    <div class="ov-actions">
      <a-button size="small" @click="emit('update:editing', !editing)">
        <template #icon><EditOutlined /></template>
        {{ editing ? '收起' : '编辑' }}
      </a-button>
      <a-button size="small" type="primary" :disabled="!canParse" :loading="isParsing" @click="emit('parse')">
        <template #icon><ThunderboltOutlined /></template>
        {{ isParsing ? '解析中...' : '重新解析' }}
      </a-button>
      <a-button size="small" @click="emit('clear')">
        <template #icon><DeleteOutlined /></template>
        清空
      </a-button>
      <a-button size="small" @click="emit('copyHex')">
        <template #icon><CopyOutlined /></template>
        复制HEX
      </a-button>
    </div>
  </div>

  <div class="ov-body">
    <div class="ov-raw">
      <span class="ov-k">原始报文</span>
      <a-tooltip placement="bottomLeft" overlay-class-name="parser-hex-tip" :mouse-enter-delay="0.15">
        <template #title>
          <div class="hex-tip-body">{{ tooltipHex }}</div>
        </template>
        <span class="ov-hex" @click="emit('update:editing', true)">{{ parsedHex }}</span>
      </a-tooltip>
    </div>
    <div class="ov-meta">
      <div class="meta-block">
        <span class="mb-k">总长度</span>
        <span class="mb-v">{{ result.totalBytes }} B</span>
      </div>
      <div class="meta-block">
        <span class="mb-k">版本</span>
        <span class="mb-v">{{ result.version }}</span>
      </div>
      <div class="meta-block">
        <span class="mb-k">命令</span>
        <span class="mb-v">{{ result.command }}</span>
      </div>
      <div class="meta-block meta-vin">
        <span class="mb-k">VIN</span>
        <span class="mb-v">{{ result.vin || '-' }}</span>
      </div>
      <div v-if="parsedPackId" class="meta-block meta-pack">
        <span class="mb-k">扩展包</span>
        <span class="mb-v">{{ packLabel }}</span>
      </div>
    </div>
  </div>

  <div class="wb-editor" :class="{ open: editing }">
    <div class="wb-editor-inner">
      <a-textarea
        v-model:value="hexModel"
        :rows="3"
        class="hex-input"
        spellcheck="false"
        placeholder="修改报文后点击「重新解析」"
      />
    </div>
  </div>
  <Transition name="alert">
    <a-alert
      v-if="parseState === 'error' && parseError"
      type="error"
      show-icon
      message="解析失败"
      :description="parseError"
      class="parse-alert"
    />
  </Transition>
  <Transition name="alert">
    <a-alert
      v-if="result.warnings && result.warnings.length"
      type="warning"
      show-icon
      message="解析告警"
      class="parse-alert"
    >
      <template #description>
        <div v-for="(w, i) in result.warnings" :key="i">{{ w }}</div>
      </template>
    </a-alert>
  </Transition>
</template>

<style scoped>
.ov-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px 0;
}

.ov-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.ov-bar {
  width: 3px;
  height: 14px;
  border-radius: 2px;
  background: var(--primary);
}

.ov-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.ov-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 12px 10px;
}

/* 原始报文行:截断展示,悬浮 Tooltip 看完整内容,点击进入编辑 */
.ov-raw {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.ov-k {
  font-size: 12px;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.ov-hex {
  font-family: var(--font-mono);
  font-size: 12px;
  letter-spacing: 0.5px;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 3px 10px;
  max-width: 620px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
  transition:
    border-color 0.12s ease,
    color 0.12s ease;
}

.ov-hex:hover {
  border-color: var(--primary);
  color: var(--text-primary);
}

/* Metadata 信息块:总长度 / 版本 / 命令 / VIN 一眼可见 */
.ov-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.meta-block {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
}

.mb-k {
  font-size: 12px;
  color: var(--text-secondary);
}

.mb-v {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-mono);
  white-space: nowrap;
}

/* VIN 主色强调,便于快速定位车辆 */
.meta-vin .mb-v {
  color: var(--primary);
}

/* 编辑区:grid-rows 0fr/1fr 平滑展开收起(与全局 cc-body 同款折叠动画) */
.wb-editor {
  display: grid;
  grid-template-rows: 0fr;
  transition:
    grid-template-rows 0.2s ease,
    padding-top 0.2s ease;
  padding: 0 12px;
}

.wb-editor.open {
  grid-template-rows: 1fr;
  padding-top: 8px;
}

.wb-editor-inner {
  overflow: hidden;
  min-height: 0;
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

<style>
/* 报文概览:原始报文完整 HEX 悬浮预览(全局样式 —— Tooltip 挂载于 body,scoped 无法命中) */
.parser-hex-tip {
  max-width: 460px;
}

.parser-hex-tip .ant-tooltip-inner {
  max-height: 420px;
  overflow: auto;
}

.parser-hex-tip .hex-tip-body {
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.75;
  white-space: pre;
}
</style>
