<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import * as ParserService from '../../../wailsjs/go/bridge/ParserService'
import type { parser as parserNs } from '../../../wailsjs/go/models'
import ByteGridView, { type ByteHover, type ByteRange } from '../parser/ByteGridView.vue'
import FieldTableView from '../parser/FieldTableView.vue'
import JsonTree from '../JsonTree.vue'
import ResizableDividerCol from '../layout/ResizableDividerCol.vue'
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

// ---------- 右栏视图切换:字段表(默认)/ JSON 解析树;切换帧不重置 ----------
const view = ref<'fields' | 'json'>('fields')

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
  const same = cur !== null && cur.source === 'field' && cur.range.start === r.start && cur.range.end === r.end
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

// ---------- 双栏宽度:HEX ↔ 解析 内部分割(默认 45%,可拖) ----------
const colsEl = ref<HTMLElement | null>(null)
const HEX_MIN_PX = 200
const PARSE_MIN_PX = 240
const COLS_DIVIDER_PX = 12 // 与 .resizable-divider-col 的 12px 命中区同步
const hexW = ref<number | null>(null)

function onColsDrag(px: number) {
  hexW.value = px
}

// 容器出现/窗口缩放后钳制,保证解析栏有可用宽度
function clampHexW() {
  const w = colsEl.value?.clientWidth ?? 0
  if (w <= 0 || hexW.value === null) return
  const max = Math.max(HEX_MIN_PX, w - PARSE_MIN_PX - COLS_DIVIDER_PX)
  hexW.value = Math.min(Math.max(hexW.value, HEX_MIN_PX), max)
}

watch(colsEl, (el) => {
  if (!el) return
  if (hexW.value === null) hexW.value = Math.round(el.clientWidth * 0.45)
  clampHexW()
})

function onWindowResize() {
  clampHexW()
}

onMounted(() => window.addEventListener('resize', onWindowResize))
onBeforeUnmount(() => window.removeEventListener('resize', onWindowResize))
</script>

<template>
  <section class="zone">
    <template v-if="frame">
      <header class="zone-head">
        <span class="zone-title">报文详情</span>
        <span class="bar-sep" />
        <span
          class="dir-badge"
          :class="frame.kind === 'link' ? 'dir-link' : frame.kind === 'warn' ? 'dir-error' : frame.dir"
        >
          {{ dirText(frame) }}
        </span>
        <span class="detail-meta mono">{{ fmtMs(frame.time) }}</span>
        <span v-if="frame.vin" class="detail-vin mono">{{ frame.vin }}</span>
        <span v-if="frame.cmd" class="detail-cmd mono">{{ frame.cmd }}</span>
        <span v-if="frame.summary" class="detail-desc" :title="frame.summary">{{ frame.summary }}</span>
        <div class="spacer" />
        <span v-if="frame.hex" class="detail-bytes mono">{{ frame.hex.length / 2 }} B</span>
        <template v-if="frame.kind === 'normal'">
          <span class="detail-label">扩展包</span>
          <a-select v-model:value="selectedPackId" size="small" class="pack-select" :options="packOptions" />
        </template>
      </header>

      <div v-if="frame.kind === 'normal'" ref="colsEl" class="detail-cols">
        <div class="detail-left" :style="hexW !== null ? { flex: '0 0 auto', width: hexW + 'px' } : undefined">
          <div class="col-head">
            HEX 字节视图
            <span class="col-hint">悬停字节查看字段信息</span>
          </div>
          <div class="col-body">
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
        </div>
        <ResizableDividerCol :min-px="HEX_MIN_PX" :right-min-px="PARSE_MIN_PX" @drag="onColsDrag" />
        <div class="detail-right">
          <div class="col-head">
            协议解析
            <span v-if="view === 'fields'" class="col-hint">
              {{ parseResult?.fields.length ?? 0 }} 个字段 · 悬停/点击联动字节
            </span>
            <span v-else class="col-hint">JSON 解析树 · 与字段表同源</span>
            <div class="spacer" />
            <a-button
              size="small"
              class="view-toggle"
              :type="view === 'fields' ? 'primary' : 'default'"
              @click="view = 'fields'"
            >
              字段表
            </a-button>
            <a-button
              size="small"
              class="view-toggle"
              :type="view === 'json' ? 'primary' : 'default'"
              @click="view = 'json'"
            >
              JSON
            </a-button>
          </div>
          <div v-if="view === 'fields'" class="col-body">
            <FieldTableView
              :fields="parseResult?.fields ?? []"
              :active="activeRange"
              @hover="onFieldHover"
              @pin="onFieldPin"
            />
          </div>
          <div v-else class="col-body">
            <!-- JSON 视图:与字段表同源(parseResult.tree);解析中/失败时给空态提示 -->
            <div v-if="parseResult" class="json-body">
              <JsonTree :value="parseResult.tree" />
            </div>
            <div v-else class="json-empty">无解析结果</div>
          </div>
        </div>
      </div>

      <div v-else class="detail-raw">
        <div class="col-head">
          原始报文
          <span class="col-hint">{{ frame.summary || '该帧不做字段解析' }}</span>
        </div>
        <div class="raw-body">
          <pre v-if="frame.hex" class="raw-hex">{{ frame.hex }}</pre>
        </div>
      </div>
    </template>

    <div v-else class="zone-empty">
      <div class="console-empty-title">报文详情</div>
      <div class="console-empty-sub">点击上方报文查看详情</div>
    </div>

    <!-- 悬停信息卡片(照抄 PacketParserPage:字段卡/字节卡 + 屏幕边缘翻转) -->
    <div v-if="byteHover" class="byte-card" :style="cardStyle">
      <template v-if="byteHover.source === 'field' && byteHover.field">
        <div class="bc-name">{{ byteHover.field.name }}</div>
        <div class="bc-kv">
          <span class="bc-k">Offset</span><span class="bc-v mono">{{ byteHover.field.offset }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">Length</span><span class="bc-v mono">{{ byteHover.field.length }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">类型</span><span class="bc-v mono">{{ byteHover.field.type }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">HEX</span><span class="bc-v mono bc-hex">{{ truncHex(byteHover.field.rawHex) }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">原始值</span><span class="bc-v mono">{{ byteHover.field.rawValue }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">解析值</span>
          <span class="bc-v"
            >{{ byteHover.field.offsetVal
            }}<span v-if="byteHover.field.unit" class="bc-u"> {{ byteHover.field.unit }}</span></span
          >
        </div>
      </template>
      <template v-else>
        <div class="bc-name">字节 #{{ byteHover.byte }}</div>
        <div class="bc-kv">
          <span class="bc-k">Offset</span><span class="bc-v mono">{{ byteHover.byte }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">HEX</span><span class="bc-v mono">{{ hoverByteHex }}</span>
        </div>
        <div class="bc-kv">
          <span class="bc-k">Decimal</span><span class="bc-v mono">{{ hoverByteDec }}</span>
        </div>
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
/* 右栏 JSON 视图:滚动语义与字段表一致(flex 撑满 + 溢出滚动),留白参照 col-head 的 16px 侧距 */
.json-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 8px 16px;
}

.json-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 头部切换按钮:不参与 flex 收缩,保持紧凑 */
.view-toggle {
  flex: none;
}
</style>
