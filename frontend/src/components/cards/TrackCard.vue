<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import CollapsibleCard from '../layout/CollapsibleCard.vue'

const fileName = ref('')

function onFileChange(info: { file: { name: string } }) {
  fileName.value = info.file.name
  message.info('轨迹文件已选择,轨迹回放功能将在后续版本支持')
}
</script>

<template>
  <CollapsibleCard title="轨迹" :default-open="false">
    <template #extra>
      <a-tag>规划中</a-tag>
    </template>
    <a-upload accept=".gpx" :show-upload-list="false" :before-upload="() => false" @change="onFileChange">
      <a-button size="small" block>导入轨迹文件 (.gpx)</a-button>
    </a-upload>
    <div v-if="fileName" class="track-file">已选择: {{ fileName }}</div>
  </CollapsibleCard>
</template>
