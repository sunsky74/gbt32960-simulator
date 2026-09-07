<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  AppstoreOutlined, CopyOutlined, DeleteOutlined, ImportOutlined, RedoOutlined, SearchOutlined,
} from '@ant-design/icons-vue'
import * as ExtService from '../../wailsjs/go/bridge/ExtService'
import { cfg } from '../composables/useConnConfig'
import { reloadSchemaPreservingGroups, store } from '../state'

const loading = ref(false)
const switchingId = ref<string | null>(null)

async function refresh() {
  loading.value = true
  try {
    store.packs = await ExtService.ListPacks()
  } catch (e) {
    message.error('读取扩展包列表失败: ' + String(e))
  } finally {
    loading.value = false
  }
}

// 覆盖导入当前绑定包会改变其单元/命令布局:刷新 schema 让表单即时更新
async function refreshSchemaIfBound(packID: string) {
  if (store.config?.extensionPack === packID) {
    await reloadSchemaPreservingGroups(store.config?.version ?? '2016')
  }
}

const boundId = computed(() => store.config?.extensionPack)

// 首字母徽标:按包 id 稳定散列出色相,深浅主题下均以低饱和底 + 同色相文字呈现
function hueOf(id: string): number {
  let h = 0
  for (const ch of id) h = (h * 31 + (ch.codePointAt(0) ?? 0)) % 360
  return h
}

function initialOf(label: string): string {
  return (label.trim()[0] ?? '?').toUpperCase()
}

// 应用范围次要文本:2016 · 客户端 · 解析
function scopeTextOf(scope?: string[]): string {
  const parts: string[] = []
  if (!scope || scope.includes('client')) parts.push('客户端')
  if (scope?.includes('parser')) parts.push('解析')
  return parts.join(' · ')
}

// ---------- 搜索与过滤 ----------
const keyword = ref('')
const statusFilter = ref<'all' | 'enabled' | 'disabled' | 'bound' | 'unbound'>('all')
const versionFilter = ref<'all' | '2016' | '2025'>('all')

const filteredPacks = computed(() =>
  store.packs.filter((p) => {
    if (versionFilter.value !== 'all' && p.baseVersion !== versionFilter.value) return false
    switch (statusFilter.value) {
      case 'enabled':
        if (!p.enabled) return false
        break
      case 'disabled':
        if (p.enabled) return false
        break
      case 'bound':
        if (p.id !== boundId.value) return false
        break
      case 'unbound':
        if (p.id === boundId.value) return false
        break
    }
    const kw = keyword.value.trim().toLowerCase()
    if (!kw) return true
    return (
      p.label.toLowerCase().includes(kw) ||
      p.id.toLowerCase().includes(kw) ||
      (p.vendor ?? '').toLowerCase().includes(kw)
    )
  }),
)

const filterCounts = computed(() => ({
  all: store.packs.length,
  enabled: store.packs.filter((p) => p.enabled).length,
  disabled: store.packs.filter((p) => !p.enabled).length,
  bound: store.packs.filter((p) => p.id === boundId.value).length,
  unbound: store.packs.filter((p) => p.id !== boundId.value).length,
}))

// ---------- 详情 Drawer:基本信息 / JSON 配置 双页签 ----------
type PackDetail = (typeof store.packs)[number]
const detailPack = ref<PackDetail | null>(null)
const detailTab = ref<'info' | 'json'>('info')

// JSON 原文按包缓存;jsonVersion 触发 computed 重算(Map 本身非响应式)
const packJsonCache = new Map<string, string>()
const jsonVersion = ref(0)
const packJsonLoading = ref(false)
const packJsonError = ref('')

function openDetail(p: PackDetail) {
  detailPack.value = p
  detailTab.value = 'info'
  jsonError.value = ''
}

async function ensurePackJson(p: PackDetail) {
  if (packJsonCache.has(p.id)) return
  packJsonLoading.value = true
  packJsonError.value = ''
  try {
    const raw = await ExtService.GetPackJSON(p.id)
    packJsonCache.set(p.id, JSON.stringify(JSON.parse(raw), null, 2))
    jsonVersion.value++
  } catch (e) {
    packJsonError.value = String(e)
  } finally {
    packJsonLoading.value = false
  }
}

