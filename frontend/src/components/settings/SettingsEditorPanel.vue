<script setup lang="ts">
// 编辑器:IDE 化预留配置,当前全部为规划占位。
import PlannedSettingRows from './PlannedSettingRows.vue'
import type { PlannedRow } from './planned'

// 「规划中」占位行(禁用,仅展示)
const plannedRows: PlannedRow[] = [
  { title: '编辑器字体', description: 'HEX / JSON 数据视图使用的等宽字体', control: 'select', value: '系统默认' },
  { title: '编辑器字号', description: '数据视图字号(11 ~ 16px)', control: 'select', value: '12' },
  { title: 'Tab 宽度', description: 'JSON 编辑缩进宽度', control: 'select', value: '2' },
  { title: '自动换行', description: '长行折行显示而非横向滚动', control: 'switch' },
  { title: '显示行号', description: '数据编辑视图显示行号', control: 'switch' },
  { title: 'JSON 自动格式化', description: '粘贴 / 保存 JSON 时自动整理格式', control: 'switch' },
  { title: 'HEX 显示方式', description: '字节视图分组大小与大小写偏好', control: 'select', value: '大写 · 8 字节' },
]
</script>

<template>
  <p class="cat-hint">以下为编辑器 / 数据视图的预留配置,将随 IDE 化演进一步步接入,当前尚未生效。</p>
  <div class="row-group">
    <PlannedSettingRows :rows="plannedRows" />
  </div>
</template>

<style scoped>
/* 分类说明 */
.cat-hint {
  margin: 10px 0 6px;
  font-size: var(--fs-12);
  line-height: 1.7;
  color: var(--text-tertiary);
}
</style>
