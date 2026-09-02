<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import { CopyOutlined, DeleteOutlined, EditOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import type { parser as parserNs } from '../../wailsjs/go/models'
import ByteGridView, { type ByteHover, type ByteRange } from '../components/parser/ByteGridView.vue'
import FieldTableView from '../components/parser/FieldTableView.vue'
import { onBeforeUnmount, onMounted } from 'vue'

type ParseResult = parserNs.Result
type ParsedField = parserNs.Field

// ---------- 两阶段状态机:input(居中输入) ↔ parsed(解析工作台),不跳路由 ----------
const stage = ref<'input' | 'parsed'>('input')
const editing = ref(false)

const hexInput = ref('')
const result = ref<ParseResult | null>(null)
// 字节视图展示"解析时"的报文快照,与编辑中的输入解耦
const parsedHex = ref('')
const parseError = ref('')
const parsing = ref(false)

// ---------- 联动高亮共享状态:悬停 + 钉住,active 取钉住优先 ----------
interface HoverState {
  range: ByteRange
  source: 'byte' | 'field'
  byte: number | null
}

const hovered = ref<HoverState | null>(null)
const pinned = ref<HoverState | null>(null)
const active = computed(() => pinned.value ?? hovered.value)
const activeRange = computed<ByteRange | null>(() => active.value?.range ?? null)
const activeSource = computed<'byte' | 'field' | null>(() => active.value?.source ?? null)
const activeByte = computed<number | null>(() => active.value?.byte ?? null)

// 字节悬停信息卡片(跟随鼠标)
const byteHover = ref<{ x: number; y: number; byte: number; field: ParsedField | null } | null>(null)

function findField(i: number): ParsedField | null {
  const fields = result.value?.fields
  if (!fields) return null
  for (const f of fields) {
    if (f.offset <= i && i < f.offset + f.length) return f
  }
  return null
}

function onByteHover(h: ByteHover | null) {
  if (!h) {
    hovered.value = null
    byteHover.value = null
    return
  }
  const f = findField(h.range.start)
  hovered.value = {
    range: f ? { start: f.offset, end: f.offset + f.length } : h.range,
    source: 'byte',
    byte: h.range.start,
  }
  byteHover.value = { x: h.x, y: h.y, byte: h.range.start, field: f }
}

function onBytePin(r: ByteRange | null) {
  if (!r) {
    pinned.value = null
    return
  }
  const f = findField(r.start)
  pinned.value = {
    range: f ? { start: f.offset, end: f.offset + f.length } : r,
    source: 'byte',
    byte: r.start,
  }
}

function onFieldHover(r: ByteRange | null) {
  hovered.value = r ? { range: r, source: 'field', byte: null } : null
}

function onFieldPin(r: ByteRange | null) {
  pinned.value = r ? { range: r, source: 'field', byte: null } : null
}

function onBgClick(e: MouseEvent) {
  // 点击联动区域/工具栏/拖拽条之外的空白 → 取消钉住
  if (!(e.target as HTMLElement).closest('.duo-zone, .wb-zone, .hbar, .vbar')) {
    pinned.value = null
  }
}

const cardStyle = computed<{ left?: string; top?: string }>(() => {
  const c = byteHover.value
  if (!c) return {}
  const W = 250
  const x = Math.max(8, Math.min(c.x + 16, window.innerWidth - W - 16))
  const y = c.y + 24 > window.innerHeight - 150 ? Math.max(8, c.y - 150) : c.y + 24
  return { left: x + 'px', top: y + 'px' }
})

// ---------- 上下拖拽: 工具栏 ↔ 两栏区 ----------
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
const LEFT_MIN = 44 + 8 * 26 + 46 // 地址列 + 每行 8 字节 + 卡片内边距 + 边框余量
const RIGHT_MIN = 560
const leftW = ref<number | null>(null) // null = 默认 40fr/60fr
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
  if (stacked.value) {
    // 窄窗堆叠:上下两行均分,minmax(0,1fr) 防止内容撑破与裁切
    return { gridTemplateColumns: '1fr', gridTemplateRows: 'minmax(0, 1fr) minmax(0, 1fr)' }
  }
  return { gridTemplateColumns: `${leftW.value !== null ? leftW.value + 'px' : '40fr'} 6px 1fr` }
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

// ---------- 校验与解析 ----------
// 去空白、0x 前缀与 , : - _ 分隔符(与 Go NormalizeHex 对齐)
function strippedHex(): string {
  return hexInput.value.replace(/\s+/g, '').replace(/0[xX]/g, '').replace(/[,:_-]/g, '')
}

// 去空白与 0x 前缀后:必须全部是 hex 且长度为偶数
function validateHex(): string | null {
  const s = strippedHex()
  if (!s) return '请输入 HEX 报文'
  const bad = s.match(/[^0-9a-fA-F]/g)
  if (bad) return `报文包含非法字符「${[...new Set(bad)].slice(0, 8).join(' ')}」,仅支持 0-9 / A-F 与空格、, : - _ 分隔`
  if (s.length % 2 !== 0) return `HEX 长度为奇数(${s.length} 个字符),无法构成完整字节`
  return null
}

async function doParse() {
  parseError.value = ''
  const err = validateHex()
  if (err) {
    parseError.value = err
    // 工作台内解析失败:自动展开编辑区,便于就地修改
    if (stage.value === 'parsed') editing.value = true
    return
  }
  parsing.value = true
  try {
    const res = await ParserService.ParsePacket(strippedHex())
    result.value = res
    parsedHex.value = strippedHex()
    stage.value = 'parsed'
    editing.value = false
    pinned.value = null
    hovered.value = null
    byteHover.value = null
  } catch (e) {
    parseError.value = String(e).replace(/^Error:\s*/, '')
  } finally {
    parsing.value = false
  }
}

function doClear() {
  hexInput.value = ''
  result.value = null
  parsedHex.value = ''
  parseError.value = ''
  pinned.value = null
  hovered.value = null
  byteHover.value = null
  editing.value = false
  topH.value = null
  leftW.value = null
  stage.value = 'input'
}

function doCopy() {
  if (!hexInput.value.trim()) {
    message.info('没有可复制的内容')
    return
  }
  void navigator.clipboard.writeText(hexInput.value)
  message.success('已复制输入内容')
}

function doCopyHex() {
  // 非编辑态复制解析快照(与工具栏/字节视图一致);编辑态复制当前输入
  const s = editing.value ? strippedHex() : parsedHex.value
  if (!s) {
    message.info('没有可复制的内容')
    return
  }
  void navigator.clipboard.writeText(s)
  message.success(`已复制 ${s.length / 2} 字节 HEX`)
}
</script>

<template>
  <div ref="pageEl" class="page-root parser-page" @click="onBgClick">
    <!-- 状态 A:居中输入,不渲染分析区 -->
    <div v-if="stage === 'input'" class="stage-input">
      <div class="input-hero">
        <h1 class="hero-title">国标报文解析器</h1>
        <p class="hero-sub">支持 GB/T 32960-2016 / 2025</p>
        <a-textarea
          v-model:value="hexInput"
          :rows="7"
          placeholder="粘贴报文,例如: 232301FE4C5356... 或 23 23 01 FE ..."
          class="hex-input"
          spellcheck="false"
        />
        <div class="hero-actions">
          <a-button type="primary" :loading="parsing" @click="doParse">
            <template #icon><ThunderboltOutlined /></template>
            解析报文
          </a-button>
          <a-button @click="doClear">
            <template #icon><DeleteOutlined /></template>
            清空
          </a-button>
          <a-button @click="doCopy">
            <template #icon><CopyOutlined /></template>
            复制
          </a-button>
        </div>
        <a-alert
          v-if="parseError"
          type="error"
          show-icon
          message="无法解析"
          :description="parseError"
          class="parse-alert"
        />
        <p class="hero-hint">支持空格 / 换行 / 连续 HEX / 0x 前缀与 , : - _ 分隔 · 解析后支持字节 ↔ 字段双向联动</p>
      </div>
    </div>

    <!-- 状态 B:解析工作台(顶部工具栏 + 左右分析区) -->
    <template v-else-if="result">
      <div
        ref="topZoneEl"
        class="zone wb-zone"
        :class="{ 'zone-fixed-h': topH !== null }"
        :style="topH !== null ? { height: topH + 'px' } : undefined"
      >
        <div class="wb-toolbar">
          <span class="wb-label">原始报文</span>
          <span class="wb-hex" title="点击编辑报文" @click="editing = true">{{ parsedHex }}</span>
          <a-tag class="info-tag"><span class="tag-label">总长</span> {{ result.totalBytes }} B</a-tag>
          <a-tag class="info-tag"><span class="tag-label">版本</span> {{ result.version }}</a-tag>
          <a-tag class="info-tag"><span class="tag-label">命令</span> {{ result.command }}</a-tag>
          <a-tag class="info-tag"><span class="tag-label">VIN</span> {{ result.vin || '-' }}</a-tag>
          <span class="wb-spacer"></span>
          <a-button size="small" @click="editing = !editing">
            <template #icon><EditOutlined /></template>
            {{ editing ? '收起' : '编辑' }}
          </a-button>
          <a-button size="small" type="primary" :loading="parsing" @click="doParse">
            <template #icon><ThunderboltOutlined /></template>
            重新解析
          </a-button>
          <a-button size="small" @click="doClear">
            <template #icon><DeleteOutlined /></template>
            清空
          </a-button>
          <a-button size="small" @click="doCopyHex">
            <template #icon><CopyOutlined /></template>
            复制HEX
          </a-button>
        </div>
        <div v-if="editing" class="wb-editor">
          <a-textarea
            v-model:value="hexInput"
            :rows="3"
            class="hex-input"
            spellcheck="false"
            placeholder="修改报文后点击「重新解析」"
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
      </div>

      <div
        class="hbar"
        title="拖拽调整上下高度,双击恢复"
        @pointerdown="onTopBarDown"
        @dblclick="resetTop"
      ></div>

      <div ref="duoZoneEl" class="duo-zone" :style="duoStyle()">
        <div class="zone duo-left">
          <div class="zone-body duo-body">
            <p class="section-label">报文字节视图(悬停字节查看字段信息)</p>
            <ByteGridView
              :normalized-hex="parsedHex"
              :active="activeRange"
              :active-source="activeSource"
              :active-byte="activeByte"
              @hover="onByteHover"
              @pin="onBytePin"
            />
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
            <p class="section-label">解析结果({{ result.fields.length }} 个字段 · 悬停/点击联动字节)</p>
            <FieldTableView :fields="result.fields" :active="activeRange" @hover="onFieldHover" @pin="onFieldPin" />
          </div>
        </div>
      </div>
    </template>

    <!-- 字节悬停信息卡片 -->
    <div v-if="byteHover" class="byte-card" :style="cardStyle">
      <template v-if="byteHover.field">
        <div class="bc-name">{{ byteHover.field.name }}</div>
        <div class="bc-kv"><span class="bc-k">Offset</span><span class="bc-v mono">{{ byteHover.field.offset }}</span></div>
        <div class="bc-kv"><span class="bc-k">Length</span><span class="bc-v mono">{{ byteHover.field.length }}</span></div>
        <div class="bc-kv"><span class="bc-k">类型</span><span class="bc-v mono">{{ byteHover.field.type }}</span></div>
        <div class="bc-kv"><span class="bc-k">原始值</span><span class="bc-v mono">{{ byteHover.field.rawValue }}</span></div>
        <div class="bc-kv">
          <span class="bc-k">解析值</span>
          <span class="bc-v">{{ byteHover.field.offsetVal }}<span v-if="byteHover.field.unit" class="bc-u"> {{ byteHover.field.unit }}</span></span>
        </div>
      </template>
      <template v-else>
        <div class="bc-name">字节 Offset {{ byteHover.byte }}</div>
        <div class="bc-kv">
          <span class="bc-k">HEX</span>
          <span class="bc-v mono">{{ parsedHex.slice(byteHover.byte * 2, byteHover.byte * 2 + 2).toUpperCase() }}</span>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.parser-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: hidden; /* 页面本身不滚动;两侧面板各自内部滚动 */
  padding: 16px;
}

