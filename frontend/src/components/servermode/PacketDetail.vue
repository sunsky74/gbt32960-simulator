<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import * as ParserService from '../../../wailsjs/go/bridge/ParserService'
import type { parser as parserNs } from '../../../wailsjs/go/models'
import ByteGridView, { type ByteHover, type ByteRange } from '../parser/ByteGridView.vue'
import FieldTableView from '../parser/FieldTableView.vue'
import { fmtMs, type StreamRow } from './types'

type ParsedField = parserNs.Field

const props = defineProps<{
  frame: StreamRow | null
  parserPacks: Array<{ id: string; label: string }>
}>()

// ---------- 解析:ParserService.ParsePacket + 扩展包下拉(沿用报文解析页惯例) ----------
const selectedPackId = ref('') // '' = 不使用扩展包
const parseResult = ref<parserNs.Result | null>(null)

const packOptions = computed(() => [
  { label: '不使用扩展包', value: '' },
  ...props.parserPacks.map((p) => ({ label: p.label, value: p.id })),
])

async function parse(hex: string) {
  try {
    parseResult.value = await ParserService.ParsePacket(hex, selectedPackId.value)
  } catch (e) {
    parseResult.value = null
    message.error('解析失败: ' + String(e))
  }
}

// 切换详情帧:重置解析与联动;kind≠normal(未知/加密/告警)不解析,仅展示 hex 与说明
watch(
  () => props.frame,
  (f) => {
    parseResult.value = null
    clearLink()
    if (f && f.kind === 'normal' && f.hex) void parse(f.hex)
  },
)

watch(selectedPackId, () => {
  const f = props.frame
  if (f && f.kind === 'normal' && f.hex) void parse(f.hex)
})

// ---------- 联动高亮共享状态:悬停 + 钉住,active 取钉住优先(照抄 PacketParserPage) ----------
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

function clearLink() {
  hovered.value = null
  pinned.value = null
  byteHover.value = null
}

const byteHover = ref<{
  x: number
  y: number
  byte: number
  field: ParsedField | null
  source: 'byte' | 'field'
  issue?: string
} | null>(null)

function findField(i: number): ParsedField | null {
  const fields = parseResult.value?.fields
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
  byteHover.value = { x: h.x, y: h.y, byte: h.range.start, field: f, source: 'byte', issue: h.issue }
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

function onFieldHover(r: ByteRange | null, pos?: { x: number; y: number }) {
  if (!r) {
    hovered.value = null
    return
  }
  const cur = hovered.value
  const same =
    cur !== null && cur.source === 'field' && cur.range.start === r.start && cur.range.end === r.end
  if (!same) hovered.value = { range: r, source: 'field', byte: null }
  if (pos) byteHover.value = { x: pos.x, y: pos.y, byte: r.start, field: findField(r.start), source: 'field' }
}

function onFieldPin(r: ByteRange | null) {
  pinned.value = r ? { range: r, source: 'field', byte: null } : null
}

// 卡片跟随鼠标定位(照抄 PacketParserPage:屏幕边缘翻转)
const cardStyle = computed<{ left?: string; top?: string }>(() => {
  const c = byteHover.value
  if (!c) return {}
  const W = 250
  const x = Math.max(8, Math.min(c.x + 16, window.innerWidth - W - 16))
  const y = c.y + 24 > window.innerHeight - 170 ? Math.max(8, c.y - 170) : c.y + 24
  return { left: x + 'px', top: y + 'px' }
})

function truncHex(s: string): string {
  if (!s) return '-'
  return s.length > 48 ? s.slice(0, 48) + '…' : s
}

const hoverByteHex = computed(() => {
  const c = byteHover.value
  const hex = props.frame?.hex ?? ''
  if (!c || !hex) return ''
  return hex.slice(c.byte * 2, c.byte * 2 + 2).toUpperCase()
})

const hoverByteDec = computed(() => {
  const h = hoverByteHex.value
  return h ? parseInt(h, 16) : ''
})

function dirText(f: StreamRow): string {
  if (f.kind === 'link') return 'Link'
  if (f.kind === 'warn') return 'Error'
  return f.dir === 'tx' ? 'TX' : 'RX'
}
</script>

<template>
  <section class="detail-pane">
    <template v-if="frame">
      <div class="detail-head">
        <span class="dh-dir" :class="frame.kind === 'link' ? 'dir-link' : frame.kind === 'warn' ? 'dir-error' : frame.dir">
          {{ dirText(frame) }}
        </span>
        <span class="dh-meta mono">{{ fmtMs(frame.time) }}</span>
        <span v-if="frame.vin" class="dh-vin mono">{{ frame.vin }}</span>
        <span v-if="frame.cmd" class="dh-cmd mono">{{ frame.cmd }}</span>
        <span v-if="frame.hex" class="dh-bytes mono">{{ frame.hex.length / 2 }} B</span>
        <div class="spacer" />
        <template v-if="frame.kind === 'normal'">
          <span class="dh-label">扩展包</span>
          <a-select v-model:value="selectedPackId" size="small" class="pack-select" :options="packOptions" />
        </template>
      </div>

      <div v-if="frame.kind === 'normal'" class="detail-cols">
        <div class="detail-left">
          <p class="section-label">报文字节视图(悬停字节查看字段信息)</p>
          <ByteGridView
            :normalized-hex="frame.hex"
            :active="activeRange"
            :active-source="activeSource"
            :active-byte="activeByte"
            :issues="parseResult?.issues ?? []"
            @hover="onByteHover"
            @pin="onBytePin"
          />
        </div>
        <div class="detail-right">
          <p class="section-label">解析结果({{ parseResult?.fields.length ?? 0 }} 个字段 · 悬停/点击联动字节)</p>
          <FieldTableView
            :fields="parseResult?.fields ?? []"
            :active="activeRange"
            @hover="onFieldHover"
            @pin="onFieldPin"
          />
        </div>
      </div>

      <div v-else class="detail-raw">
        <p class="section-label">{{ frame.summary || '该帧不做字段解析' }}</p>
        <pre v-if="frame.hex" class="raw-hex">{{ frame.hex }}</pre>
      </div>
    </template>

    <div v-else class="detail-empty">
      <div class="de-title">报文详情</div>
      <div class="de-sub">点击上方报文行查看字节视图与解析结果</div>
    </div>

    <!-- 悬停信息卡片(照抄 PacketParserPage:字段卡/字节卡 + 屏幕边缘翻转) -->
    <div v-if="byteHover" class="byte-card" :style="cardStyle">
      <template v-if="byteHover.source === 'field' && byteHover.field">
        <div class="bc-name">{{ byteHover.field.name }}</div>
        <div class="bc-kv"><span class="bc-k">Offset</span><span class="bc-v mono">{{ byteHover.field.offset }}</span></div>
        <div class="bc-kv"><span class="bc-k">Length</span><span class="bc-v mono">{{ byteHover.field.length }}</span></div>
        <div class="bc-kv"><span class="bc-k">类型</span><span class="bc-v mono">{{ byteHover.field.type }}</span></div>
        <div class="bc-kv"><span class="bc-k">HEX</span><span class="bc-v mono bc-hex">{{ truncHex(byteHover.field.rawHex) }}</span></div>
        <div class="bc-kv"><span class="bc-k">原始值</span><span class="bc-v mono">{{ byteHover.field.rawValue }}</span></div>
        <div class="bc-kv">
          <span class="bc-k">解析值</span>
          <span class="bc-v">{{ byteHover.field.offsetVal }}<span v-if="byteHover.field.unit" class="bc-u"> {{ byteHover.field.unit }}</span></span>
        </div>
      </template>
      <template v-else>
        <div class="bc-name">字节 #{{ byteHover.byte }}</div>
        <div class="bc-kv"><span class="bc-k">Offset</span><span class="bc-v mono">{{ byteHover.byte }}</span></div>
        <div class="bc-kv"><span class="bc-k">HEX</span><span class="bc-v mono">{{ hoverByteHex }}</span></div>
        <div class="bc-kv"><span class="bc-k">Decimal</span><span class="bc-v mono">{{ hoverByteDec }}</span></div>
        <div class="bc-kv">
          <span class="bc-k">所属字段</span>
          <span class="bc-v">{{ byteHover.field?.name || '-' }}</span>
        </div>
        <div v-if="byteHover.issue" class="bc-issue">{{ byteHover.issue }}</div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.detail-pane {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
}

.detail-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--border-subtle);
  min-height: 34px;
}

