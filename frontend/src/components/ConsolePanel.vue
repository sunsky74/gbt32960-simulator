<script setup lang="ts">
import { nextTick, reactive, ref, watch } from 'vue'
import { store, showToast } from '../state'
import { ConsoleService, type ConsoleEvent, type DownlinkInfo } from '../api/backend'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import { engine } from '../../wailsjs/go/models'
import { cfg } from '../composables/useConnConfig'
import {
  answerHint,
  answerInputKind,
  defaultAnswerValue,
  encodeAnswerValue,
  type AnswerInputKind,
} from '../composables/paramAnswer'
import JsonTree from './JsonTree.vue'

const listEl = ref<HTMLElement | null>(null)
const autoScroll = ref(true)
const search = ref('')
const expandedIdx = ref<number | null>(null)

watch(
  () => store.consoleEvents.length,
  async () => {
    if (!autoScroll.value) return
    await nextTick()
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  },
)

function visible(): ConsoleEvent[] {
  const list =
    store.consoleFilter === 'all'
      ? store.consoleEvents
      : store.consoleEvents.filter((e) => e.kind === store.consoleFilter)
  if (!search.value) return list
  const kw = search.value.toLowerCase()
  return list.filter(
    (e) =>
      (e.cmd ?? '').toLowerCase().includes(kw) ||
      (e.message ?? '').toLowerCase().includes(kw) ||
      (e.hex ?? '').toLowerCase().includes(kw),
  )
}

function kindText(k: string): string {
  switch (k) {
    case 'tx':
      return '发送'
    case 'rx':
      return '接收'
    case 'conn':
      return '链路'
    default:
      return '错误'
  }
}

function timeText(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString('zh-CN', { hour12: false })
  } catch {
    return ''
  }
}

function copyHex(hex?: string) {
  if (hex) {
    void navigator.clipboard.writeText(hex)
  }
}

function downlinkSummary(e: ConsoleEvent): string {
  const d = e.downlink
  if (!d) return ''
  switch (d.kind) {
    case 'query':
      return `查询参数 [${(d.paramIds ?? []).map((id) => '0x' + id.toString(16).padStart(2, '0')).join(' ')}]`
    case 'setup':
      return `设置参数 ${(d.params ?? []).length} 项`
    case 'control':
      return `控制命令 ${d.controlHex ?? ''}`
    case 'remote':
      return `私有远控 流水号=${d.serialNumber} 信息类型=0x${(d.infoTypeFlag ?? 0).toString(16)} 体=${d.bodyHex ?? ''}`
    default:
      return ''
  }
}

/* ---------------- 应答弹窗(标准 0x80/0x81/0x82) ---------------- */

// 一行待填参数:id + 协议定义(无定义则自由 hex)+ 输入形态 + 当前值(十进制/hex 字符串)
interface ParamAnswerRow {
  id: number
  spec?: engine.ParamSpec
  kind: AnswerInputKind
  value: string
  hint: string
}

const respondModal = reactive({
  open: false,
  downlink: null as DownlinkInfo | null,
  respCode: 1,
  paramRows: [] as ParamAnswerRow[],
  respCodeOptions: [] as engine.ResponseCode[],
})

// 协议元数据按版本缓存;打开弹窗时按当前 cfg.version 懒加载,切版本后自然取新键
const paramSpecCache = new Map<string, engine.ParamSpec[]>()
const respCodeCache = new Map<string, engine.ResponseCode[]>()

async function loadParamSpecs(version: string): Promise<engine.ParamSpec[]> {
  const cached = paramSpecCache.get(version)
  if (cached) return cached
  try {
    const specs = await ConsoleService.GetParamSpecs(version)
    paramSpecCache.set(version, specs)
    return specs
  } catch (e) {
    showToast('加载参数定义失败: ' + String(e))
    return []
  }
}

async function loadRespCodes(version: string): Promise<engine.ResponseCode[]> {
  const cached = respCodeCache.get(version)
  if (cached) return cached
  try {
    const codes = await ConsoleService.GetResponseCodes(version)
    respCodeCache.set(version, codes)
    return codes
  } catch (e) {
    showToast('加载应答码失败: ' + String(e))
    return []
  }
}

async function openRespond(e: ConsoleEvent) {
  const d = e.downlink
  if (!d) return
  respondModal.downlink = d
  respondModal.open = true
  const version = cfg.version || '2016'
  if (d.cmd === 0x80) {
    const specs = await loadParamSpecs(version)
    respondModal.paramRows = (d.paramIds ?? []).map((id) => {
      const spec = specs.find((s) => s.id === id)
      return { id, spec, kind: answerInputKind(spec), value: defaultAnswerValue(spec), hint: answerHint(spec) }
    })
  } else {
    respondModal.paramRows = []
  }
  const codes = await loadRespCodes(version)
  respondModal.respCodeOptions = codes
  respondModal.respCode = codes[0]?.code ?? 1
}

