<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import * as ExtService from '../../wailsjs/go/bridge/ExtService'
import { cfg } from '../composables/useConnConfig'
import { reloadSchemaPreservingGroups, store } from '../state'

const loading = ref(false)

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

async function importPack() {
  try {
    const path = await ExtService.PickPackFile()
    if (!path) return
    const info = await ExtService.ImportPack(path)
    message.success(`已导入「${info.label}」`)
    await refresh()
  } catch (e) {
    message.error(String(e))
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
        if (store.config?.extensionPack === id) {
          store.config.extensionPack = ''
          cfg.extensionPack = ''
          await reloadSchemaPreservingGroups(store.config?.version ?? '2016')
        }
        await refresh()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}

const boundId = computed(() => store.config?.extensionPack)

onMounted(refresh)
</script>

<template>
  <div class="pack-manager">
    <div class="pack-actions">
      <a-button size="small" type="primary" @click="importPack">导入扩展包</a-button>
      <a-button size="small" @click="refresh" :loading="loading">重新扫描</a-button>
    </div>
    <a-empty v-if="store.packs.length === 0" description="暂无扩展包。导入一个 JSON 扩展包即可在连接档案中绑定。" />
    <a-table
      v-else
      :data-source="store.packs"
      :pagination="false"
      size="small"
      row-key="id"
    >
      <a-table-column title="名称" data-index="label" />
      <a-table-column title="ID" data-index="id" />
      <a-table-column title="厂商" data-index="vendor" />
      <a-table-column title="版本" data-index="baseVersion" width="70" />
      <a-table-column title="实时单元" data-index="unitCount" width="80" />
      <a-table-column title="命令" data-index="commandCount" width="70" />
      <a-table-column title="状态" width="90">
        <template #default="{ record }">
          <a-tag v-if="record.id === boundId" color="green">已绑定</a-tag>
          <span v-else class="dim">未绑定</span>
        </template>
      </a-table-column>
      <a-table-column title="操作" width="80">
        <template #default="{ record }">
          <a-button size="small" type="text" danger @click="deletePack(record.id, record.label)">删除</a-button>
        </template>
      </a-table-column>
    </a-table>
  </div>
</template>

<style scoped>
.pack-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.dim {
  color: var(--text-secondary, #999);
}
</style>
