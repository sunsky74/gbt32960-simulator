import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { FormInstance, Rule } from 'ant-design-vue/es/form'
import * as ConnectionService from '../../wailsjs/go/bridge/ConnectionService'
import { cfg } from './useConnConfig'
import { loadProfiles, refreshState, reloadSchemaForVersion, reloadSchemaPreservingGroups, savedBinding, store, syncSavedBinding } from '../state'

// 连接表单实例(顶栏按钮与连接配置卡共用同一份校验)
export const formRef = ref<FormInstance>()
export const testing = ref(false)

export const rules: Record<string, Rule[]> = {
  name: [{ required: true, message: '请输入连接名称', trigger: 'blur' }],
  host: [{ required: true, message: '请输入平台 IP 地址', trigger: 'blur' }],
  port: [
    { required: true, message: '请输入端口', trigger: 'blur' },
    { type: 'number', min: 1, max: 65535, message: '端口范围 1~65535', trigger: 'blur' },
  ],
  version: [{ required: true, message: '请选择报文版本', trigger: 'change' }],
  vin: [
    { required: true, message: '请输入 VIN', trigger: 'blur' },
    { len: 17, message: 'VIN 必须为 17 位', trigger: 'blur' },
  ],
  iccid: [{ max: 20, message: 'ICCID 最长 20 位', trigger: 'blur' }],
}

export const connectDisabled = computed(() => store.connBusy || store.connState !== 'idle')

export async function validateForm(): Promise<boolean> {
  try {
    await formRef.value?.validate()
    return true
  } catch {
    return false
  }
}

export async function testConnect() {
  testing.value = true
  const r = await ConnectionService.TestConnect(cfg)
  testing.value = false
  if (r.ok) {
    message.success(`连接正常: ${r.message}${r.elapsedMs ? ` (${r.elapsedMs}ms)` : ''}`)
  } else {
    message.error(`连接失败: ${r.message}`)
  }
}

export async function connect() {
  if (store.connBusy) return
  store.connBusy = true
  try {
    if (!(await validateForm())) return
    await ConnectionService.Connect(cfg)
    store.config = cfg
    if (cfg.version !== savedBinding.version) {
      await reloadSchemaForVersion(cfg.version)
    } else if (cfg.extensionPack !== savedBinding.pack) {
      await reloadSchemaPreservingGroups(cfg.version)
    }
    syncSavedBinding(cfg)
    await refreshState()
    await loadProfiles()
  } catch (e) {
    message.error(`连接失败: ${String(e)}`)
  } finally {
    store.connBusy = false
  }
}

export async function saveOnly() {
  if (!(await validateForm())) return
  try {
    await ConnectionService.SaveConfig(cfg)
    store.config = cfg
    if (cfg.version !== savedBinding.version) {
      await reloadSchemaForVersion(cfg.version)
    } else if (cfg.extensionPack !== savedBinding.pack) {
      await reloadSchemaPreservingGroups(cfg.version)
    }
    syncSavedBinding(cfg)
    message.success(`配置「${cfg.name}」已保存`)
    await loadProfiles()
  } catch (e) {
    message.error(`保存失败: ${String(e)}`)
  }
}

export async function onVersionChange() {
  await reloadSchemaForVersion(cfg.version)
}
