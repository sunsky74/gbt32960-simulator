<script setup lang="ts">
import { reactive } from 'vue'
import { message } from 'ant-design-vue'
import { PlayCircleOutlined } from '@ant-design/icons-vue'

const form = reactive({ ip: '0.0.0.0', port: 32960 })
const running = false

function startServer() {
  message.info('服务端 TCP 功能规划在后续阶段实现,当前为界面框架')
}

const connColumns = [
  { title: 'IP', dataIndex: 'ip' },
  { title: '端口', dataIndex: 'port', width: 100 },
  { title: '状态', dataIndex: 'state', width: 100 },
]

const conns: unknown[] = []

const packetColumns = [
  { title: '时间', dataIndex: 'time', width: 110 },
  { title: '来源', dataIndex: 'peer', width: 160 },
  { title: '报文 (HEX)', dataIndex: 'hex' },
]

const packets: unknown[] = []
</script>

<template>
  <div class="page-root server-page">
    <h2 class="page-title">服务端模式</h2>

    <div class="zone">
      <div class="zone-body">
        <p class="section-label">在本机启动 TCP Server,接收客户端连接与上报报文(第一阶段为界面框架)</p>
        <div class="listen-form">
          <span class="form-label">Listen IP</span>
          <a-input v-model:value="form.ip" size="small" class="form-input" placeholder="0.0.0.0" />
          <span class="form-label">Listen Port</span>
          <a-input-number v-model:value="form.port" size="small" :min="1" :max="65535" class="form-input" />
          <a-button type="primary" size="small" @click="startServer">
            <template #icon><PlayCircleOutlined /></template>
            启动服务
          </a-button>
        </div>
      </div>
    </div>

    <div class="zone">
      <div class="zone-body status-bar">
        <span>服务状态:</span>
        <a-tag>未启动</a-tag>
        <span class="status-hint">{{ running ? '' : '启动后此处展示运行状态与监听地址' }}</span>
      </div>
    </div>

    <div class="zone">
      <div class="zone-body">
        <p class="section-label">客户端连接</p>
        <a-table :data-source="conns" :columns="connColumns" size="small" :pagination="false">
          <template #emptyText>
            <div class="console-empty-title">暂无连接</div>
            <div class="console-empty-sub">服务启动后,接入的客户端将显示在此</div>
          </template>
        </a-table>
      </div>
    </div>

    <div class="zone">
      <div class="zone-body">
        <p class="section-label">接收报文</p>
        <a-table :data-source="packets" :columns="packetColumns" size="small" :pagination="false">
          <template #emptyText>
            <div class="console-empty-title">暂无报文</div>
            <div class="console-empty-sub">收到的客户端报文将显示在此</div>
          </template>
        </a-table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.server-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.page-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.section-label {
  color: var(--text-secondary);
  font-size: 12px;
  margin: 0 0 10px;
}

.listen-form {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.form-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.form-input {
  width: 160px;
}

.status-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
}

.status-hint {
  color: var(--text-tertiary);
  font-size: 12px;
}
</style>
