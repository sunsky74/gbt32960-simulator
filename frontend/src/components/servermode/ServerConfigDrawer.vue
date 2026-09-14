<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { CaretRightOutlined, StopOutlined } from '@ant-design/icons-vue'
import * as ServerService from '../../../wailsjs/go/bridge/ServerService'

const props = defineProps<{
  running: boolean
}>()

// ---------- 服务配置抽屉:表单状态 + 启停/空闲设置动作 ----------
const cfg = reactive({
  ip: '127.0.0.1',
  port: 32960,
  idleEnabled: true,
  idleSeconds: 60,
  maxConns: 64,
  maxFrameBytes: 8192,
  logLines: 500,
  maxVinsPerConn: 128,
})
const cfgOpen = ref(false)

async function loadCfg() {
  try {
    const saved = await ServerService.LoadConfig()
    Object.assign(cfg, saved)
  } catch (e) {
    message.error('读取服务端配置失败: ' + String(e))
  }
}

async function start(): Promise<boolean> {
  const loopback = cfg.ip === '127.0.0.1' || cfg.ip === 'localhost'
  const doStart = async (force: boolean): Promise<boolean> => {
    try {
      const st = await ServerService.Start({ ...cfg }, force)
      if (st.running) {
        message.success('服务已启动 ' + st.listenAddr)
        return true
      }
      return false
    } catch (e) {
      message.error(String(e))
      return false
    }
  }
  if (loopback) return doStart(false)
  return new Promise((resolve) => {
    Modal.confirm({
      title: '监听地址非环回',
      content: `即将监听 ${cfg.ip}:${cfg.port},局域网内任何设备都可连接(协议无认证)。确认继续?`,
      okText: '继续监听',
      cancelText: '取消',
      onOk: async () => resolve(await doStart(true)),
      onCancel: () => resolve(false),
    })
  })
}

async function startFromDrawer() {
  if (await start()) cfgOpen.value = false
}

async function stop() {
  try {
    await ServerService.Stop()
  } catch (e) {
    message.error(String(e))
  }
}

// 打开抽屉(顶栏「服务配置」按钮)
function open() {
  cfgOpen.value = true
}

// 运行中即时下发空闲断开设置(AC-10)
watch(
  () => [cfg.idleEnabled, cfg.idleSeconds] as const,
  async ([enabled, seconds]) => {
    if (!props.running) return
    try {
      await ServerService.UpdateIdle(enabled, seconds)
    } catch (e) {
      message.error('空闲断开设置下发失败: ' + String(e))
    }
  },
)

onMounted(() => {
  // 加载已保存配置(与原页面 onMounted 中的 loadCfg 行为一致)
  void loadCfg()
})

defineExpose({ open, start, stop })
</script>

<template>
  <!-- 服务配置抽屉:Listen/空闲设置不再常驻顶栏 -->
  <a-drawer v-model:open="cfgOpen" title="服务配置" placement="right" :width="440">
    <div class="cfg-form">
      <div class="cfg-item">
        <span class="form-label">Listen IP</span>
        <a-input v-model:value="cfg.ip" size="small" placeholder="127.0.0.1" />
      </div>
      <div class="cfg-item">
        <span class="form-label">Listen Port</span>
        <a-input-number v-model:value="cfg.port" size="small" :min="1" :max="65535" class="cfg-num" />
      </div>
      <div class="cfg-item">
        <span class="form-label">空闲断开</span>
        <a-switch v-model:checked="cfg.idleEnabled" size="small" />
      </div>
      <div class="cfg-item">
        <span class="form-label">空闲秒数</span>
        <a-input-number
          v-model:value="cfg.idleSeconds"
          size="small"
          :min="5"
          :max="3600"
          class="cfg-num"
          :disabled="!cfg.idleEnabled"
        />
      </div>
      <div class="cfg-group-title">高级参数</div>
      <p class="cfg-group-hint">停止服务后修改,重新启动生效</p>
      <div class="cfg-item">
        <span class="form-label">最大连接数</span>
        <a-input-number v-model:value="cfg.maxConns" size="small" :min="1" :max="512" class="cfg-num" />
      </div>
      <div class="cfg-item">
        <span class="form-label">单帧上限(字节)</span>
        <a-input-number v-model:value="cfg.maxFrameBytes" size="small" :min="512" :max="65536" class="cfg-num" />
      </div>
      <div class="cfg-item">
        <span class="form-label">日志保留行数</span>
        <a-input-number v-model:value="cfg.logLines" size="small" :min="100" :max="10000" class="cfg-num" />
      </div>
      <div class="cfg-item">
        <span class="form-label">平台链路 VIN 上限</span>
        <a-input-number v-model:value="cfg.maxVinsPerConn" size="small" :min="1" :max="1024" class="cfg-num" />
      </div>
    </div>
    <a-alert
      v-if="running"
      type="info"
      show-icon
      message="服务运行中:修改空闲断开设置将立即下发生效"
      class="cfg-hint"
    />
    <a-alert v-else type="info" show-icon message="启动后客户端可连接此地址上报报文" class="cfg-hint" />
    <div class="cfg-actions">
      <a-button v-if="!running" type="primary" size="small" @click="startFromDrawer">
        <template #icon><CaretRightOutlined /></template>
        启动服务
      </a-button>
      <a-button v-else danger size="small" @click="stop">
        <template #icon><StopOutlined /></template>
        停止服务
      </a-button>
    </div>
  </a-drawer>
</template>

<style scoped>
/* 配置抽屉分组标题与提示(高级参数) */
.cfg-group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-tertiary);
  letter-spacing: 0.5px;
  margin-top: 4px;
}

.cfg-group-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