.dh-dir {
  font-size: 11px;
  font-weight: 600;
}

.dh-dir.rx {
  color: var(--success);
}

.dh-dir.tx {
  color: var(--primary);
}

.dh-dir.dir-link {
  color: #9254de;
}

.dh-dir.dir-error {
  color: var(--error);
}

.mono {
  font-family: var(--font-mono);
  font-size: 12px;
}

.dh-meta {
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.dh-vin {
  color: var(--primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 260px;
}

.dh-cmd {
  color: var(--text-primary);
}

.dh-bytes {
  color: var(--text-tertiary);
}

.dh-label {
  color: var(--text-secondary);
  font-size: 12px;
}

.pack-select {
  width: 200px;
}

.section-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 6px 12px;
  flex: none;
}

/* ---------- 双栏:左 HEX 字节视图,右协议解析结果 ---------- */
.detail-cols {
  flex: 1;
  min-height: 0;
  display: flex;
}

.detail-left {
  flex: 1 1 44%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border-subtle);
}

.detail-left :deep(.byte-grid) {
  flex: 1;
  min-height: 0;
}

.detail-right {
  flex: 1 1 56%;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.detail-right :deep(.field-table) {
  flex: 1;
  min-height: 0;
}

/* 未知/加密/告警行:仅 hex 原文与说明 */
.detail-raw {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 12px 10px;
}

.raw-hex {
  font-family: var(--font-mono);
  font-size: 12px;
  letter-spacing: 0.4px;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 8px 10px;
  margin: 0;
}

/* ---------- 空态 ---------- */
.detail-empty {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  user-select: none;
}

.de-title {
  color: var(--text-secondary);
  font-size: 14px;
}

.de-sub {
  color: var(--text-tertiary);
  font-size: 12px;
}

/* ---------- 字节悬停信息卡片(样式照抄 PacketParserPage) ---------- */
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
  animation: card-in 0.1s ease-out;
}

@keyframes card-in {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
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

.bc-hex {
  white-space: normal;
  word-break: break-all;
}

.bc-u {
  color: var(--text-tertiary);
}

.bc-issue {
  margin-top: 6px;
  padding: 5px 7px;
  border-radius: 3px;
  font-size: 11px;
  line-height: 1.5;
  color: #ff7875;
  background: rgba(255, 77, 79, 0.1);
}

[data-theme='light'] .bc-issue {
  color: #cf1322;
  background: rgba(207, 19, 34, 0.08);
}
</style>
