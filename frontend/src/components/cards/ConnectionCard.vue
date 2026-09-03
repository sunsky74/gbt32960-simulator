<script setup lang="ts">
import { computed, onMounted } from 'vue'
import CollapsibleCard from '../layout/CollapsibleCard.vue'
import { cfg } from '../../composables/useConnConfig'
import { formRef, rules, saveOnly, onVersionChange } from '../../composables/useConnActions'
import * as ExtService from '../../../wailsjs/go/bridge/ExtService'
import { store } from '../../state'

async function refreshPacks() {
  try {
    store.packs = await ExtService.ListPacks()
  } catch {
    store.packs = []
  }
}

const packOptions = computed(() =>
  store.packs.map((p) => ({
    value: p.id,
    label: p.enabled ? `${p.label} (${p.baseVersion})` : `${p.label} (${p.baseVersion}) · 已停用`,
    // 停用包不可新选;当前已绑定的停用包保持可选,便于在 UI 中看到并换绑
    disabled: !p.enabled && p.id !== cfg.extensionPack,
  })),
)

onMounted(refreshPacks)
</script>

<template>
  <CollapsibleCard title="连接配置">
    <a-form ref="formRef" :model="cfg" :rules="rules" layout="vertical" class="mini-form">
      <a-form-item label="连接名称" name="name">
        <a-input v-model:value="cfg.name" placeholder="如: 本地网关" />
      </a-form-item>

      <a-form-item label="平台地址" required>
        <a-space-compact class="addr-compact">
          <a-form-item name="host" noStyle>
            <a-input v-model:value="cfg.host" placeholder="127.0.0.1" style="width: 100%" />
          </a-form-item>
          <a-form-item name="port" noStyle>
            <a-input-number v-model:value="cfg.port" :min="1" :max="65535" placeholder="32960" style="width: 110px" />
          </a-form-item>
        </a-space-compact>
      </a-form-item>

      <a-form-item label="报文版本" name="version">
        <a-select v-model:value="cfg.version" @change="onVersionChange">
          <a-select-option value="2016">GB/T 32960-2016</a-select-option>
          <a-select-option value="2025">GB/T 32960-2025</a-select-option>
        </a-select>
      </a-form-item>

      <a-form-item label="扩展包" name="extensionPack">
        <a-select
          v-model:value="cfg.extensionPack"
          :options="packOptions"
          allow-clear
          placeholder="请选择协议扩展包"
          @dropdown-visible-change="(open: boolean) => open && refreshPacks()"
        />
      </a-form-item>

      <a-form-item label="心跳间隔" name="heartbeatSec">
        <a-input-number
          v-model:value="cfg.heartbeatSec"
          :min="0"
          :max="3600"
          addon-after="秒"
          style="width: 180px"
        />
        <span class="field-hint">0 = 不发心跳 (0x07)</span>
      </a-form-item>

      <a-space direction="vertical" size="small" class="switch-row">
        <a-switch v-model:checked="cfg.autoClockSync" size="small" />
        <span>登录后自动校时 (0x08)</span>
      </a-space>
      <a-space direction="vertical" size="small" class="switch-row">
        <a-switch v-model:checked="cfg.autoReconnect" size="small" />
        <span>断线自动重连</span>
      </a-space>

      <a-collapse ghost class="tls-collapse">
        <a-collapse-panel key="tls" header="TLS / SSL 证书">
          <a-form layout="vertical" class="mini-form">
            <a-space direction="vertical" size="small" class="switch-row">
              <a-switch v-model:checked="cfg.tls.enabled" size="small" />
              <span>启用 TLS</span>
            </a-space>
            <a-space direction="vertical" size="small" class="switch-row">
              <a-switch v-model:checked="cfg.tls.insecure" size="small" />
              <span>跳过证书校验(仅调试)</span>
            </a-space>
            <a-form-item label="ServerName(SNI)" class="tls-field">
              <a-input v-model:value="cfg.tls.serverName" placeholder="可选" />
            </a-form-item>
            <a-form-item label="CA 证书" class="tls-field">
              <a-textarea v-model:value="cfg.tls.ca" :rows="2" placeholder="PEM 内容或文件路径" />
            </a-form-item>
            <a-form-item label="客户端证书" class="tls-field">
              <a-textarea v-model:value="cfg.tls.clientCert" :rows="2" placeholder="PEM 内容或文件路径(双向认证)" />
            </a-form-item>
            <a-form-item label="客户端私钥" class="tls-field">
              <a-textarea v-model:value="cfg.tls.clientKey" :rows="2" placeholder="PEM 内容或文件路径" />
            </a-form-item>
          </a-form>
        </a-collapse-panel>
      </a-collapse>
    </a-form>
  </CollapsibleCard>

  <div class="right-action-bar">
    <a-space wrap>
      <a-button size="small" @click="saveOnly">保存配置</a-button>
    </a-space>
  </div>
</template>
