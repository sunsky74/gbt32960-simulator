<script setup lang="ts">
// 高级:扩展包入口、调试开关(规划)与危险操作。
import { Modal } from 'ant-design-vue'
import { resetAppSettings } from '../../composables/useAppSettings'
import SettingRow from './SettingRow.vue'
import PlannedSettingRows from './PlannedSettingRows.vue'
import type { PlannedRow } from './planned'

defineProps<{
  /** 后端只读存储路径(null 表示仍在加载,页面挂载时取一次) */
  storagePaths: Record<string, string> | null
  /** 打开本地目录 */
  openDir: (path?: string) => void
}>()

// 「规划中」占位行(禁用,仅展示)
const plannedRows: PlannedRow[] = [
  { title: 'Debug 模式', description: '控制台输出协议编解码细节', control: 'switch' },
  { title: '显示协议原始日志', description: '控制台显示未解析的原始帧 hex', control: 'switch' },
  { title: '日志级别', description: '运行日志输出级别', control: 'select', value: 'INFO' },
]

// 恢复默认(危险操作,二次确认)
function confirmReset() {
  Modal.confirm({
    title: '恢复默认设置?',
    content: '将重置外观、常用等应用设置(不影响连接档案、扩展包与报文配置)。此操作不可撤销。',
    okText: '恢复默认',
    okType: 'danger',
    cancelText: '取消',
    onOk() {
      resetAppSettings()
    },
  })
}
</script>

<template>
  <div class="settings-group-title">扩展包</div>
  <div class="row-group">
    <SettingRow title="扩展包目录" mono-desc :description="storagePaths?.packs ?? '读取中…'">
      <template #action>
        <a-button size="small" :disabled="!storagePaths" @click="openDir(storagePaths?.packs)">打开目录</a-button>
      </template>
    </SettingRow>
  </div>
  <div class="settings-group-title">调试</div>
  <div class="row-group">
    <PlannedSettingRows :rows="plannedRows" />
  </div>
  <div class="settings-group-title danger-zone">危险操作</div>
  <div class="row-group">
    <SettingRow title="恢复默认设置" description="重置外观 / 常用等应用设置;不影响连接档案、扩展包与报文配置">
      <template #action>
        <a-button size="small" danger @click="confirmReset">恢复默认</a-button>
      </template>
    </SettingRow>
  </div>
</template>

<style scoped>
/* 分组:标题 + 行组 */
.settings-group-title {
  font-size: var(--fs-12);
  font-weight: 600;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin: 20px 0 2px;
}

.settings-group-title:first-child {
  margin-top: 6px;
}

.settings-group-title.danger-zone {
  color: var(--error);
}
</style>
