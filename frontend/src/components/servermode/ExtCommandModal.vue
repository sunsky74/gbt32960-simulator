<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import * as ServerService from '../../../wailsjs/go/bridge/ServerService'
import {
  bitsArrayOf,
  bitOptions,
  boolOf,
  hexOf,
  numOf,
  setBitsArray,
  setBool,
  setEnum,
  setHex,
  setNum,
} from '../../composables/useFieldHelpers'
import type { FieldSchema } from '../../api/backend'

const props = defineProps<{
  selectedVin: string
}>()

// ---------- 平台下发:扩展包 down 命令模板 ----------
interface ExtCommandInfo {
  packId: string
  packLabel: string
  key: string
  label: string
  code: number
  respType: string
  fields: FieldSchema[]
  defaults: Record<string, unknown>
}

const extCmds = ref<ExtCommandInfo[]>([])
const extCmdOpen = ref(false)
const extCmdKey = ref('')
const extCmdRow = ref<Record<string, unknown>>({})

const extCmdOptions = computed(() =>
  extCmds.value.map((c) => ({ value: `${c.packId}/${c.key}`, label: `${c.packLabel} · ${c.label}` })),
)

const activeExtCmd = computed(() => {
  const [packId, key] = extCmdKey.value.split('/')
  return extCmds.value.find((c) => c.packId === packId && c.key === key) ?? null
})

async function refreshExtCmds() {
  extCmds.value = (await ServerService.ServerExtCommands().catch(() => [])) ?? []
}

function onExtCmdSelect(val: string) {
  extCmdKey.value = val
  const cmd = extCmds.value.find((c) => `${c.packId}/${c.key}` === val)
  extCmdRow.value = cmd ? { ...cmd.defaults } : {}
}

async function sendExtCmd() {
  const cmd = activeExtCmd.value
  if (!cmd || !props.selectedVin) return
  try {
    await ServerService.SendExtCommand(cmd.packId, cmd.key, props.selectedVin, extCmdRow.value)
    message.success(`已下发「${cmd.label}」→ ${props.selectedVin}`)
    extCmdOpen.value = false
  } catch (e) {
    message.error('下发失败: ' + String(e))
  }
}

// 打开弹窗:每次打开前刷新命令列表(与原 openExtCmdModal 行为一致)
function open() {
  void refreshExtCmds()
  extCmdOpen.value = true
}

defineExpose({ open })
</script>

<template>
  <!-- 平台下发:扩展包 down 命令模板 -->
  <a-modal
    v-model:open="extCmdOpen"
    title="平台下发扩展命令"
    :width="520"
    ok-text="下发"
    cancel-text="取消"
    :ok-button-props="{ disabled: !activeExtCmd }"
    @ok="sendExtCmd"
  >
    <p class="modal-hint">目标车辆 {{ selectedVin || '(未选中)' }} · 命令来自已导入扩展包(scope 含 server)的下发模板</p>
    <a-select
      :value="extCmdKey || undefined"
      :options="extCmdOptions"
      placeholder="选择下发命令"
      style="width: 100%"
      @change="onExtCmdSelect"
    />
    <div v-if="activeExtCmd" class="extcmd-fields">
      <template v-for="f in activeExtCmd.fields" :key="f.key">
        <div v-if="f.kind === 'enum'" class="field">
          <span class="field-label">{{ f.label }}</span>
          <a-select
            :value="numOf(extCmdRow, f.key)"
            size="small"
            style="flex: 1"
            :options="(f.enum ?? []).map((e) => ({ value: e.value, label: e.label }))"
            @change="(v: unknown) => setEnum(extCmdRow, f.key, v)"
          />
        </div>
        <div v-else-if="f.kind === 'int' || f.kind === 'float'" class="field">
          <span class="field-label"
            >{{ f.label }}<em v-if="f.unit"> ({{ f.unit }})</em></span
          >
          <a-input-number
            :value="numOf(extCmdRow, f.key)"
            size="small"
            style="flex: 1"
            :min="f.min"
            :max="f.max"
            @change="(v: number | string | null | undefined) => setNum(extCmdRow, f.key, v)"
          />
        </div>
        <div v-else-if="f.kind === 'bytes'" class="field">
          <span class="field-label"
            >{{ f.label }}<em v-if="f.length"> ({{ f.length }}B hex)</em></span
          >
          <a-input
            :value="hexOf(extCmdRow, f.key)"
            class="hex-input"
            size="small"
            style="flex: 1"
            @update:value="(v: string) => setHex(extCmdRow, f.key, f, v)"
          />
        </div>
        <div v-else-if="f.kind === 'bool'" class="field field-bool">
          <span class="field-label">{{ f.label }}</span>
          <a-switch
            :checked="boolOf(extCmdRow, f.key)"
            size="small"
            @change="(v: unknown) => setBool(extCmdRow, f.key, v)"
          />
        </div>
        <div v-else-if="f.kind === 'bitgroup'" class="field field-bits">
          <span class="field-label">{{ f.label }}</span>
          <a-checkbox-group
            :value="bitsArrayOf(extCmdRow, f)"
            :options="bitOptions(f)"
            class="bits-group"
            @change="(vals: Array<string | number | boolean>) => setBitsArray(extCmdRow, f, vals)"
          />
        </div>
      </template>
    </div>
  </a-modal>
</template>

<style scoped>
.extcmd-fields {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}
</style>
