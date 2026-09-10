<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import type { bridge as bridgeNs } from '../../wailsjs/go/models'
import type { parser as parserNs } from '../../wailsjs/go/models'
import type { ByteHover, ByteRange } from '../components/parser/ByteGridView.vue'
import type { HoverCardInfo } from '../components/parser/HoverInfoCard.vue'
import { useSplitter } from '../components/parser/useSplitter'
import HoverInfoCard from '../components/parser/HoverInfoCard.vue'
import ParseOverview from '../components/parser/ParseOverview.vue'
import ParserDuoZone from '../components/parser/ParserDuoZone.vue'
import ParserInputHero from '../components/parser/ParserInputHero.vue'

type ParseResult = parserNs.Result
type ParsedField = parserNs.Field

// ---------- 页面阶段:input(输入态) / parsed(工作台),不跳路由 ----------
// parseState 四态状态机:
//   idle    初始 / 清空后 —— 干净空状态,不显示任何错误
//   parsing 解析中 —— 按钮 loading,禁止重复触发
//   success 解析成功 —— 展示工作台
//   error   解析失败 —— 仅此状态显示 Error Alert(用户主动解析才进入,绝不因输入为空自动触发)
const stage = ref<'input' | 'parsed'>('input')
const parseState = ref<'idle' | 'parsing' | 'success' | 'error'>('idle')
const editing = ref(false)

const hexInput = ref('')
const result = ref<ParseResult | null>(null)
// 字节视图展示"解析时"的报文快照,与编辑中的输入解耦
const parsedHex = ref('')
const parseError = ref('')

// ---------- 扩展包(meta.scope 含 parser 的包才可选;未选择 = 不使用,空态显示引导话术) ----------
const parserPacks = ref<bridgeNs.PackInfo[]>([])
const selectedPackId = ref('')
// 解析成功时使用的包(与 parsedHex/result 同为快照,工作台概览据此展示)
const parsedPackId = ref('')

const packOptions = computed(() =>
  parserPacks.value.map((p) => ({ value: p.id, label: `${p.label} (${p.baseVersion})` })),
)

function packLabelOf(id: string): string {
  return parserPacks.value.find((p) => p.id === id)?.label ?? id
}

async function loadParserPacks() {
  try {
    parserPacks.value = (await ParserService.ParserPacks()) ?? []
  } catch {
    // 列表加载失败不阻塞解析(按无包可用处理)
  }
  if (selectedPackId.value && !parserPacks.value.some((p) => p.id === selectedPackId.value)) {
    selectedPackId.value = ''
  }
}

const isParsing = computed(() => parseState.value === 'parsing')
// 去空白/分隔符后为空 → 禁用解析按钮(空输入不允许触发解析,初始态保持干净)
const canParse = computed(() => strippedHex() !== '')

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

// 字节/字段悬停信息卡片(跟随鼠标):source 区分字节卡与字段卡
const byteHover = ref<HoverCardInfo | null>(null)

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
  // 同一字段区间内移动:仅跟随鼠标刷新信息卡片,不重建高亮(避免高频重扫)
  const cur = hovered.value
  const same =
    cur !== null &&
    cur.source === 'field' &&
    cur.range.start === r.start &&
    cur.range.end === r.end
  if (!same) hovered.value = { range: r, source: 'field', byte: null }
  if (pos) byteHover.value = { x: pos.x, y: pos.y, byte: r.start, field: findField(r.start), source: 'field' }
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

// ---------- 上下拖拽: 工具栏 ↔ 两栏区 ----------
const pageEl = ref<HTMLElement | null>(null)
const topZoneEl = ref<HTMLElement | null>(null)
const TOP_MIN = 120
const BOTTOM_MIN = 150

const { size: topH, onDown: onTopBarDown, reset: resetTop } = useSplitter({
  axis: 'y',
  bodyClass: 'dragging-ns',
  min: TOP_MIN,
  trackSize: () => pageEl.value?.clientHeight ?? 600,
  startSize: () => topZoneEl.value?.offsetHeight ?? null,
  maxSize: (total) => Math.max(TOP_MIN, Math.min(Math.floor(total / 2), total - BOTTOM_MIN)),
})

// 左右两栏区(拖拽宽度/刷新动画由组件内部管理,页面仅调用其方法)
const duoEl = ref<InstanceType<typeof ParserDuoZone> | null>(null)

