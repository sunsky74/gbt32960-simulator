<script setup lang="ts">
// 快捷键:规划中的快捷键方案展示表,当前尚未生效。
import { PLANNED } from './planned'

const shortcuts = [
  { action: '解析报文', key: 'Ctrl / ⌘ + Enter' },
  { action: '清空报文', key: 'Ctrl / ⌘ + L' },
  { action: '复制 HEX', key: 'Ctrl / ⌘ + Shift + C' },
  { action: '保存配置', key: 'Ctrl / ⌘ + S' },
  { action: '切换页面', key: 'Ctrl / ⌘ + 1~5' },
]
</script>

<template>
  <p class="cat-hint">以下为规划的快捷键方案;快捷键功能将在后续版本开放,当前尚未生效。</p>
  <table class="keys-table">
    <thead>
      <tr>
        <th>操作</th>
        <th>快捷键</th>
        <th>状态</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="s in shortcuts" :key="s.action">
        <td>{{ s.action }}</td>
        <td>
          <span class="key-cap mono">{{ s.key }}</span>
        </td>
        <td>
          <span class="row-badge">{{ PLANNED }}</span>
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
/* 分类说明 */
.cat-hint {
  margin: 10px 0 6px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-tertiary);
}

/* 快捷键表 */
.keys-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  margin-top: 6px;
}

.keys-table th {
  text-align: left;
  font-weight: 500;
  color: var(--text-tertiary);
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-subtle);
}

.keys-table td {
  padding: 8px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

.key-cap {
  font-size: 11px;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 1px 6px;
}

.row-badge {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: 3px;
  color: var(--text-tertiary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
}

.mono {
  font-family: var(--font-mono);
}
</style>
