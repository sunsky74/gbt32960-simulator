<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import { CopyOutlined, DeleteOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import type { parser as parserNs } from '../../wailsjs/go/models'
import ByteGridView, { type ByteRange } from '../components/parser/ByteGridView.vue'
import FieldTableView from '../components/parser/FieldTableView.vue'
import { onBeforeUnmount, onMounted } from 'vue'

type ParseResult = parserNs.Result

const hexInput = ref('')
const result = ref<ParseResult | null>(null)
const parseError = ref('')
const parsing = ref(false)

// ---------- 联动高亮共享状态(唯二份:悬停 + 钉住;active 取钉住优先) ----------
const hoveredRange = ref<ByteRange | null>(null)
const pinnedRange = ref<ByteRange | null>(null)
const activeRange = computed(() => pinnedRange.value ?? hoveredRange.value)

function onHover(r: ByteRange | null) {
  hoveredRange.value = r
}
function onPin(r: ByteRange | null) {
  pinnedRange.value = r
}
function onBgClick(e: MouseEvent) {
  // 点击两个联动卡片之外的空白 → 取消钉住
  if (!(e.target as HTMLElement).closest('.duo-zone')) {
    pinnedRange.value = null
  }
}

// ---------- 上下拖拽: 输入卡 ↔ 两栏区 ----------
const pageEl = ref<HTMLElement | null>(null)
const topZoneEl = ref<HTMLElement | null>(null)
const TOP_MIN = 120
const BOTTOM_MIN = 150
const topH = ref<number | null>(null) // null = 自然高度(默认)

let topDrag: { startY: number; startH: number } | null = null

function topMaxAllowed(): number {
  const total = pageEl.value?.clientHeight ?? 600
  return Math.max(TOP_MIN, Math.min(Math.floor(total / 2), total - BOTTOM_MIN))
}

function onTopBarDown(e: PointerEvent) {
  if (!topZoneEl.value) return
  topDrag = { startY: e.clientY, startH: topZoneEl.value.offsetHeight }
  document.body.classList.add('dragging-ns')
  window.addEventListener('pointermove', onTopBarMove)
  window.addEventListener('pointerup', onTopBarUp)
}

function onTopBarMove(e: PointerEvent) {
  if (!topDrag) return
  const next = topDrag.startH + (e.clientY - topDrag.startY)
  topH.value = Math.round(Math.min(Math.max(next, TOP_MIN), topMaxAllowed()))
}

function onTopBarUp() {
  topDrag = null
  document.body.classList.remove('dragging-ns')
  window.removeEventListener('pointermove', onTopBarMove)
  window.removeEventListener('pointerup', onTopBarUp)
}

function resetTop() {
  topH.value = null
}

// ---------- 左右拖拽: 字节视图 ↔ 解析结果 ----------
const duoZoneEl = ref<HTMLElement | null>(null)
const LEFT_MIN = 44 + 6 * 26 + 46 // 地址列 + 6 字节 + 卡片内边距 + 边框余量
const RIGHT_MIN = 560
const leftW = ref<number | null>(null) // null = 默认 42fr/58fr
const stacked = ref(false)

let sideDrag: { startX: number; startW: number } | null = null

function sideMaxAllowed(): number {
  const total = duoZoneEl.value?.clientWidth ?? 1000
  return Math.max(LEFT_MIN, total - RIGHT_MIN - 12)
}

function onSideBarDown(e: PointerEvent) {
  const left = duoZoneEl.value?.querySelector('.duo-left') as HTMLElement | null
  if (!left) return
  sideDrag = { startX: e.clientX, startW: left.offsetWidth }
  document.body.classList.add('dragging-ew')
  window.addEventListener('pointermove', onSideBarMove)
  window.addEventListener('pointerup', onSideBarUp)
}

function onSideBarMove(e: PointerEvent) {
  if (!sideDrag) return
  const next = sideDrag.startW + (e.clientX - sideDrag.startX)
  leftW.value = Math.round(Math.min(Math.max(next, LEFT_MIN), sideMaxAllowed()))
}

function onSideBarUp() {
  sideDrag = null
  document.body.classList.remove('dragging-ew')
  window.removeEventListener('pointermove', onSideBarMove)
  window.removeEventListener('pointerup', onSideBarUp)
}

function resetSide() {
  leftW.value = null
}

function duoStyle(): Record<string, string> {
  if (stacked.value) return { gridTemplateColumns: '1fr' }
  return { gridTemplateColumns: `${leftW.value !== null ? leftW.value + 'px' : '42fr'} 6px 1fr` }
}

// 窄窗(<1200px)堆叠检测
let mql: MediaQueryList | null = null
function onMql(e: MediaQueryListEvent) {
  stacked.value = e.matches
}

onMounted(() => {
  mql = window.matchMedia('(max-width: 1199px)')
  stacked.value = mql.matches
  mql.addEventListener('change', onMql)
})

onBeforeUnmount(() => {
  mql?.removeEventListener('change', onMql)
  onTopBarUp()
  onSideBarUp()
})

// ---------- 解析 ----------
async function doParse() {
  parseError.value = ''
  result.value = null
  pinnedRange.value = null
  hoveredRange.value = null
  if (!hexInput.value.trim()) {
    parseError.value = '请输入 HEX 报文'
    return
  }
  parsing.value = true
  try {
    result.value = await ParserService.ParsePacket(hexInput.value)
  } catch (e) {
    parseError.value = String(e).replace(/^Error:\s*/, '')
  } finally {
    parsing.value = false
  }
}

function doClear() {
  hexInput.value = ''
  result.value = null
  parseError.value = ''
  pinnedRange.value = null
  hoveredRange.value = null
}

function doCopy() {
  if (!hexInput.value.trim()) {
    message.info('没有可复制的内容')
    return
  }
  void navigator.clipboard.writeText(hexInput.value)
  message.success('已复制输入内容')
}

const normalizedHex = computed(() =>
  hexInput.value.replace(/0[xX]/g, '').replace(/[^0-9a-fA-F]/g, ''),
)

const infoTags = computed(() => {
  if (!result.value) return []
  const r = result.value
  return [
    { label: '总长度', value: `${r.totalBytes} B` },
    { label: '版本', value: `${r.version} ${r.versionByte}` },
    { label: '命令', value: r.command },
    { label: '应答', value: r.responseType },
    { label: 'VIN', value: r.vin || '-' },
    { label: '加密', value: r.encryption },
  ]
})
</script>

<template>
  <div ref="pageEl" class="page-root parser-page" @click="onBgClick">
    <div
      ref="topZoneEl"
      class="zone"
      :class="{ 'zone-fixed-h': topH !== null }"
      :style="topH !== null ? { height: topH + 'px' } : undefined"
    >
      <div class="zone-body input-body">
        <div class="input-head">
          <p class="section-label">原始报文(HEX,支持空格 / 换行 / 0x 前缀)</p>
          <div class="parser-actions">
            <a-button type="primary" size="small" :loading="parsing" @click="doParse">
              <template #icon><ThunderboltOutlined /></template>
              解析报文
            </a-button>
            <a-button size="small" @click="doClear">
              <template #icon><DeleteOutlined /></template>
              清空
            </a-button>
            <a-button size="small" @click="doCopy">
              <template #icon><CopyOutlined /></template>
              复制
            </a-button>
          </div>
        </div>
        <div class="input-fill" :class="{ fixed: topH !== null }">
          <a-textarea
            v-model:value="hexInput"
            :rows="4"
            placeholder="粘贴报文,例如: 232301FE4C5356... 或 23 23 01 FE ..."
            class="hex-input"
            spellcheck="false"
          />
        </div>

        <a-alert
          v-if="parseError"
          type="error"
          show-icon
          message="解析失败"
          :description="parseError"
          class="parse-alert"
        />
        <a-alert
          v-if="result && result.warnings && result.warnings.length"
          type="warning"
          show-icon
          message="解析告警"
          class="parse-alert"
        >
          <template #description>
            <div v-for="(w, i) in result.warnings" :key="i">{{ w }}</div>
          </template>
        </a-alert>
      </div>
    </div>

    <template v-if="result">
      <div
        class="hbar"
        title="拖拽调整上下高度,双击恢复"
        @pointerdown="onTopBarDown"
        @dblclick="resetTop"
      ></div>
      <div ref="duoZoneEl" class="duo-zone" :style="duoStyle()">
        <div class="zone duo-left">
          <div class="zone-body duo-body">
            <p class="section-label">报文字节视图</p>
            <div class="info-tags">
              <a-tag v-for="t in infoTags" :key="t.label" class="info-tag">
                <span class="tag-label">{{ t.label }}</span> {{ t.value }}
              </a-tag>
            </div>
            <ByteGridView :normalized-hex="normalizedHex" :active="activeRange" @hover="onHover" @pin="onPin" />
          </div>
        </div>

        <div
          v-if="!stacked"
          class="vbar"
          title="拖拽调整左右宽度,双击恢复"
          @pointerdown="onSideBarDown"
          @dblclick="resetSide"
        ></div>

        <div class="zone duo-right">
          <div class="zone-body duo-body">
            <p class="section-label">解析结果({{ result.fields.length }} 个字段,悬停/点击联动字节)</p>
            <FieldTableView :fields="result.fields" :active="activeRange" @hover="onHover" @pin="onPin" />
          </div>
        </div>
      </div>
    </template>

    <div v-else class="zone">
      <div class="zone-body parser-empty">
        <div class="console-empty-title">输入 HEX 报文开始解析</div>
        <div class="console-empty-sub">支持 GB/T 32960-2016 全字段解析;2025 版当前提供帧级解析</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.parser-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: hidden; /* 高度分配由拖拽控制,页面本身不滚动;各卡片内部自滚 */
}