onMounted(() => {
  void loadParserPacks()
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
  if (parseState.value === 'parsing') return // 解析中禁止重复触发(按钮 loading+disabled 已禁用,再兜底)
  const wasParsed = stage.value === 'parsed'
  parseError.value = ''
  const err = validateHex()
  if (err) {
    // 只有用户主动点击解析且输入非法时才进入 error 态
    parseError.value = err
    parseState.value = 'error'
    // 工作台内解析失败:自动展开编辑区,便于就地修改;保留已有结果与用户输入
    if (stage.value === 'parsed') editing.value = true
    return
  }
  parseState.value = 'parsing'
  try {
    const res = await ParserService.ParsePacket(strippedHex(), selectedPackId.value)
    result.value = res
    parsedHex.value = strippedHex()
    parsedPackId.value = selectedPackId.value
    stage.value = 'parsed'
    parseState.value = 'success'
    editing.value = false
    pinned.value = null
    hovered.value = null
    byteHover.value = null
    if (wasParsed) duoEl.value?.flash()
  } catch (e) {
    parseError.value = String(e).replace(/^Error:\s*/, '')
    parseState.value = 'error'
  }
}

function doClear() {
  hexInput.value = ''
  result.value = null
  parsedHex.value = ''
  parsedPackId.value = ''
  parseError.value = ''
  parseState.value = 'idle' // 清空 → 回到干净的初始态,错误提示随之消失
  pinned.value = null
  hovered.value = null
  byteHover.value = null
  editing.value = false
  resetTop()
  duoEl.value?.resetWidth()
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
    <!-- 状态 A/B:居中输入(解析中保持原内容不闪),状态 C 解析工作台;交叉淡化切换 -->
    <Transition name="stage" mode="out-in">
      <ParserInputHero
        v-if="stage === 'input'"
        key="stage-input"
        v-model:hexInput="hexInput"
        v-model:selectedPackId="selectedPackId"
        :pack-options="packOptions"
        :can-parse="canParse"
        :is-parsing="isParsing"
        :parse-state="parseState"
        :parse-error="parseError"
        @parse="doParse"
        @clear="doClear"
        @copy="doCopy"
      />

      <!-- 状态 C:解析工作台(顶部摘要/操作 + 左右分析区),整体与输入态交叉淡化 -->
      <div v-else-if="result" key="stage-workspace" class="workspace">
        <div
          ref="topZoneEl"
          class="zone wb-zone"
          :class="{ 'zone-fixed-h': topH !== null }"
          :style="topH !== null ? { height: topH + 'px' } : undefined"
        >
          <ParseOverview
            v-model:hexInput="hexInput"
            v-model:editing="editing"
            :result="result"
            :parsed-hex="parsedHex"
            :parsed-pack-id="parsedPackId"
            :pack-label="packLabelOf(parsedPackId)"
            :can-parse="canParse"
            :is-parsing="isParsing"
            :parse-state="parseState"
            :parse-error="parseError"
            @parse="doParse"
            @clear="doClear"
            @copy-hex="doCopyHex"
          />
        </div>

        <div
          class="hbar"
          title="拖拽调整上下高度,双击恢复"
          @pointerdown="onTopBarDown"
          @dblclick="resetTop"
        ></div>

        <ParserDuoZone
          ref="duoEl"
          :normalized-hex="parsedHex"
          :active="activeRange"
          :active-source="activeSource"
          :active-byte="activeByte"
          :issues="result.issues ?? []"
          :fields="result.fields"
          @byte-hover="onByteHover"
          @byte-pin="onBytePin"
          @field-hover="onFieldHover"
          @field-pin="onFieldPin"
        />
      </div>
    </Transition>

    <HoverInfoCard :card="byteHover" :parsed-hex="parsedHex" />
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

/* ---------- 状态切换:输入态 ↔ 工作台 交叉淡化,无布局跳变 ---------- */
.stage-enter-active {
  transition: opacity 0.2s ease;
}

.stage-leave-active {
  transition: opacity 0.14s ease;
}

.stage-enter-from,
.stage-leave-to {
  opacity: 0;
}

/* 工作台根容器:占满页面剩余高度,内部 toolbar/分割条/双栏独立滚动 */
.workspace {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.zone-fixed-h {
  flex: none; /* 固定高度时退出 flex 分配,高度完全由拖拽决定 */
}

/* ---------- 状态 C:报文概览(顶部信息区,自然高度不占过多空间) ---------- */
.wb-zone {
  flex: none; /* 概览区自然高度;拖拽后由 zone-fixed-h + height 控制 */
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
</style>
