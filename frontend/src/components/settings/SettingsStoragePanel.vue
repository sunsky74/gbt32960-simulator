<script setup lang="ts">
// 数据与存储:后端只读路径展示 + 导出缓冲落库;其余为规划占位。
import SettingRow from './SettingRow.vue'
import PlannedSettingRows from './PlannedSettingRows.vue'
import { CTRL_W, type PlannedRow } from './planned'

defineProps<{
  /** 后端只读存储路径(null 表示仍在加载,页面挂载时取一次) */
  storagePaths: Record<string, string> | null
  /** 打开本地目录 */
  openDir: (path?: string) => void
  /** 导出缓冲条数保存到后端(失败时回滚显示值) */
  saveExportCap: (v: number | string | null) => void
}>()

// 控制台导出缓冲条数:值由页面持有,切分类不丢
const exportCap = defineModel<number | null>('exportCap', { required: true })

// 「规划中」占位行(禁用,仅展示)
const locationPlanned: PlannedRow[] = [
  { title: '自定义存储位置', description: '将配置 / 扩展包数据迁移到自定义目录', control: 'button', value: '修改位置' },
]
const historyPlanned: PlannedRow[] = [
  { title: '保存解析历史', description: '记录最近解析的报文,便于快速重放', control: 'switch' },
  { title: '最大历史记录数量', description: '超出后自动淘汰最旧记录', control: 'select', value: '100' },
]
const cleanPlanned: PlannedRow[] = [
  {
    title: '清理缓存与历史数据',
    description: '清除界面缓存与解析历史(不影响连接档案与扩展包)',
    control: 'button',
    value: '清理',
  },
]
</script>

<template>
  <div class="settings-group-title">存储位置</div>
  <div class="row-group">
    <SettingRow title="配置目录" mono-desc :description="storagePaths?.config ?? '读取中…'">
      <template #action>
        <a-button size="small" :disabled="!storagePaths" @click="openDir(storagePaths?.config)">打开目录</a-button>
      </template>
    </SettingRow>
    <SettingRow title="扩展包目录" mono-desc :description="storagePaths?.packs ?? '读取中…'">
      <template #action>
        <a-button size="small" :disabled="!storagePaths" @click="openDir(storagePaths?.packs)">打开目录</a-button>
      </template>
    </SettingRow>
    <PlannedSettingRows :rows="locationPlanned" />
  </div>
  <div class="settings-group-title">历史与缓存</div>
  <div class="row-group">
    <PlannedSettingRows :rows="historyPlanned" />
    <SettingRow title="控制台导出缓冲条数" description="客户端控制台导出保留的事件条数;数值越大内存占用越高">
      <template #action>
        <a-input-number
          v-model:value="exportCap"
          size="small"
          :min="1000"
          :max="500000"
          :style="{ width: CTRL_W }"
          :disabled="exportCap === null"
          @change="saveExportCap"
        />
      </template>
    </SettingRow>
    <PlannedSettingRows :rows="cleanPlanned" />
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
</style>
