<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import * as ExtService from '../../../wailsjs/go/bridge/ExtService'
import type { bridge } from '../../../wailsjs/go/models'
import PackJsonPanel from './PackJsonPanel.vue'

const props = defineProps<{
  pack: bridge.PackInfo | null
  boundId?: string
  switchingId: string | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'toggle', id: string, label: string, enabled: boolean): void
  (e: 'delete', id: string, label: string): void
}>()

// ---------- 详情 Drawer:基本信息 / JSON 配置 双页签 ----------
const detailTab = ref<'info' | 'json'>('info')

// 打开新包或关闭时回到「基本信息」页签(与原 openDetail 的重置行为一致)
watch(
  () => props.pack,
  () => {
    detailTab.value = 'info'
  },
)

// JSON 原文按包缓存;jsonVersion 触发 computed 重算(Map 本身非响应式)
const packJsonCache = new Map<string, string>()
const jsonVersion = ref(0)
const packJsonLoading = ref(false)
const packJsonError = ref('')

async function ensurePackJson(p: bridge.PackInfo) {
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
  if (tab === 'json' && props.pack) void ensurePackJson(props.pack)
}

async function copyJson() {
  const text = props.pack ? packJsonCache.get(props.pack.id) : ''
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
  const p = props.pack
  if (!p) return ''
  const src = packJsonCache.get(p.id)
  return src ? highlightJson(src) : ''
})

// 覆盖导入 / 删除后按 id 失效 JSON 缓存(由页面调用,与原 packJsonCache.delete 一致)
function invalidateJson(id: string) {
  packJsonCache.delete(id)
}

defineExpose({ invalidateJson })
</script>

<template>
  <!-- 详情 Drawer:基本信息 / JSON 配置 双页签 -->
  <a-drawer
    :open="!!pack"
    width="440"
    placement="right"
    class="ext-detail-drawer"
    :title="pack?.label ?? ''"
    @close="emit('close')"
  >
    <template v-if="pack">
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
          <div class="dd-kv"><span class="dd-k">ID</span><span class="dd-v mono">{{ pack.id }}</span></div>
          <div class="dd-kv"><span class="dd-k">厂商</span><span class="dd-v">{{ pack.vendor || '-' }}</span></div>
          <div class="dd-kv"><span class="dd-k">协议基准</span><span class="dd-v">GB/T 32960-{{ pack.baseVersion }}</span></div>
          <div class="dd-kv">
            <span class="dd-k">应用范围</span>
            <span class="dd-v">
              <span v-if="!pack.scope || pack.scope.includes('client')" class="dd-scope">客户端模拟</span>
              <span v-if="pack.scope?.includes('parser')" class="dd-scope">报文解析</span>
            </span>
          </div>
        </div>

        <div class="dd-card">
          <div class="dd-kv">
            <span class="dd-k">启用状态</span>
            <span class="dd-v" :class="pack.enabled ? 'dd-ok' : 'dd-off'">
              {{ pack.enabled ? '● 已启用' : '○ 已禁用' }}
            </span>
          </div>
          <div class="dd-kv">
            <span class="dd-k">绑定状态</span>
            <span class="dd-v" :class="pack.id === boundId ? 'dd-ok' : ''">
              {{ pack.id === boundId ? '已绑定当前档案' : '未绑定' }}
            </span>
          </div>
          <div class="dd-kv"><span class="dd-k">实时数据单元</span><span class="dd-v">{{ pack.unitCount }} 个</span></div>
          <div class="dd-kv"><span class="dd-k">私有命令</span><span class="dd-v">{{ pack.commandCount }} 条</span></div>
        </div>

        <p v-if="pack.id !== boundId" class="dd-hint">在「客户端模拟 → 连接配置 → 扩展包」中绑定后生效。</p>

        <div class="dd-actions">
          <a-button
            v-if="pack.enabled"
            class="dd-btn"
            :loading="switchingId === pack.id"
            @click="emit('toggle', pack.id, pack.label, false)"
          >禁用扩展包</a-button>
          <a-button
            v-else
            class="dd-btn"
            :loading="switchingId === pack.id"
            @click="emit('toggle', pack.id, pack.label, true)"
          >启用扩展包</a-button>
          <a-button class="dd-btn dd-btn-danger" @click="emit('delete', pack.id, pack.label)">删除扩展包</a-button>
        </div>
      </div>

      <!-- ===== JSON 配置:高亮代码块 + 复制 ===== -->
      <PackJsonPanel
        v-else
        :html="detailJsonHtml"
        :loading="packJsonLoading"
        :error="packJsonError"
        @copy="copyJson"
      />
    </template>
  </a-drawer>
</template>

<style scoped>
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

.mono {
  font-family: var(--font-mono);
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