function switchDetailTab(tab: 'info' | 'json') {
  detailTab.value = tab
  if (tab === 'json' && detailPack.value) void ensurePackJson(detailPack.value)
}

async function copyJson() {
  const text = detailPack.value ? packJsonCache.get(detailPack.value.id) : ''
  if (!text) return
  await navigator.clipboard.writeText(text)
  message.success('已复制配置内容')
}

// 简易 JSON 语法高亮:先转义再按 token 着色,输出安全 HTML(内容来源本地包文件)
function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightJson(src: string): string {
  return escapeHtml(src).replace(
    /("(?:\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (m, str: string, colon: string, kw: string, num: string) => {
      if (str) return colon ? `<span class="j-key">${str}</span>${colon}` : `<span class="j-str">${str}</span>`
      if (kw) return `<span class="j-kw">${kw}</span>`
      if (num) return `<span class="j-num">${num}</span>`
      return m
    },
  )
}

const detailJsonHtml = computed(() => {
  void jsonVersion.value
  const p = detailPack.value
  if (!p) return ''
  const src = packJsonCache.get(p.id)
  return src ? highlightJson(src) : ''
})

// ---------- 启用 / 禁用 / 删除 ----------
async function toggleEnabled(id: string, label: string, enabled: boolean) {
  switchingId.value = id
  try {
    await ExtService.SetPackEnabled(id, enabled)
    message.success(enabled ? `已启用「${label}」` : `已停用「${label}」${boundId.value === id ? ',运行时不再生效,重新启用自动恢复' : ''}`)
    await refreshSchemaIfBound(id)
    await refresh()
    if (detailPack.value?.id === id) {
      detailPack.value = store.packs.find((p) => p.id === id) ?? null
    }
  } catch (e) {
    message.error(String(e))
    await refresh()
  } finally {
    switchingId.value = null
  }
}

