<script setup lang="ts">
// 外观:主题与界面动画 / 性能上限为真实生效项,其余为规划占位。
import { appSettings } from '../../composables/useAppSettings'
import SettingRow from './SettingRow.vue'
import PlannedSettingRows from './PlannedSettingRows.vue'
import { type PlannedRow } from './planned'

// 界面 → 性能上限档位选项(与 useAppSettings 的钳制区间一致)
const packetStreamCapOptions = [100, 200, 500, 1000, 2000].map((v) => ({ value: v, label: String(v) }))
const consoleEventCapOptions = [1000, 5000, 10000, 50000].map((v) => ({ value: v, label: String(v) }))

// 「规划中」占位行(禁用,仅展示)
const plannedRows: PlannedRow[] = [
  { title: '界面密度', description: '紧凑 / 默认 / 宽松三档全局间距', control: 'select', value: '默认' },
  { title: '字体大小', description: '小 / 默认 / 大三档界面字号', control: 'select', value: '默认' },
  { title: '显示侧边栏文字', description: '关闭后侧边导航仅显示图标', control: 'switch' },
  { title: '自动折叠侧边栏', description: '窗口较窄时自动收起侧边导航', control: 'switch' },
]
</script>

<template>
  <div class="settings-group-title">主题</div>
  <div class="row-group">
    <SettingRow title="应用程序主题" description="选择界面配色;「跟随系统」随操作系统外观实时切换">
      <template #action>
        <a-radio-group v-model:value="appSettings.themeMode" button-style="solid" size="small">
          <a-radio-button value="dark">深色</a-radio-button>
          <a-radio-button value="light">浅色</a-radio-button>
          <a-radio-button value="auto">跟随系统</a-radio-button>
        </a-radio-group>
      </template>
    </SettingRow>
  </div>
  <div class="settings-group-title">界面</div>
  <div class="row-group">
    <SettingRow title="启用界面动画" description="关闭后抑制全局面板过渡与动画,降低视觉噪声">
      <template #action>
        <a-switch v-model:checked="appSettings.animations" size="small" />
      </template>
    </SettingRow>
    <SettingRow title="服务端报文流保留行数" description="服务端模式报文流最多渲染的行数;数值越大渲染与内存开销越高">
      <template #action>
        <a-select
          v-model:value="appSettings.packetStreamCap"
          size="small"
          :options="packetStreamCapOptions"
          class="w-180"
        />
      </template>
    </SettingRow>
    <SettingRow title="控制台保留条数" description="客户端控制台最多保留的事件条数;数值越大内存占用越高">
      <template #action>
        <a-select
          v-model:value="appSettings.consoleEventCap"
          size="small"
          :options="consoleEventCapOptions"
          class="w-180"
        />
      </template>
    </SettingRow>
    <PlannedSettingRows :rows="plannedRows" />
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