.zone-fixed-h {
  flex: none; /* 固定高度时退出 flex 分配,高度完全由拖拽决定 */
}

.input-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.input-head .section-label {
  margin: 0;
}

.parser-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.hex-input :deep(textarea) {
  font-family: SFMono-Regular, Consolas, Menlo, monospace;
  letter-spacing: 0.5px;
}

.parse-alert {
  margin-top: 10px;
}

/* 左右两栏:列宽由拖拽状态驱动;窄窗(<1200px)由 JS 切换为堆叠 */
.duo-zone {
  display: grid;
  gap: 8px;
  align-items: stretch;
  min-height: 0;
  flex: 1;
}

.hbar {
  height: 6px;
  border-radius: 3px;
  cursor: row-resize;
  flex-shrink: 0;
  background: transparent;
  transition: background-color 0.12s ease;
}

.hbar:hover {
  background: var(--primary-hover-bg);
}

.vbar {
  width: 6px;
  border-radius: 3px;
  cursor: col-resize;
  background: transparent;
  transition: background-color 0.12s ease;
}

.vbar:hover {
  background: var(--primary-hover-bg);
}

/* 输入区:默认自然高度;固定高度时文本框弹性填满 */
.input-body {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.input-fill {
  min-height: 100px;
}

.input-fill.fixed {
  flex: 1;
  min-height: 0;
  display: flex;
}

.input-fill.fixed :deep(textarea) {
  flex: 1;
  height: 100%;
  resize: none;
}

.duo-body {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.info-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 8px;
}

.info-tag {
  margin-inline-end: 0;
  font-size: 12px;
}

.tag-label {
  color: var(--text-tertiary);
}

.duo-left :deep(.byte-grid) {
  flex: 1;
  min-height: 200px;
}

.duo-right {
  min-width: 0;
}

.parser-empty {
  padding: 48px 16px;
  text-align: center;
}
</style>