.zone-fixed-h {
  flex: none; /* 固定高度时退出 flex 分配,高度完全由拖拽决定 */
}

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

.hero-hint {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 面板标题:与表格滚动高度测算配合,禁止浏览器默认 p 上下外边距 */
.section-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 0 10px;
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

/* ---------- 状态 B:工具栏 ---------- */
.wb-zone {
  flex: none; /* 工具栏自然高度;拖拽后由 zone-fixed-h + height 控制 */
}

.wb-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  flex-wrap: wrap;
}

.wb-label {
  font-size: 12px;
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.wb-hex {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 2px 8px;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.wb-hex:hover {
  border-color: var(--border-strong);
  color: var(--text-primary);
}

.wb-spacer {
  flex: 1;
}

.wb-editor {
  padding: 8px 12px 0;
}

.info-tag {
  margin-inline-end: 0;
  font-size: 12px;
}

.tag-label {
  color: var(--text-tertiary);
}

/* ---------- 左右两栏:列宽由拖拽状态驱动;窄窗(<1200px)由 JS 切换为堆叠 ---------- */
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

.duo-body {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.duo-left :deep(.byte-grid) {
  flex: 1;
  min-height: 200px;
}

.duo-right {
  min-width: 0;
}

.duo-right :deep(.field-table) {
  flex: 1;
  min-height: 0;
}

/* ---------- 字节悬停信息卡片 ---------- */
.byte-card {
  position: fixed;
  z-index: 1200;
  pointer-events: none;
  min-width: 200px;
  max-width: 250px;
  padding: 8px 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-strong);
  border-radius: 4px;
  box-shadow: var(--shadow);
  font-size: 12px;
}

.bc-name {
  color: var(--text-primary);
  font-weight: 600;
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bc-kv {
  display: flex;
  gap: 8px;
}

.bc-k {
  color: var(--text-tertiary);
  min-width: 44px;
  flex-shrink: 0;
}

.bc-v {
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bc-u {
  color: var(--text-tertiary);
}
</style>
