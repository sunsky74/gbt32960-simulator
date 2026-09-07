<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, computed } from 'vue'
import { message } from 'ant-design-vue'
import { CopyOutlined, DeleteOutlined, EditOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'
import * as ParserService from '../../wailsjs/go/bridge/ParserService'
import type { bridge as bridgeNs } from '../../wailsjs/go/models'
import type { parser as parserNs } from '../../wailsjs/go/models'
import ByteGridView, { type ByteHover, type ByteRange } from '../components/parser/ByteGridView.vue'
import FieldTableView from '../components/parser/FieldTableView.vue'

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

// 代理:''(未选择/已清除)映射为 undefined,让 a-select 显示 placeholder 而非空白
const selectedPackProxy = computed({
  get: () => (selectedPackId.value === '' ? undefined : selectedPackId.value),
  set: (v: unknown) => {
    selectedPackId.value = typeof v === 'string' ? v : ''
  },
})

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
const byteHover = ref<{
  x: number
  y: number
  byte: number
  field: ParsedField | null
  source: 'byte' | 'field'
  issue?: string
} | null>(null)

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

const cardStyle = computed<{ left?: string; top?: string }>(() => {
  const c = byteHover.value
  if (!c) return {}
  const W = 250
  const x = Math.max(8, Math.min(c.x + 16, window.innerWidth - W - 16))
  const y = c.y + 24 > window.innerHeight - 170 ? Math.max(8, c.y - 170) : c.y + 24
  return { left: x + 'px', top: y + 'px' }
})

// 字段原始 HEX 可能很长(如大段数据单元),卡片内截断展示,完整值看表格
function truncHex(s: string): string {
  if (!s) return '-'
  return s.length > 48 ? s.slice(0, 48) + '…' : s
}

// 原始报文悬浮完整预览:两字符一组、每行 16 字节;超长截断(完整内容可点报文编辑 / 复制HEX)
const tooltipHex = computed(() => {
  const s = parsedHex.value.toUpperCase()
  if (!s) return ''
  const bytes = s.match(/../g) ?? []
  const MAX_LINES = 20
  const lines: string[] = []
  for (let i = 0; i < bytes.length; i += 16) lines.push(bytes.slice(i, i + 16).join(' '))
  if (lines.length > MAX_LINES) {
    return (
      lines.slice(0, MAX_LINES).join('\n') +
      `\n…(已截断,共 ${bytes.length} 字节;点击报文可编辑,复制HEX 可取全文)`
    )
  }
  return lines.join('\n')
})

// 字节卡片:当前悬停字节的 HEX 与十进制
const hoverByteHex = computed(() => {
  const c = byteHover.value
  if (!c) return ''
  return parsedHex.value.slice(c.byte * 2, c.byte * 2 + 2).toUpperCase()
})

const hoverByteDec = computed(() => {
  const h = hoverByteHex.value
  return h ? parseInt(h, 16) : ''
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
// 左栏最小 360px(地址列 + 每行 8 字节 + 内边距),右栏最小 480px(字段表可读宽度)
const LEFT_MIN = 360
const RIGHT_MIN = 480
const leftW = ref<number | null>(null) // null = 默认 45fr / 55fr

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

// ---------- 重新解析成功后:两栏内容平滑淡入刷新(仅工作台内重复解析触发,首次解析由工作台整体过渡接管) ----------
function flashContent() {
  const el = duoZoneEl.value
  if (!el) return
  el.classList.remove('content-refresh')
  void el.offsetWidth // 强制重排,确保连续解析时动画可重新触发
  el.classList.add('content-refresh')
}

function duoStyle(): Record<string, string> {
  // 固定左右 Grid:默认 45fr/55fr(按自由空间分配,不会因 6px 分割条/间距溢出容器);
  // 左右均设最小宽度,窗口不足时工作区内部横向滚动,绝不被压缩成竖条
  const left = leftW.value !== null ? leftW.value + 'px' : 'minmax(360px, 45fr)'
  const right = leftW.value !== null ? 'minmax(480px, 1fr)' : 'minmax(480px, 55fr)'
  return { gridTemplateColumns: `${left} 6px ${right}` }
}

onMounted(() => {
  void loadParserPacks()
})

onBeforeUnmount(() => {
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
    if (wasParsed) flashContent()
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
    <!-- 状态 A/B:居中输入(解析中保持原内容不闪),状态 C 解析工作台;交叉淡化切换 -->
    <Transition name="stage" mode="out-in">
      <div v-if="stage === 'input'" key="stage-input" class="stage-input">
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
            <a-button type="primary" :disabled="!canParse" :loading="isParsing" @click="doParse">
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
            <a-button @click="doClear">
              <template #icon><DeleteOutlined /></template>
              清空
            </a-button>
            <a-button @click="doCopy">
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

      <!-- 状态 C:解析工作台(顶部摘要/操作 + 左右分析区),整体与输入态交叉淡化 -->
      <div v-else-if="result" key="stage-workspace" class="workspace">
        <div
          ref="topZoneEl"
          class="zone wb-zone"
          :class="{ 'zone-fixed-h': topH !== null }"
          :style="topH !== null ? { height: topH + 'px' } : undefined"
        >
          <!-- 报文概览:标题 + 操作独立成行,Metadata 信息块单独一行,不与按钮互相挤压 -->
          <div class="ov-header">
            <div class="ov-title"><span class="ov-bar"></span>报文概览</div>
            <div class="ov-actions">
              <a-button size="small" @click="editing = !editing">
                <template #icon><EditOutlined /></template>
                {{ editing ? '收起' : '编辑' }}
              </a-button>
              <a-button size="small" type="primary" :disabled="!canParse" :loading="isParsing" @click="doParse">
                <template #icon><ThunderboltOutlined /></template>
                {{ isParsing ? '解析中...' : '重新解析' }}
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
          </div>

          <div class="ov-body">
            <div class="ov-raw">
              <span class="ov-k">原始报文</span>
              <a-tooltip placement="bottomLeft" overlay-class-name="parser-hex-tip" :mouse-enter-delay="0.15">
                <template #title>
                  <div class="hex-tip-body">{{ tooltipHex }}</div>
                </template>
                <span class="ov-hex" @click="editing = true">{{ parsedHex }}</span>
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
                <span class="mb-v">{{ packLabelOf(parsedPackId) }}</span>
              </div>
            </div>
          </div>

          <div class="wb-editor" :class="{ open: editing }">
            <div class="wb-editor-inner">
              <a-textarea
                v-model:value="hexInput"
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
                :issues="result.issues ?? []"
                @hover="onByteHover"
                @pin="onBytePin"
              />
            </div>
          </div>

          <div
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
      </div>
    </Transition>

    <!-- 悬停信息卡片:字段来源显示字段卡,字节来源显示字节卡(Offset/HEX/Decimal/所属字段) -->
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

/* ---------- 状态 C:报文概览(顶部信息区,自然高度不占过多空间) ---------- */
.wb-zone {
  flex: none; /* 概览区自然高度;拖拽后由 zone-fixed-h + height 控制 */
}

/* 标题行:标题居左、操作按钮居右,与 Metadata 分行避免挤压 */
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

/* ---------- 左右两栏:固定 Grid(默认 45fr/55fr,拖拽后左固定 px 右 1fr);
   左右最小宽度由 minmax 保证,窗口不足时工作区内部横向滚动而非压缩 ---------- */
.duo-zone {
  display: grid;
  gap: 8px;
  align-items: stretch;
  min-height: 0;
  flex: 1;
  overflow: auto;
}

/* 重新解析成功:两栏内容淡入刷新(仅工作台内重复解析触发) */
.duo-zone.content-refresh {
  animation: content-refresh 0.2s ease-out;
}

@keyframes content-refresh {
  from {
    opacity: 0.45;
  }
  to {
    opacity: 1;
  }
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
  flex: 1; /* 填满 .zone 面板高度:左右内容区等高,表格/字节区滚动区不塌陷 */
  min-height: 0;
}

.duo-left :deep(.byte-grid) {
  flex: 1;
  min-height: 200px;
}

/* 解析结果面板:首次出现时从右侧轻微滑入 + 淡入(180~220ms,克制不夸张) */
.duo-right {
  animation: duo-right-in 0.22s ease-out;
}

@keyframes duo-right-in {
  from {
    opacity: 0;
    transform: translateX(8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
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

/* 字段原始 HEX:超宽时折行展示(截断 + 换行,不撑破卡片) */
.bc-hex {
  white-space: normal;
  word-break: break-all;
}

.bc-u {
  color: var(--text-tertiary);
}
/* bc-issue 复用全局 style.css 版本(双主题 token 化),此处不再 scoped 重写 */
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
