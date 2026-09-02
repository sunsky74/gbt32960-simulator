<script setup lang="ts">
import PackManager from '../components/PackManager.vue'
</script>

<template>
  <div class="page">
    <div class="section-title">扩展包管理</div>
    <p class="section-hint">导入 JSON 扩展包(实时私有数据单元 / 私有命令字),在连接配置中绑定后即可使用。</p>
    <PackManager />
  </div>
</template>

<style scoped>
.page {
  padding: 16px;
}
.section-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 4px;
}
.section-hint {
  color: #888;
  font-size: 12px;
  margin-bottom: 12px;
}
</style>