function deletePack(id: string, label: string) {
  Modal.confirm({
    title: `删除扩展包「${label}」?`,
    content: '删除后无法恢复;若当前档案绑定该包,绑定将自动解除。',
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    async onOk() {
      try {
        await ExtService.DeletePack(id)
        message.success('已删除')
        packJsonCache.delete(id)
        if (store.config?.extensionPack === id) {
          store.config.extensionPack = ''
          cfg.extensionPack = ''
          await reloadSchemaPreservingGroups(store.config?.version ?? '2016')
        }
        detailPack.value = null
        await refresh()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}

// ---------- 导入弹窗:选文件 / 粘贴 JSON / 格式说明 ----------
const importOpen = ref(false)
const importTab = ref<'file' | 'json' | 'spec'>('file')
const jsonText = ref('')
const jsonError = ref('')
const importing = ref(false)

// 覆盖 meta/appendUnits/数值变换/bits/私有命令的最小可用示例(格式说明页一键填入)
const SPEC_EXAMPLE = `{
  "meta": { "id": "my-pack", "label": "我的扩展包", "vendor": "示例", "baseVersion": "2016", "scope": ["client", "parser"] },
  "realtime": {
    "appendUnits": [
      {
        "key": "telemetry", "title": "私有遥测单元", "unitCode": 128, "enabled": true,
        "fields": [
          { "key": "soc2", "label": "SOC2", "type": "u8", "unit": "%" },
          { "key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V" },
          { "key": "temp", "label": "温度", "type": "i8", "offset": 40, "unit": "°C" },
          { "key": "flags", "label": "状态位", "type": "bits",
            "bits": [{ "index": 0, "label": "锁车" }, { "index": 1, "label": "限速" }] }
        ]
      }
    ]
  },
  "commands": [
    {
      "key": "extData", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual+periodic",
      "body": { "type": "fields", "fields": [{ "key": "seq", "label": "流水号", "type": "u16" }] }
    }
  ]
}`

function openImport() {
  importTab.value = 'file'
  jsonText.value = ''
  jsonError.value = ''
  importOpen.value = true
}

async function importFromFile() {
  importing.value = true
  jsonError.value = ''
  try {
    const path = await ExtService.PickPackFile()
    if (!path) return
    const info = await ExtService.ImportPack(path)
    message.success(`已导入「${info.label}」`)
    importOpen.value = false
    packJsonCache.delete(info.id)
    await refreshSchemaIfBound(info.id)
    await refresh()
  } catch (e) {
    jsonError.value = String(e)
  } finally {
    importing.value = false
  }
}

async function importFromJSON() {
  if (!jsonText.value.trim()) {
    jsonError.value = '请输入扩展包 JSON 内容'
    return
  }
  importing.value = true
  jsonError.value = ''
  try {
    const info = await ExtService.ImportPackJSON(jsonText.value)
    message.success(`已导入「${info.label}」`)
    importOpen.value = false
    jsonText.value = ''
    packJsonCache.delete(info.id)
    await refreshSchemaIfBound(info.id)
    await refresh()
  } catch (e) {
    jsonError.value = String(e)
  } finally {
    importing.value = false
  }
}

function fillExample() {
  jsonText.value = SPEC_EXAMPLE
  jsonError.value = ''
  importTab.value = 'json'
}

async function copyExample() {
  await navigator.clipboard.writeText(SPEC_EXAMPLE)
  message.success('示例已复制,可在「粘贴 JSON」页修改后导入')
}

onMounted(refresh)
</script>

<template>
  <div class="page-root ext-page">
    <!-- 页头:标题区 -->
    <div class="ext-header">
      <div class="ext-heading">
        <h1 class="ext-title"><AppstoreOutlined class="ext-title-icon" />扩展包</h1>
        <p class="ext-sub">管理协议扩展、私有数据单元与第三方能力</p>
      </div>
    </div>

    <!-- 工具行:搜索 + 过滤成组居左,主操作居右 -->
    <div class="ext-toolbar">
      <a-input
        v-model:value="keyword"
        size="small"
        class="ext-search"
        allow-clear
        placeholder="搜索扩展包名称 / ID / 厂商"
      >
        <template #prefix><SearchOutlined /></template>
      </a-input>
      <a-select v-model:value="statusFilter" size="small" class="ext-filter">
        <a-select-option value="all">全部 ({{ filterCounts.all }})</a-select-option>
        <a-select-option value="enabled">已启用 ({{ filterCounts.enabled }})</a-select-option>
        <a-select-option value="disabled">未启用 ({{ filterCounts.disabled }})</a-select-option>
        <a-select-option value="bound">已绑定 ({{ filterCounts.bound }})</a-select-option>
        <a-select-option value="unbound">未绑定 ({{ filterCounts.unbound }})</a-select-option>
      </a-select>
      <a-select v-model:value="versionFilter" size="small" class="ext-filter-version">
        <a-select-option value="all">全部版本</a-select-option>
        <a-select-option value="2016">2016</a-select-option>
        <a-select-option value="2025">2025</a-select-option>
      </a-select>
      <div class="ext-toolbar-actions">
        <a-button size="small" :loading="loading" @click="refresh">
          <template #icon><RedoOutlined /></template>
          重新扫描
        </a-button>
        <a-button size="small" type="primary" @click="openImport">
          <template #icon><ImportOutlined /></template>
          导入扩展包
        </a-button>
      </div>
    </div>

    <!-- Card Grid -->
    <div class="ext-scroll">
      <a-empty
        v-if="filteredPacks.length === 0"
        :description="store.packs.length === 0 ? '暂无扩展包。点击右上角「导入扩展包」安装能力。' : '没有符合过滤条件的扩展包'"
        class="ext-empty"
      />
      <div v-else class="ext-grid">
        <div
          v-for="p in filteredPacks"
          :key="p.id"
          class="ext-card"
          :style="{ '--pk-h': hueOf(p.id) }"
          @click="openDetail(p)"
        >
          <div class="ec-head">
            <div class="ec-icon">{{ initialOf(p.label) }}</div>
            <div class="ec-title-wrap">
              <div class="ec-title" :title="p.label">{{ p.label }}</div>
              <div class="ec-id mono" :title="p.id">{{ p.id }}</div>
            </div>
            <!-- 原生 span 阻断冒泡:antd Switch emit('click') 首参是布尔值,
                 组件级 @click.stop 拿不到事件对象,stopPropagation 静默失效 -->
            <span class="ec-switch" @click.stop>
              <a-switch
                size="small"
                :checked="p.enabled"
                :loading="switchingId === p.id"
                @change="(v: unknown) => toggleEnabled(p.id, p.label, v === true)"
              />
            </span>
          </div>

          <div class="ec-lines">
            <div class="ec-line">
              <span class="ec-k">适用</span>
              <span class="ec-v">GB/T 32960-{{ p.baseVersion }} · {{ scopeTextOf(p.scope) }}</span>
            </div>
            <div class="ec-line">
              <span class="ec-k">厂商</span>
              <span class="ec-v ec-vendor">{{ p.vendor || '-' }}</span>
              <span class="ec-bound" :class="{ on: p.id === boundId }">{{ p.id === boundId ? '已绑定' : '未绑定' }}</span>
            </div>
          </div>

          <div class="ec-stats">
            <div class="ec-stat">
              <span class="ec-stat-num">{{ p.unitCount }}</span>
              <span class="ec-stat-label">个实时单元</span>
            </div>
            <div class="ec-stat">
              <span class="ec-stat-num">{{ p.commandCount }}</span>
              <span class="ec-stat-label">条命令</span>
            </div>
            <a class="ec-detail" @click.stop="openDetail(p)">详情 →</a>
          </div>
        </div>
      </div>
    </div>

    <!-- 详情 Drawer:基本信息 / JSON 配置 双页签 -->
    <a-drawer
      :open="!!detailPack"
      width="440"
      placement="right"
      class="ext-detail-drawer"
      :title="detailPack?.label ?? ''"
      @close="detailPack = null"
    >
      <template v-if="detailPack">
        <!-- 页签切换(segmented 风格) -->
        <div class="dd-tabs">
          <button
            class="dd-tab"
            :class="{ active: detailTab === 'info' }"
            @click="switchDetailTab('info')"
          >基本信息</button>
          <button
            class="dd-tab"
            :class="{ active: detailTab === 'json' }"
            @click="switchDetailTab('json')"
          >JSON 配置</button>
        </div>

        <!-- ===== 基本信息:轻卡片 Key-Value ===== -->
        <div v-if="detailTab === 'info'" class="dd-body">
          <div class="dd-card">
            <div class="dd-kv"><span class="dd-k">ID</span><span class="dd-v mono">{{ detailPack.id }}</span></div>
            <div class="dd-kv"><span class="dd-k">厂商</span><span class="dd-v">{{ detailPack.vendor || '-' }}</span></div>
            <div class="dd-kv"><span class="dd-k">协议基准</span><span class="dd-v">GB/T 32960-{{ detailPack.baseVersion }}</span></div>
            <div class="dd-kv">
              <span class="dd-k">应用范围</span>
              <span class="dd-v">
                <span v-if="!detailPack.scope || detailPack.scope.includes('client')" class="dd-scope">客户端模拟</span>
                <span v-if="detailPack.scope?.includes('parser')" class="dd-scope">报文解析</span>
              </span>
            </div>
          </div>

          <div class="dd-card">
            <div class="dd-kv">
              <span class="dd-k">启用状态</span>
              <span class="dd-v" :class="detailPack.enabled ? 'dd-ok' : 'dd-off'">
                {{ detailPack.enabled ? '● 已启用' : '○ 已禁用' }}
              </span>
            </div>
            <div class="dd-kv">
              <span class="dd-k">绑定状态</span>
              <span class="dd-v" :class="detailPack.id === boundId ? 'dd-ok' : ''">
                {{ detailPack.id === boundId ? '已绑定当前档案' : '未绑定' }}
              </span>
            </div>
            <div class="dd-kv"><span class="dd-k">实时数据单元</span><span class="dd-v">{{ detailPack.unitCount }} 个</span></div>
            <div class="dd-kv"><span class="dd-k">私有命令</span><span class="dd-v">{{ detailPack.commandCount }} 条</span></div>
          </div>

          <p v-if="detailPack.id !== boundId" class="dd-hint">在「客户端模拟 → 连接配置 → 扩展包」中绑定后生效。</p>

          <div class="dd-actions">
            <a-button
              v-if="detailPack.enabled"
              class="dd-btn"
              :loading="switchingId === detailPack.id"
              @click="toggleEnabled(detailPack.id, detailPack.label, false)"
            >禁用扩展包</a-button>
            <a-button
              v-else
              class="dd-btn"
              :loading="switchingId === detailPack.id"
              @click="toggleEnabled(detailPack.id, detailPack.label, true)"
            >启用扩展包</a-button>
            <a-button class="dd-btn dd-btn-danger" @click="deletePack(detailPack.id, detailPack.label)">删除扩展包</a-button>
          </div>
        </div>

        <!-- ===== JSON 配置:高亮代码块 + 复制 ===== -->
        <div v-else class="dd-body">
          <div class="dd-json-head">
            <span class="dd-json-label">JSON</span>
            <a-button
              size="small"
              type="text"
              class="dd-copy"
              :disabled="!detailJsonHtml"
              @click="copyJson"
            >
              <template #icon><CopyOutlined /></template>
              复制
            </a-button>
          </div>
          <a-spin v-if="packJsonLoading" class="dd-json-loading" />
          <a-alert
            v-else-if="packJsonError"
            type="error"
            show-icon
            message="配置读取失败"
            :description="packJsonError"
          />
          <pre v-else-if="detailJsonHtml" class="dd-json" v-html="detailJsonHtml"></pre>
          <p v-else class="dd-hint">暂无配置内容</p>
        </div>
      </template>
    </a-drawer>

    <!-- 导入弹窗 -->
    <a-modal
      v-model:open="importOpen"
      title="导入扩展包"
      :footer="null"
      :mask-closable="false"
      width="640px"
    >
      <a-tabs v-model:active-key="importTab" @change="jsonError = ''">
        <a-tab-pane key="file" tab="选择文件">
          <p class="tab-hint">从本地选择一个扩展包 JSON 文件导入。校验(语法/码位/干跑)通过后才会落盘;同 id 覆盖导入会清空该包已保存的表单值。</p>
        </a-tab-pane>
        <a-tab-pane key="json" tab="粘贴 JSON">
          <a-textarea
            v-model:value="jsonText"
            :rows="12"
            class="json-input"
            spellcheck="false"
            placeholder='粘贴扩展包 JSON,例如: {"meta": {"id": "my-pack", "label": "我的包", "baseVersion": "2016"}, ...}'
          />
          <p class="tab-hint">格式与文件导入一致,写法见「格式说明」页签;完整文档 docs/extpack-guide.md。</p>
        </a-tab-pane>
        <a-tab-pane key="spec" tab="格式说明">
          <div class="spec-body">
            <p class="spec-lead">
              扩展包 = 一个 JSON 对象,顶层三段:<code>meta</code>(元信息,必填) +
              <code>realtime.appendUnits</code>(实时追加单元,可选) + <code>commands</code>(私有命令,可选)。
            </p>

            <h4>1. meta 元信息</h4>
            <table class="spec-table">
              <tbody>
                <tr><td class="k">id</td><td>包唯一标识。2~64 位小写字母/数字/连字符;同 id 导入覆盖旧包</td></tr>
                <tr><td class="k">label</td><td>显示名,非空</td></tr>
                <tr><td class="k">vendor</td><td>厂商名,可选,仅展示</td></tr>
                <tr><td class="k">baseVersion</td><td>协议基准版本,只能 <code>"2016"</code> 或 <code>"2025"</code></td></tr>
                <tr><td class="k">scope</td><td>应用范围数组:<code>"client"</code>(客户端模拟) / <code>"parser"</code>(报文解析);缺省视为 <code>["client"]</code></td></tr>
              </tbody>
            </table>

            <h4>2. realtime.appendUnits 实时追加单元</h4>
            <p class="spec-p">拼在标准 0x02 实时报文尾部的私有 TLV(单元码 1B + 长度 2B + 数据),随实时/补发/周期上报一起发送。</p>
            <table class="spec-table">
              <tbody>
                <tr><td class="k">key / title</td><td>组键(同包唯一,禁冒号) / 表单显示标题</td></tr>
                <tr><td class="k">unitCode</td><td>单元码,只允许 <code>0x80~0xFE</code>(128~254),同包唯一;标准码位禁止占用</td></tr>
                <tr><td class="k">enabled</td><td>默认是否启用(false 时表单不勾选、不参与编码)</td></tr>
                <tr><td class="k">multiple / maxRows</td><td>true 时表单多行,<strong>每行编码为一个独立 TLV</strong>;maxRows 上限</td></tr>
                <tr><td class="k">fields</td><td>字段表,至少一项,见下节 DSL</td></tr>
              </tbody>
            </table>

            <h4>3. fields 字段 DSL</h4>
            <table class="spec-table">
              <tbody>
                <tr><td class="k">u8 / u16 / u32</td><td>无符号整数,1 / 2 / 4 字节</td></tr>
                <tr><td class="k">i8 / i16 / i32</td><td>有符号整数,1 / 2 / 4 字节</td></tr>
                <tr><td class="k">f32</td><td>IEEE 754 浮点,4 字节(不支持 scale/offset)</td></tr>
                <tr><td class="k">bits</td><td>位段开关:<code>bits: [{ "index": 0~31, "label": "…" }]</code>,宽度按最大位号自动取 1/2/4 字节</td></tr>
                <tr><td class="k">bytes</td><td>定长原始字节,<code>length</code> 1~255,表单填 hex(如 1A2B)</td></tr>
                <tr><td class="k">tail</td><td>变长尾部(hex 原样追加),建议放字段表末尾</td></tr>
              </tbody>
            </table>
            <p class="spec-p">每个字段:<code>key</code>(组内唯一)、<code>label</code>、<code>type</code>、<code>unit</code>(仅展示)。
              数值变换契约:<strong>物理值 = 线值 × scale + offset</strong>(scale 默认 1、offset 默认 0;scale 须大于 0 且换算结果须为整数线值)。
              例:<code>{ "type": "u16", "scale": 0.1 }</code> 填 12.3 V → 线值 123;温度惯例 <code>{ "type": "i8", "offset": 40 }</code>。</p>

            <h4>4. commands 私有命令(可选)</h4>
            <table class="spec-table">
              <tbody>
                <tr><td class="k">key / label</td><td>命令键(同包唯一,禁冒号、禁与标准组键同名) / 显示名</td></tr>
                <tr><td class="k">code</td><td>命令码,上行预留区:2016 为 <code>0x09~0x7F</code>,2025 为 <code>0x0C~0x7F</code></td></tr>
                <tr><td class="k">direction / trigger</td><td><code>"up"</code> / <code>manual</code> · <code>periodic</code> · <code>manual+periodic</code></td></tr>
                <tr><td class="k">body.type</td><td><code>fields</code> 平铺字段表(单行);<code>realtimeLike</code> 与实时报文同构(6B 时间 + TLV 单元,单元结构同第 2 节);<code>remoteAck</code> 0x8A 远控应答模板(仅 2016,详见完整文档)</td></tr>
              </tbody>
            </table>

            <h4>5. 常见校验规则</h4>
            <ul class="spec-list">
              <li>unitCode 与命令码同包唯一;unitCode 只能用 0x80~0xFE 自定义区</li>
              <li>key 禁含冒号 <code>:</code>;禁用保留键(vehicle / motor / fuelcell / engine / location / extremum / alarm / voltage / temperature 等)</li>
              <li>导入时做静态校验 + 编码干跑,错误带 JSON 路径(如 <code>realtime.appendUnits[0].fields[1].scale</code>),照提示修改即可</li>
            </ul>

            <h4>6. 最小完整示例</h4>
            <pre class="spec-example">{{ SPEC_EXAMPLE }}</pre>
            <div class="spec-actions">
              <a-button size="small" type="primary" @click="fillExample">填入示例并去粘贴页</a-button>
              <a-button size="small" @click="copyExample">复制示例</a-button>
            </div>
            <p class="tab-hint">完整规范、字段类型取值表与错误解读见仓库 docs/extpack-guide.md。</p>
          </div>
        </a-tab-pane>
      </a-tabs>

      <a-alert
        v-if="jsonError"
        type="error"
        show-icon
        message="导入失败"
        :description="jsonError"
        class="import-alert"
      />

      <div class="modal-footer">
        <a-button @click="importOpen = false">取消</a-button>
        <a-button v-if="importTab === 'file'" type="primary" :loading="importing" @click="importFromFile">选择文件…</a-button>
        <a-button v-else-if="importTab === 'json'" type="primary" :loading="importing" @click="importFromJSON">导入</a-button>
        <a-button v-else type="primary" @click="fillExample">填入示例</a-button>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.ext-page {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 12px 16px;
  gap: 12px;
}

/* ---------- 页头 ---------- */
.ext-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex-shrink: 0;
}

.ext-heading {
  min-width: 0;
}

.ext-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.ext-title-icon {
  color: var(--primary);
  font-size: 17px;
}

.ext-sub {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

/* ---------- 工具行:搜索+过滤成组,主操作居右 ---------- */
.ext-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.ext-search {
  min-width: 320px;
  max-width: 420px;
  flex: 1;
}

.ext-filter {
  width: 150px;
}

.ext-filter-version {
  width: 110px;
}

.ext-toolbar-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}

/* ---------- Card Grid:列数随容器宽度自适应(340px 起步,避免大屏拉扁) ---------- */
.ext-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.ext-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
  align-content: start;
}

.ext-empty {
  margin: auto;
  padding-top: 80px;
}

/* ---------- 扩展包 Card(IDE 插件中心风格:图标重心 + 弱标签 + 唯一状态源) ---------- */
.ext-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px;
  background: var(--bg-panel);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease,
    box-shadow 0.15s ease;
}

.ext-card:hover {
  border-color: var(--primary);
  background: var(--bg-elevated);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.18);
}

