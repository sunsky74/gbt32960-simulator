<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import type { FieldSchema, GroupSchema } from '../api/backend'
import { stateToPayload } from '../api/backend'
import * as MessageService from '../../wailsjs/go/bridge/MessageService'
import { store } from '../state'
import {
  addArrayItem, arrayValues, bitOptions, bitsArrayOf, bitsObjOf, boolOf, defaultFor,
  hexOf, numOf, setArrayValue, setBitsArray, setBool, setEnum, setHex, setNum,
} from '../composables/useFieldHelpers'
import ExtensionCommands from './ExtensionCommands.vue'

const activeKeys = ref<string[]>([])

// schema 加载后默认展开启用的组
watch(
  () => store.schema,
  (schema) => {
    if (schema.length > 0) {
      activeKeys.value = schema.filter((g) => g.enabled).map((g) => g.key)
    }
  },
  { immediate: true },
)

const previewOpen = ref(false)
const previewHex = ref('')
const previewCmd = ref('')

const autoReport = ref(false)
const autoInterval = ref(10)
const reissueCount = ref(10)
const reissueOffset = ref(180)
const reissueInterval = ref(10)

const schemaOrder = computed(() => store.schema.map((g) => g.key))
const online = computed(() => store.connState === 'online')

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
  for (const f of g.fields) {
    if (f.kind === 'int' && f.key === 'seq') row[f.key] = grp.rows.length + 1
    else row[f.key] = defaultFor(f)
  }
  grp.rows.push(row)
}

function removeRow(g: GroupSchema, idx: number) {
  const grp = store.groups[g.key]
  if (!grp || (g.multiple && grp.rows.length <= 1)) return
  grp.rows.splice(idx, 1)
}

// ---------- 字段取值/设值 ----------

function removeArrayItem(row: Record<string, unknown>, key: string, idx: number) {
  const arr = arrayValues(row, key)
  arr.splice(idx, 1)
  row[key] = arr
}

function enumOptions(field: FieldSchema) {
  return (field.enum ?? []).map((e) => ({ label: e.label, value: e.value }))
}

// ---------- 动作 ----------

async function saveGroups() {
  try {
    await MessageService.SaveGroups(stateToPayload(store.groups, schemaOrder.value))
    message.success('报文配置已保存并校验通过')
  } catch (e) {
    message.error('校验失败: ' + String(e))
  }
}

async function preview() {
  try {
    await MessageService.SaveGroups(stateToPayload(store.groups, schemaOrder.value))
    const r = await MessageService.Preview()
    previewCmd.value = r.cmd
    previewHex.value = r.hex
    previewOpen.value = true
  } catch (e) {
    message.error('预览失败: ' + String(e))
  }
}

async function sendRealtime() {
  try {
    await MessageService.SaveGroups(stateToPayload(store.groups, schemaOrder.value))
    await MessageService.SendRealtime()
    message.success('实时信息已发送 (0x02)')
  } catch (e) {
    message.error('发送失败: ' + String(e))
  }
}

async function sendReissue() {
  try {
    await MessageService.SaveGroups(stateToPayload(store.groups, schemaOrder.value))
    await MessageService.SendReissue(reissueCount.value, reissueOffset.value, reissueInterval.value)
    message.success(
      `补发完成 (0x03 × ${reissueCount.value}, 时间 ${reissueOffset.value}s 前起每 ${reissueInterval.value}s 一条)`,
    )
  } catch (e) {
    message.error('补发失败: ' + String(e))
  }
}

async function toggleAutoReport(checked: boolean) {
  try {
    await MessageService.SetAutoReport(checked, autoInterval.value)
    message.success(checked ? `周期上报已开启 (每 ${autoInterval.value}s)` : '周期上报已停止')
  } catch (e) {
    autoReport.value = false
    message.error(String(e))
  }
}

async function onIntervalChange(v: number | string | null | undefined) {
  autoInterval.value = typeof v === 'number' ? v : Number(v) || 10
  if (autoReport.value) {
    await MessageService.SetAutoReport(true, autoInterval.value)
  }
}
</script>