async function sendRespond() {
  const d = respondModal.downlink
  if (!d) return
  try {
    if (d.cmd === 0x80) {
      const rows = respondModal.paramRows.map((r) => {
        const row = new engine.ParamResponseRow()
        row.id = r.id
        row.hex = encodeAnswerValue(r.spec, r.value)
        return row
      })
      await MessageService.RespondParamQuery(rows, respondModal.respCode)
    } else {
      await MessageService.RespondAck(d.cmd, respondModal.respCode)
    }
    respondModal.open = false
  } catch (e) {
    showToast('应答失败: ' + String(e))
  }
}

/* ---------------- 导出 ---------------- */

const exportModal = reactive({ open: false, format: 'csv' as 'csv' | 'log' | 'json' })

const formatOptions = [
  { value: 'csv', label: 'CSV', desc: 'Excel 可打开,列结构化' },
  { value: 'log', label: 'LOG', desc: '纯文本逐行,带 HEX 与解码' },
  { value: 'json', label: 'JSON', desc: '完整结构,可再处理' },
]

async function doExport() {
  const kinds = store.consoleFilter === 'all' ? [] : [store.consoleFilter]
  try {
    const r = await ConsoleService.ExportConsole(exportModal.format, kinds)
    exportModal.open = false
    if (r) {
      showToast(`已导出 ${r.count} 条 → ${r.path}`)
    }
  } catch (e) {
    showToast('导出失败: ' + String(e))
  }
}

async function clearConsole() {
  store.consoleEvents = []
  await ConsoleService.ClearConsole()
}
</script>

