<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { AppstoreOutlined } from '@ant-design/icons-vue'
import * as ExtService from '../../wailsjs/go/bridge/ExtService'
import { cfg } from '../composables/useConnConfig'
import { reloadSchemaPreservingGroups, store } from '../state'
import PackToolbar from '../components/extensions/PackToolbar.vue'
import PackCard from '../components/extensions/PackCard.vue'
import PackDetailDrawer from '../components/extensions/PackDetailDrawer.vue'
import ImportPackModal from '../components/extensions/ImportPackModal.vue'
import type { PackStatusFilter, PackVersionFilter } from '../components/extensions/types'
import type { bridge } from '../../wailsjs/go/models'

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

// ---------- 搜索与过滤 ----------
const keyword = ref('')
const statusFilter = ref<PackStatusFilter>('all')
const versionFilter = ref<PackVersionFilter>('all')

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

// ---------- 详情 Drawer ----------
const detailPack = ref<bridge.PackInfo | null>(null)
const drawerRef = ref<InstanceType<typeof PackDetailDrawer> | null>(null)
const importModalRef = ref<InstanceType<typeof ImportPackModal> | null>(null)

function openDetail(p: bridge.PackInfo) {
  detailPack.value = p
  // 与原实现一致:打开详情时顺带清空导入弹窗的错误提示(页签重置由 Drawer 内部处理)
  importModalRef.value?.clearError()
}

function openImport() {
  importModalRef.value?.open()
}

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
        drawerRef.value?.invalidateJson(id)
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

// 导入成功后:失效该包 JSON 缓存 → 必要时刷新 schema → 重新拉取列表(与原实现顺序一致)
async function onImported(info: bridge.PackInfo) {
  drawerRef.value?.invalidateJson(info.id)
  await refreshSchemaIfBound(info.id)
  await refresh()
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

    <!-- 工具行:搜索 + 过滤 + 主操作 -->
    <PackToolbar
      v-model:keyword="keyword"
      v-model:status="statusFilter"
      v-model:version="versionFilter"
      :counts="filterCounts"
      :loading="loading"
      @refresh="refresh"
      @import="openImport"
    />

    <!-- Card Grid -->
    <div class="ext-scroll">
      <a-empty
        v-if="filteredPacks.length === 0"
        :description="store.packs.length === 0 ? '暂无扩展包。点击右上角「导入扩展包」安装能力。' : '没有符合过滤条件的扩展包'"
        class="ext-empty"
      />
      <div v-else class="ext-grid">
        <PackCard
          v-for="p in filteredPacks"
          :key="p.id"
          :pack="p"
          :bound-id="boundId"
          :switching="switchingId === p.id"
          @open="openDetail(p)"
          @toggle="(enabled: boolean) => toggleEnabled(p.id, p.label, enabled)"
        />
      </div>
    </div>

    <!-- 详情 Drawer:基本信息 / JSON 配置 双页签 -->
    <PackDetailDrawer
      ref="drawerRef"
      :pack="detailPack"
      :bound-id="boundId"
      :switching-id="switchingId"
      @close="detailPack = null"
      @toggle="(id: string, label: string, enabled: boolean) => toggleEnabled(id, label, enabled)"
      @delete="(id: string, label: string) => deletePack(id, label)"
    />

    <!-- 导入弹窗 -->
    <ImportPackModal ref="importModalRef" @imported="onImported" />
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
</style>