<template>
  <div class="zone zone-top">
    <a-tabs size="small" class="zone-tabs">
      <template #tabBarExtraContent>
        <div class="header-actions">
          <a-space :size="8">
            <a-button size="small" type="primary" :disabled="!online" @click="sendRealtime">发送 (0x02)</a-button>
            <a-button size="small" @click="preview">预览 HEX</a-button>
            <a-button size="small" @click="saveGroups">保存配置</a-button>
          </a-space>
        </div>
      </template>

      <a-tab-pane key="realtime" tab="实时数据">
        <div class="zone-body">
          <div class="report-bar">
            <span class="report-item">
              <a-switch v-model:checked="autoReport" size="small" @change="toggleAutoReport" />
              <span class="report-label">周期上报</span>
            </span>
            <span class="report-label">间隔</span>
            <a-input-number
              v-model:value="autoInterval"
              :min="1"
              :max="3600"
              size="small"
              addon-after="秒"
              class="w-130"
              @change="onIntervalChange"
            />
          </div>

          <a-collapse v-model:activeKey="activeKeys" ghost expand-icon-position="end" class="group-collapse">
            <a-collapse-panel v-for="g in store.schema" :key="g.key">
              <template #header>
                <span class="group-title">{{ g.title }}</span>
                <a-tag v-if="g.multiple" color="blue" class="row-tag">{{ ensureGroup(g).rows.length }} 行</a-tag>
              </template>
              <template #extra>
                <a-switch v-model:checked="ensureGroup(g).enabled" size="small" @click.stop />
              </template>

              <div class="group-body">
                <div v-for="(row, ri) in ensureGroup(g).rows" :key="ri" class="group-row">
                  <div v-if="g.multiple" class="row-head">
                    <span class="row-label">第 {{ ri + 1 }} 行</span>
                    <a-button size="small" type="text" danger @click="removeRow(g, ri)">删除行</a-button>
                  </div>
                  <div class="fields-grid">
                    <template v-for="f in g.fields" :key="f.key">
                      <div v-if="f.kind === 'enum'" class="field">
                        <span class="field-label">{{ f.label }}</span>
                        <a-select
                          :value="numOf(row, f.key)"
                          size="small"
                          :options="enumOptions(f)"
                          @change="(v: unknown) => setEnum(row, f.key, v)"
                        />
                      </div>

                      <div v-else-if="f.kind === 'int' || f.kind === 'float'" class="field">
                        <span class="field-label">{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span>
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
                        <span class="field-label">{{ f.label }}<em v-if="f.length"> ({{ f.length }}B hex)</em></span>
                        <a-input
                          :value="hexOf(row, f.key)"
                          class="hex-input"
                          size="small"
                          :placeholder="f.length ? `${f.length * 2} 个 hex 字符` : 'hex'"
                          @update:value="(v: string) => setHex(row, f.key, f, v)"
                        />
                      </div>

                      <div v-else-if="f.kind === 'bool'" class="field field-bool">
                        <span class="field-label">{{ f.label }}</span>
                        <a-switch
                          :checked="boolOf(row, f.key)"
                          size="small"
                          @change="(v: unknown) => setBool(row, f.key, v)"
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

                      <div v-else-if="f.kind === 'array_float'" class="field field-array">
                        <span class="field-label">{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span>
                        <div class="array-editor">
                          <a-space v-for="(_, ai) in arrayValues(row, f.key)" :key="ai" size="small">
                            <a-input-number
                              :value="arrayValues(row, f.key)[ai]"
                              size="small"
                              class="w-84"
                              @change="(v: number | string | null | undefined) => setArrayValue(row, f.key, ai, v)"
                            />
                            <a-button size="small" type="text" @click="removeArrayItem(row, f.key, ai)">×</a-button>
                          </a-space>
                          <a-button size="small" type="dashed" @click="addArrayItem(row, f.key)">＋ {{ f.itemLabel }}</a-button>
                        </div>
                      </div>
                    </template>
                  </div>
                </div>

                <a-button v-if="g.multiple" size="small" type="dashed" block @click="addRow(g)">＋ 添加一行</a-button>
              </div>
            </a-collapse-panel>
          </a-collapse>
        </div>
      </a-tab-pane>

      <a-tab-pane key="reissue" tab="补发测试">
        <div class="zone-body">
          <div class="reissue-panel">
            <a-alert
              type="info"
              show-icon
              message="补发与实时共用同一份报文配置(实时数据页的分组内容)"
              description="区别仅在数据采集时间:补发把时间戳回拨到过去,模拟终端离线时段积攒的数据在重连后补报。"
              class="reissue-hint"
            />

            <a-form layout="vertical" class="reissue-form">
              <a-form-item label="起始时间偏移">
                <a-input-number
                  v-model:value="reissueOffset"
                  :min="0"
                  :max="86400"
                  addon-after="秒"
                  class="w-200"
                />
                <span class="field-hint">第一条的采集时间 = 当前时间 − 偏移</span>
              </a-form-item>
              <a-form-item label="补发条数">
                <a-input-number v-model:value="reissueCount" :min="1" :max="100" addon-after="条" class="w-200" />
                <span class="field-hint">模拟离线期间采集的报文条数</span>
              </a-form-item>
              <a-form-item label="补发间隔">
                <a-input-number
                  v-model:value="reissueInterval"
                  :min="1"
                  :max="3600"
                  addon-after="秒"
                  class="w-200"
                />
                <span class="field-hint">每条的时间戳按此间隔逐条前移</span>
              </a-form-item>
            </a-form>

            <a-button type="primary" :disabled="!online" @click="sendReissue">发送补发 (0x03)</a-button>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="custom" tab="自定义数据" :disabled="!store.config?.extensionPack">
        <ExtensionCommands />
      </a-tab-pane>
    </a-tabs>

      <a-modal v-model:open="previewOpen" title="报文预览" :footer="null" width="760px">
      <a-typography-text type="secondary">{{ previewCmd }} — 完整帧 HEX(点击即可复制)</a-typography-text>
      <a-typography-paragraph copyable code class="preview-hex">{{ previewHex }}</a-typography-paragraph>
    </a-modal>
  </div>
</template>

<style scoped>
.hex-input :deep(input) {
  font-family: var(--font-mono);
}
</style>
