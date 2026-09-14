<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { GroupSchema } from '../api/backend'
import { stateToPayload } from '../api/backend'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import { store } from '../state'
import {
  bitOptions,
  bitsArrayOf,
  defaultFor,
  hexOf,
  numOf,
  setBitsArray,
  setEnum,
  setHex,
  setNum,
} from '../composables/useFieldHelpers'

const reports = ref<Record<string, { on: boolean; interval: number }>>({})

function ensureGroup(g: GroupSchema) {
  if (!store.groups[g.key]) {
    const row: Record<string, unknown> = {}
    for (const f of g.fields) row[f.key] = defaultFor(f)
    store.groups[g.key] = { enabled: g.enabled, rows: [row] }
  }
  return store.groups[g.key]
}

function addRow(g: GroupSchema) {
  const grp = ensureGroup(g)
  if (!g.multiple || (g.maxRows && grp.rows.length >= g.maxRows)) return
  const row: Record<string, unknown> = {}
  for (const f of g.fields) row[f.key] = defaultFor(f)
  grp.rows.push(row)
}

function removeRow(g: GroupSchema, idx: number) {
  const grp = store.groups[g.key]
  if (!grp || (g.multiple && grp.rows.length <= 1)) return
  grp.rows.splice(idx, 1)
}

// extHint 空态三态文案:未绑定 / 版本不匹配 / 未声明 commands(消化 oracle 审核 D 的静默问题)。
const extHint = computed(() => {
  const packId = store.config?.extensionPack
  if (!packId) return '绑定含 commands 的扩展包后,在此配置与发送私有命令'
  const info = store.packs.find((p) => p.id === packId)
  if (!info) return `扩展包「${packId}」未找到,请在设置页重新导入`
  if (info.baseVersion !== (store.config?.version ?? '2016')) {
    return `扩展包「${info.label}」基准版本 ${info.baseVersion} 与当前档案版本 ${store.config?.version} 不匹配,请调整档案版本或换绑其他包`
  }
  return `扩展包「${info.label}」未声明 commands 段,无可配置的私有命令`
})

function cmdKeyOf(g: GroupSchema): string {
  return g.key.split(':')[0]
}

function reportOf(key: string) {
  if (!reports.value[key]) reports.value[key] = { on: false, interval: 10 }
  return reports.value[key]
}

async function saveAll() {
  const order = [...store.schema.map((g) => g.key), ...store.extSchema.map((g) => g.key)]
  await MessageService.SaveGroups(stateToPayload(store.groups, order))
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
    <a-empty v-if="store.extSchema.length === 0" :description="extHint" />
    <a-collapse v-else ghost expand-icon-position="end" class="group-collapse">
      <a-collapse-panel v-for="g in store.extSchema" :key="g.key">
        <template #header>
          <span class="group-title">{{ g.title }}</span>
        </template>
        <div class="group-body">
          <div class="extcmd-actions">
            <a-button size="small" type="primary" @click="send(g)">发送</a-button>
          </div>
          <div v-for="(row, ri) in ensureGroup(g).rows" :key="ri" class="group-row">
            <div class="row-head">
              <span v-if="g.multiple" class="row-label">第 {{ ri + 1 }} 行</span>
              <a-button
                v-if="g.multiple && ensureGroup(g).rows.length > 1"
                size="small"
                type="text"
                danger
                @click="removeRow(g, ri)"
                >删除行</a-button
              >
            </div>
            <div class="fields-grid">
              <template v-for="f in g.fields" :key="f.key">
                <div v-if="f.kind === 'enum'" class="field">
                  <span class="field-label">{{ f.label }}</span>
                  <a-select
                    :value="numOf(row, f.key)"
                    size="small"
                    :options="f.enum?.map((e) => ({ value: e.value, label: e.label })) ?? []"
                    @change="(v: unknown) => setEnum(row, f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'int' || f.kind === 'float'" class="field">
                  <span class="field-label"
                    >{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span
                  >
                  <a-input-number
                    :value="numOf(row, f.key)"
                    size="small"
                    :step="f.kind === 'int' ? 1 : 0.1"
                    :min="f.min"
                    :max="f.max"
                    style="width: 100%"
                    @change="(v: number | string | null | undefined) => setNum(row, f.key, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'bytes'" class="field">
                  <span class="field-label"
                    >{{ f.label }}<em v-if="f.length"> ({{ f.length }}B hex)</em></span
                  >
                  <a-input
                    :value="hexOf(row, f.key)"
                    class="hex-input"
                    size="small"
                    :placeholder="f.length ? `${f.length * 2} 个 hex 字符` : 'hex'"
                    @update:value="(v: string) => setHex(row, f.key, f, v)"
                  />
                </div>
                <div v-else-if="f.kind === 'bitgroup'" class="field field-bits">
                  <span class="field-label">{{ f.label }}</span>
                  <a-checkbox-group
                    :value="bitsArrayOf(row, f)"
                    :options="bitOptions(f)"
                    class="bits-group"
                    @change="(vals: Array<string | number | boolean>) => setBitsArray(row, f, vals)"
                  />
                </div>
              </template>
            </div>
          </div>
          <a-button v-if="g.multiple" size="small" type="dashed" block @click="addRow(g)">＋ 添加一行</a-button>
          <div class="extcmd-actions">
            <a-switch
              v-model:checked="reportOf(cmdKeyOf(g)).on"
              size="small"
              @change="(v: unknown) => toggleReport(g, v === true)"
            />
            <span class="report-label">周期上报</span>
            <a-input-number
              v-model:value="reportOf(cmdKeyOf(g)).interval"
              :min="1"
              :max="3600"
              size="small"
              addon-after="秒"
              class="w-130"
              @change="() => onReportIntervalChange(g)"
            />
          </div>
        </div>
      </a-collapse-panel>
    </a-collapse>
  </div>
</template>

<style scoped>
/* row-head/row-label/row-bytes 复用全局样式(group-row 体系),不再 scoped 重写 */
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
