<script setup lang="ts">
import { computed, markRaw, onMounted, ref, type Component } from 'vue'
import { message } from 'ant-design-vue'
import {
  ApiOutlined, CodeOutlined, DatabaseOutlined, ExperimentOutlined, GlobalOutlined,
  SettingOutlined, SearchOutlined, ToolOutlined,
} from '@ant-design/icons-vue'
import * as SystemService from '../../wailsjs/go/bridge/SystemService'
import * as SettingsService from '../../wailsjs/go/bridge/SettingsService'
import { bridge } from '../../wailsjs/go/models'
import SettingsCommonPanel from '../components/settings/SettingsCommonPanel.vue'
import SettingsAppearancePanel from '../components/settings/SettingsAppearancePanel.vue'
import SettingsEditorPanel from '../components/settings/SettingsEditorPanel.vue'
import SettingsParserPanel from '../components/settings/SettingsParserPanel.vue'
import SettingsStoragePanel from '../components/settings/SettingsStoragePanel.vue'
import SettingsNetworkPanel from '../components/settings/SettingsNetworkPanel.vue'
import SettingsKeysPanel from '../components/settings/SettingsKeysPanel.vue'
import SettingsAdvancedPanel from '../components/settings/SettingsAdvancedPanel.vue'

// ---------- 分类注册表:左侧导航 + 右侧面板一一对应 ----------
interface SettingsCategory {
  key: string
  title: string
  icon: Component
}

const categories: SettingsCategory[] = [
  { key: 'common', title: '常用', icon: markRaw(SettingOutlined) },
  { key: 'appearance', title: '外观', icon: markRaw(ApiOutlined) },
  { key: 'editor', title: '编辑器', icon: markRaw(CodeOutlined) },
  { key: 'parser', title: '报文解析', icon: markRaw(SearchOutlined) },
  { key: 'storage', title: '数据与存储', icon: markRaw(DatabaseOutlined) },
  { key: 'network', title: '网络', icon: markRaw(GlobalOutlined) },
  { key: 'keys', title: '快捷键', icon: markRaw(ToolOutlined) },
  { key: 'advanced', title: '高级', icon: markRaw(ExperimentOutlined) },
]

const activeCategory = ref('appearance')
const currentCategory = computed(() => categories.find((c) => c.key === activeCategory.value))

// ---------- 数据与存储:后端只读路径(存储 / 高级两个面板共用,页面挂载时取一次) ----------
const storagePaths = ref<Record<string, string> | null>(null)

// ---------- 控制台导出缓冲(bridge.SettingsService,后端持久化) ----------
const consoleExportCap = ref<number | null>(null)

async function loadExportCap() {
  try {
    const s = await SettingsService.GetAppSettings()
    consoleExportCap.value = s.consoleExportCap
  } catch {
    consoleExportCap.value = null
  }
}

async function saveExportCap(v: number | string | null) {
  const n = typeof v === 'number' ? v : Number(v)
  if (!Number.isFinite(n) || n <= 0) return
  try {
    await SettingsService.SetAppSettings(bridge.AppSettings.createFrom({ consoleExportCap: n }))
    message.success('已保存')
  } catch (e) {
    message.error('保存失败: ' + String(e))
    void loadExportCap()
  }
}

onMounted(async () => {
  try {
    storagePaths.value = await SystemService.StoragePaths()
  } catch {
    storagePaths.value = null
  }
  void loadExportCap()
})

async function openDir(path?: string) {
  if (!path) return
  try {
    await SystemService.OpenDirectory(path)
  } catch (e) {
    message.error(String(e))
  }
}
</script>

<template>
  <!-- 左右分栏:左侧分类导航 / 右侧设置内容(各自独立滚动);
       不复用 .page-root(其 flex-direction: column 会把分栏压成上下堆叠) -->
  <div class="settings-page">
    <aside class="settings-nav">
      <div class="sn-head">
        <span class="sn-title">设置</span>
      </div>
      <div class="sn-scroll">
        <div
          v-for="c in categories"
          :key="c.key"
          class="sn-item"
          :class="{ active: activeCategory === c.key }"
          @click="activeCategory = c.key"
        >
          <component :is="c.icon" class="sn-icon" />
          <span>{{ c.title }}</span>
        </div>
      </div>
    </aside>

    <section class="settings-body">
      <div class="sb-header">
        <h1 class="sb-title">{{ currentCategory?.title }}</h1>
      </div>
      <div class="sb-scroll">
        <!-- 限宽内容容器:大屏不横向无限拉伸(VS Code 设置页风格) -->
        <div class="sb-content">

          <!-- ================ 常用 ================ -->
          <SettingsCommonPanel v-if="activeCategory === 'common'" />

          <!-- ================ 外观 ================ -->
          <SettingsAppearancePanel v-else-if="activeCategory === 'appearance'" />

          <!-- ================ 编辑器 ================ -->
          <SettingsEditorPanel v-else-if="activeCategory === 'editor'" />

          <!-- ================ 报文解析 ================ -->
          <SettingsParserPanel v-else-if="activeCategory === 'parser'" />

          <!-- ================ 数据与存储 ================ -->
          <SettingsStoragePanel
            v-else-if="activeCategory === 'storage'"
            v-model:export-cap="consoleExportCap"
            :storage-paths="storagePaths"
            :open-dir="openDir"
            :save-export-cap="saveExportCap"
          />

          <!-- ================ 网络 ================ -->
          <SettingsNetworkPanel v-else-if="activeCategory === 'network'" />

          <!-- ================ 快捷键 ================ -->
          <SettingsKeysPanel v-else-if="activeCategory === 'keys'" />

          <!-- ================ 高级 ================ -->
          <SettingsAdvancedPanel
            v-else-if="activeCategory === 'advanced'"
            :storage-paths="storagePaths"
            :open-dir="openDir"
          />

        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* 左右分栏(显式 row,不复用 .page-root 的 column 方向) */
.settings-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: row;
  gap: 12px;
  padding: 16px;
  overflow: hidden;
}

/* ---------- 左侧分类导航(IDE Settings 目录):固定宽 + 独立滚动 ---------- */
.settings-nav {
  width: 220px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.sn-head {
  flex-shrink: 0;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-subtle);
}

.sn-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.sn-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
}

.sn-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background-color 0.1s ease, color 0.1s ease;
}

.sn-item:hover {
  background: var(--item-hover-bg);
  color: var(--text-primary);
}

.sn-item.active {
  background: var(--primary-hover-bg);
  color: var(--primary);
  font-weight: 500;
}

.sn-icon {
  font-size: 14px;
}

/* ---------- 右侧设置内容:占满剩余空间 + 独立滚动 ---------- */
.settings-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.sb-header {
  flex-shrink: 0;
  padding: 12px 24px;
  border-bottom: 1px solid var(--border-subtle);
}

.sb-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.sb-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* 限宽内容容器:大屏禁止横向无限拉伸 */
.sb-content {
  max-width: 880px;
  margin: 0 auto;
  padding: 6px 24px 28px;
}
</style>