.ec-head {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 首字母徽标:40x40 视觉重心,色相按包 id 稳定散列 */
.ec-icon {
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  font-size: 17px;
  font-weight: 700;
  font-family: var(--font-mono);
  color: hsl(var(--pk-h, 210) 55% 52%);
  background: hsl(var(--pk-h, 210) 55% 52% / 0.14);
}

.ec-title-wrap {
  min-width: 0;
  flex: 1;
}

.ec-switch {
  flex-shrink: 0;
  display: inline-flex;
}

.ec-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ec-id {
  font-size: 11px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-top: 2px;
}

.mono {
  font-family: var(--font-mono);
}

/* 元数据行:次要文本,无药丸边框(降噪) */
.ec-lines {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ec-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ec-k {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.ec-v {
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ec-vendor {
  min-width: 0;
}

/* 绑定状态:唯一保留的轻量徽标,紧跟厂商名 */
.ec-bound {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  border-radius: 3px;
  color: var(--text-tertiary);
  background: var(--bg-elevated);
}

.ec-bound.on {
  color: var(--success);
  background: rgba(82, 196, 26, 0.12);
}

[data-theme='light'] .ec-bound.on {
  background: rgba(56, 158, 13, 0.12);
}

/* 统计行:数字 + 单位 + 详情入口 */
.ec-stats {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}

.ec-stat {
  display: flex;
  align-items: baseline;
  gap: 5px;
}

.ec-stat-num {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.ec-stat-label {
  font-size: 11px;
  color: var(--text-tertiary);
}

.ec-detail {
  margin-left: auto;
  font-size: 12px;
  color: var(--primary);
  transition: opacity 0.12s ease;
}

.ec-detail:hover {
  opacity: 0.75;
}

/* ---------- 详情 Drawer:双页签 + 轻卡片 + JSON 代码块 ---------- */
.dd-tabs {
  display: flex;
  gap: 4px;
  padding: 3px;
  background: var(--bg-page);
  border-radius: 8px;
  margin-bottom: 14px;
}

.dd-tab {
  flex: 1;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  font-size: 12px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}

.dd-tab:hover {
  color: var(--text-primary);
}

.dd-tab.active {
  background: var(--bg-panel);
  color: var(--text-primary);
  font-weight: 500;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
}

.dd-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* 轻卡片:柔和底色 + 圆角,内为 Key-Value 左右对齐 */
.dd-card {
  background: var(--bg-page);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.dd-kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.dd-k {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.dd-v {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-primary);
  text-align: right;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dd-v.dd-ok {
  color: var(--primary);
}

.dd-v.dd-off {
  color: var(--text-tertiary);
  font-weight: 400;
}

.dd-scope {
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  border-radius: 3px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
}

.dd-scope + .dd-scope {
  margin-left: 6px;
}

.dd-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-tertiary);
}

/* 操作按钮:统一柔和次要态,删除为浅淡危险交互 */
.dd-actions {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}

.dd-btn {
  flex: 1;
  height: 32px;
  font-size: 12px;
}

.dd-btn-danger {
  color: var(--error-color, #ff4d4f);
  border-color: var(--border-subtle);
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.dd-btn-danger:hover {
  background: rgba(255, 77, 79, 0.08) !important;
  border-color: rgba(255, 77, 79, 0.35) !important;
  color: var(--error) !important;
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

/* ---------- 导入弹窗(沿用 PackManager 样式) ---------- */
.tab-hint {
  margin: 0 0 8px;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.json-input :deep(textarea) {
  font-family: var(--font-mono);
  font-size: 12px;
}

.import-alert {
  margin-top: 8px;
}

.import-alert :deep(.ant-alert-description) {
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 140px;
  overflow: auto;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.spec-body {
  max-height: 430px;
  overflow-y: auto;
  padding-right: 4px;
}

.spec-lead {
  margin: 4px 0 8px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-primary);
}

.spec-body h4 {
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.spec-body code {
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 0 4px;
}

.spec-p {
  margin: 0 0 6px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-secondary);
}

.spec-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.spec-table td {
  border: 1px solid var(--border-subtle);
  padding: 4px 8px;
  line-height: 1.6;
  color: var(--text-secondary);
  vertical-align: top;
}

.spec-table td.k {
  width: 130px;
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: nowrap;
}

.spec-list {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-secondary);
}

.spec-example {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 10px 12px;
  max-height: 250px;
  overflow: auto;
  white-space: pre;
}

.spec-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
</style>

<style>
/* 详情 Drawer 全局覆盖(渲染于 body portal,scoped 无法命中):
   柔和面板底色 + 细左边框 + 轻弥散阴影;关闭按钮 hover 圆角背景 */
.ext-detail-drawer .ant-drawer-content {
  background: var(--bg-panel);
}

.ext-detail-drawer .ant-drawer-header {
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border-subtle);
  padding: 14px 16px;
}

.ext-detail-drawer .ant-drawer-title {
  font-size: 15px;
  font-weight: 600;
}

.ext-detail-drawer .ant-drawer-body {
  padding: 16px;
}

.ext-detail-drawer .ant-drawer-close {
  width: 30px;
  height: 30px;
  margin-inline-end: -6px;
  border-radius: 6px;
  transition: background-color 0.15s ease;
}

.ext-detail-drawer .ant-drawer-close:hover {
  background: var(--item-hover-bg);
}

.ext-detail-drawer .ant-drawer-content-wrapper {
  box-shadow: -8px 0 32px rgba(0, 0, 0, 0.24);
}
</style>
