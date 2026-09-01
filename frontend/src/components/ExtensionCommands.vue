<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import type { GroupSchema } from '../api/backend'
import { stateToPayload } from '../api/backend'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import { store } from '../state'
import {
  bitOptions, bitsArrayOf, defaultFor, hexOf, numOf, setBitsArray, setEnum, setHex, setNum,
} from '../composables/useFieldHelpers'

const reports = ref<Record<string, { on: boolean; interval: number }>>({})

function ensureGroup(g: GroupSchema) {
  if (!store.groups[g.key]) {
    const row: Record<string, unknown> = {}
    for (const f of g.fields) row[f.key] = defaultFor(f)
    store.groups[g.key] = { enabled: true, rows: [row] }
  }
  return store.groups[g.key]
}

function cmdKeyOf(g: GroupSchema): string {
  return g.key.split(':')[0]
}

function reportOf(key: string) {
  if (!reports.value[key]) reports.value[key] = { on: false, interval: 10 }
  return reports.value[key]
}

async function saveAll() {
  try {
    const order = [...store.schema.map((g) => g.key), ...store.extSchema.map((g) => g.key)]
    await MessageService.SaveGroups(stateToPayload(store.groups, order))
  } catch (e) {
    message.error('保存失败: ' + String(e))
  }
}

async function send(g: GroupSchema) {
  try {
    await saveAll()
    await MessageService.SendExtension(cmdKeyOf(g))
    message.success(`「${g.title}」已发送`)
  } catch (e) {
    message.error('发送失败: ' + String(e))
  }
}

async function toggleReport(g: GroupSchema, checked: boolean) {
  const r = reportOf(cmdKeyOf(g))
  try {
    await saveAll()
    await MessageService.SetExtAutoReport(cmdKeyOf(g), checked, r.interval)
    r.on = checked
    message.success(checked ? `周期上报已开启 (每 ${r.interval}s)` : '周期上报已停止')
  } catch (e) {
    r.on = false // 评审 P2-1:失败回滚,与 RealTimePanel 的 toggleAutoReport 行为一致
    message.error(String(e))
  }
}

async function onReportIntervalChange(g: GroupSchema) {
  const r = reportOf(cmdKeyOf(g))
  if (!r.on) return
  try {
    await MessageService.SetExtAutoReport(cmdKeyOf(g), true, r.interval)
  } catch (e) {
    message.error(String(e))
  }
}
</script>

<template>
  <div class="zone-body">
    <a-empty v-if="store.extSchema.length === 0" description="绑定含 commands 的扩展包后,在此配置与发送私有命令" />
    <a-collapse v-else ghost expand-icon-position="end" class="group-collapse">
      <a-collapse-panel v-for="g in store.extSchema" :key="g.key">
        <template #header>
          <span class="group-title">{{ g.title }}</span>
        </template>
        <div class="group-body">
          <div class="extcmd-actions">
            <a-button size="small" type="primary" @click="send(g)">发送</a-button>
          </div>
          <div class="group-row">
            <div class="fields-grid">
              <template v-for="f in g.fields" :key="f.key">
                <div v-if="f.kind === 'enum'" class="field">
                  <span class="field-label">{{ f.label }}</span>
                  <a-select
                    :value="numOf(ensureGroup(g).rows[0], f.key)"
                    size="small"
                    :options="f.enum?.map((e) => ({ value: e.value, label: e.label })) ?? []"
                    @change="(v: unknown) => setEnum(ensureGroup(g).rows[0], f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'int' || f.kind === 'float'" class="field">
                  <span class="field-label">{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span>
                  <a-input-number
                    :value="numOf(ensureGroup(g).rows[0], f.key)"
                    size="small"
                    :step="f.kind === 'int' ? 1 : 0.1"
                    :min="f.min"
                    :max="f.max"
                    style="width: 100%"
                    @change="(v: number | string | null | undefined) => setNum(ensureGroup(g).rows[0], f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'bytes'" class="field">
                  <span class="field-label">{{ f.label }}<em v-if="f.length"> ({{ f.length }}B hex)</em></span>
                  <a-input
                    :value="hexOf(ensureGroup(g).rows[0], f.key)"
                    class="hex-input"
                    size="small"
                    :placeholder="f.length ? `${f.length * 2} 个 hex 字符` : 'hex'"
                    @update:value="(v: string) => setHex(ensureGroup(g).rows[0], f.key, f, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'bitgroup'" class="field field-bits">
                  <span class="field-label">{{ f.label }}</span>
                  <a-checkbox-group
                    :value="bitsArrayOf(ensureGroup(g).rows[0], f)"
                    :options="bitOptions(f)"
                    class="bits-group"
                    @change="(vals: Array<string | number | boolean>) => setBitsArray(ensureGroup(g).rows[0], f, vals)"
                  />
                </div>
              </template>
            </div>
          </div>
          <div class="extcmd-actions">
            <a-switch v-model:checked="reportOf(cmdKeyOf(g)).on" size="small" @change="(v: unknown) => toggleReport(g, v === true)" />
            <span class="report-label">周期上报</span>
            <a-input-number
              v-model:value="reportOf(cmdKeyOf(g)).interval"
              :min="1"
              :max="3600"
              size="small"
              addon-after="秒"
              style="width: 130px"
              @change="() => onReportIntervalChange(g)"
            />
          </div>
        </div>
      </a-collapse-panel>
    </a-collapse>
  </div>
</template>

<style scoped>
.extcmd-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.hex-input :deep(input) {
  font-family: var(--font-mono);
}
</style>