<template>
  <div class="zone console-panel">
    <div class="console-toolbar">
      <a-button
        v-for="f in ['all', 'tx', 'rx', 'conn', 'error'] as const"
        :key="f"
        size="small"
        :type="store.consoleFilter === f ? 'primary' : 'default'"
        @click="store.consoleFilter = f"
      >
        {{ f === 'all' ? '全部' : kindText(f) }}
      </a-button>
      <a-input
        v-model:value="search"
        size="small"
        class="search-input"
        placeholder="搜索 cmd / hex / 文本"
        allow-clear
      />
      <label class="toolbar-label check-line"> <a-checkbox v-model:checked="autoScroll" />跟随 </label>
      <a-button
        size="small"
        :type="store.consolePaused ? 'primary' : 'default'"
        @click="store.consolePaused = !store.consolePaused"
      >
        {{ store.consolePaused ? '继续' : '暂停' }}
      </a-button>
      <a-button size="small" @click="clearConsole">清空</a-button>
      <a-button size="small" @click="exportModal.open = true">导出</a-button>
      <span class="toolbar-count">{{ store.consoleEvents.length }} 条</span>
    </div>

    <div ref="listEl" class="console-list">
      <div v-for="(e, i) in visible()" :key="i" class="console-row" @click="expandedIdx = expandedIdx === i ? null : i">
        <div class="row-main">
          <span class="row-time">{{ timeText(e.time) }}</span>
          <span class="row-kind" :class="e.kind">{{ kindText(e.kind) }}</span>
          <span class="row-cmd">{{ e.cmd }}</span>
          <span class="row-msg">{{ e.message }}</span>
          <span v-if="e.bytes" class="row-bytes">{{ e.bytes }}B</span>
          <a-button v-if="e.downlink" size="small" type="link" class="respond-btn" @click.stop="openRespond(e)">
            应答
          </a-button>
        </div>
        <div v-if="e.downlink && expandedIdx !== i" class="row-sub" @click.stop="expandedIdx = i">
          ⤷ {{ downlinkSummary(e) }}
        </div>
        <div v-if="expandedIdx === i && (e.hex || e.decoded)" class="row-detail">
          <div class="detail-title" @click.stop="copyHex(e.hex)">HEX(点击复制)</div>
          <div class="detail-hex">{{ e.hex }}</div>
          <div v-if="e.decoded" class="detail-json">
            <JsonTree :value="e.decoded" />
          </div>
        </div>
      </div>
      <div v-if="visible().length === 0" class="console-empty">
        <div class="console-empty-title">暂无事件</div>
        <div class="console-empty-sub">连接平台后，收发报文将在此显示</div>
      </div>
    </div>

    <!-- 应答弹窗 -->
    <a-modal
      v-model:open="respondModal.open"
      :title="`应答下行命令 0x${(respondModal.downlink?.cmd ?? 0).toString(16).toUpperCase()}`"
      :width="520"
      ok-text="发送应答"
      cancel-text="取消"
      @ok="sendRespond"
    >
      <template v-if="respondModal.downlink?.cmd === 0x80">
        <p class="modal-hint">平台查询了 {{ respondModal.paramRows.length }} 个参数,填写参数值:</p>
        <div class="param-rows">
          <div v-for="(row, ri) in respondModal.paramRows" :key="ri" class="param-row">
            <div class="param-head">
              <span class="param-id">0x{{ row.id.toString(16).padStart(2, '0').toUpperCase() }}</span>
              <span v-if="row.spec?.name" class="param-name">{{ row.spec.name }}</span>
            </div>
            <a-select
              v-if="row.kind === 'select'"
              v-model:value="row.value"
              size="small"
              :options="(row.spec?.options ?? []).map((o) => ({ value: String(o.value), label: o.label }))"
            />
            <a-input
              v-else-if="row.kind === 'number'"
              v-model:value="row.value"
              size="small"
              placeholder="十进制数值"
            />
            <a-input v-else v-model:value="row.value" size="small" placeholder="hex,如 00ff" />
            <div v-if="row.hint" class="param-hint">{{ row.hint }}</div>
          </div>
        </div>
      </template>
      <template v-else>
        <p class="modal-hint">发送应答码(空载荷)。</p>
      </template>
      <div class="respond-code">
        <span class="toolbar-label">应答码</span>
        <a-radio-group v-model:value="respondModal.respCode" class="resp-code-group" button-style="solid" size="small">
          <a-radio-button v-for="c in respondModal.respCodeOptions" :key="c.code" :value="c.code">
            0x{{ c.code.toString(16).padStart(2, '0').toUpperCase() }} {{ c.label }}
          </a-radio-button>
        </a-radio-group>
      </div>
    </a-modal>

    <!-- 导出弹窗 -->
    <a-modal
      v-model:open="exportModal.open"
      title="导出控制台事件"
      :width="420"
      ok-text="选择位置并导出"
      cancel-text="取消"
      @ok="doExport"
    >
      <p class="modal-hint">
        范围:{{ store.consoleFilter === 'all' ? '全部事件' : kindText(store.consoleFilter) }}(最多 50000 条)
      </p>
      <a-radio-group v-model:value="exportModal.format" class="format-group">
        <a-radio v-for="f in formatOptions" :key="f.value" :value="f.value" class="format-item">
          {{ f.label }}
          <span class="format-desc">{{ f.desc }}</span>
        </a-radio>
      </a-radio-group>
    </a-modal>
  </div>
</template>

<style scoped>
/* 日志列表在 Panel 内纵向排布,空态容器复用全局 .console-empty(console-empty-*) */
.console-list {
  display: flex;
  flex-direction: column;
}

/* .check-line 已提升至全局 style.css(服务端 PacketStream 复用同款"跟随"开关行) */

/* 事件方向徽标:仅上色,布局交给全局 .row-kind */
.row-kind.tx {
  color: var(--primary-text);
}
.row-kind.rx {
  color: var(--success-text);
}
.row-kind.conn {
  color: var(--warning-text);
}
.row-kind.error {
  color: var(--error);
}

.respond-btn {
  padding: 0 4px;
  height: auto;
}

.row-sub {
  padding: 2px 0 2px 140px;
  color: var(--text-secondary);
  font-size: var(--fs-12);
  cursor: pointer;
}

.param-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 260px;
  overflow-y: auto;
  margin-bottom: 12px;
}

.param-row {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.param-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.param-id {
  color: var(--primary-text);
  font-size: var(--fs-12);
  font-variant-numeric: tabular-nums;
}

.param-name {
  color: var(--text-secondary);
  font-size: var(--fs-12);
}

.param-hint {
  color: var(--text-tertiary);
  font-size: var(--fs-11);
  line-height: 1.4;
}

.respond-code {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
  flex-wrap: wrap;
}

/* 2025 应答码有 7 项,超出弹窗宽度时换行而非撑破 */
.resp-code-group {
  display: flex;
  flex-wrap: wrap;
  flex: 1;
  min-width: 0;
}

.format-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.format-item {
  display: flex;
  align-items: center;
}

.format-desc {
  color: var(--text-tertiary);
  font-size: var(--fs-12);
  margin-left: 8px;
}
</style>
