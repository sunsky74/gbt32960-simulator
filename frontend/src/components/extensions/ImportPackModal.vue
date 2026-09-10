<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import * as ExtService from '../../../wailsjs/go/bridge/ExtService'
import type { bridge } from '../../../wailsjs/go/models'

const emit = defineEmits<{
  (e: 'imported', info: bridge.PackInfo): void
}>()

// ---------- 导入弹窗:选文件 / 粘贴 JSON / 格式说明 ----------
const importOpen = ref(false)
const importTab = ref<'file' | 'json' | 'spec'>('file')
const jsonText = ref('')
const jsonError = ref('')
const importing = ref(false)

// 覆盖 meta/appendUnits/数值变换/bits/私有命令的最小可用示例(格式说明页一键填入)
const SPEC_EXAMPLE = `{
  "meta": { "id": "my-pack", "label": "我的扩展包", "vendor": "示例", "baseVersion": "2016", "scope": ["client", "parser"] },
  "realtime": {
    "appendUnits": [
      {
        "key": "telemetry", "title": "私有遥测单元", "unitCode": 128, "enabled": true,
        "fields": [
          { "key": "soc2", "label": "SOC2", "type": "u8", "unit": "%" },
          { "key": "packVolt", "label": "包电压", "type": "u16", "scale": 0.1, "unit": "V" },
          { "key": "temp", "label": "温度", "type": "i8", "offset": 40, "unit": "°C" },
          { "key": "flags", "label": "状态位", "type": "bits",
            "bits": [{ "index": 0, "label": "锁车" }, { "index": 1, "label": "限速" }] }
        ]
      }
    ]
  },
  "commands": [
    {
      "key": "extData", "label": "扩展数据上报", "code": 9, "direction": "up", "trigger": "manual+periodic",
      "body": { "type": "fields", "fields": [{ "key": "seq", "label": "流水号", "type": "u16" }] }
    }
  ]
}`

function open() {
  importTab.value = 'file'
  jsonText.value = ''
  jsonError.value = ''
  importOpen.value = true
}

// 页面打开详情时会顺带清掉导入错误提示(与原 openDetail 行为一致)
function clearError() {
  jsonError.value = ''
}

async function importFromFile() {
  importing.value = true
  jsonError.value = ''
  try {
    const path = await ExtService.PickPackFile()
    if (!path) return
    const info = await ExtService.ImportPack(path)
    message.success(`已导入「${info.label}」`)
    importOpen.value = false
    // 缓存失效 / schema 刷新 / 列表刷新由页面统一处理(顺序与原实现一致)
    emit('imported', info)
  } catch (e) {
    jsonError.value = String(e)
  } finally {
    importing.value = false
  }
}

async function importFromJSON() {
  if (!jsonText.value.trim()) {
    jsonError.value = '请输入扩展包 JSON 内容'
    return
  }
  importing.value = true
  jsonError.value = ''
  try {
    const info = await ExtService.ImportPackJSON(jsonText.value)
    message.success(`已导入「${info.label}」`)
    importOpen.value = false
    jsonText.value = ''
    // 缓存失效 / schema 刷新 / 列表刷新由页面统一处理(顺序与原实现一致)
    emit('imported', info)
  } catch (e) {
    jsonError.value = String(e)
  } finally {
    importing.value = false
  }
}

function fillExample() {
  jsonText.value = SPEC_EXAMPLE
  jsonError.value = ''
  importTab.value = 'json'
}

async function copyExample() {
  await navigator.clipboard.writeText(SPEC_EXAMPLE)
  message.success('示例已复制,可在「粘贴 JSON」页修改后导入')
}

defineExpose({ open, clearError })
</script>

<template>
  <!-- 导入弹窗 -->
  <a-modal
    v-model:open="importOpen"
    title="导入扩展包"
    :footer="null"
    :mask-closable="false"
    width="640px"
  >
    <a-tabs v-model:active-key="importTab" @change="jsonError = ''">
      <a-tab-pane key="file" tab="选择文件">
        <p class="tab-hint">从本地选择一个扩展包 JSON 文件导入。校验(语法/码位/干跑)通过后才会落盘;同 id 覆盖导入会清空该包已保存的表单值。</p>
      </a-tab-pane>
      <a-tab-pane key="json" tab="粘贴 JSON">
        <a-textarea
          v-model:value="jsonText"
          :rows="12"
          class="json-input"
          spellcheck="false"
          placeholder='粘贴扩展包 JSON,例如: {"meta": {"id": "my-pack", "label": "我的包", "baseVersion": "2016"}, ...}'
        />
        <p class="tab-hint">格式与文件导入一致,写法见「格式说明」页签;完整文档 docs/extpack-guide.md。</p>
      </a-tab-pane>
      <a-tab-pane key="spec" tab="格式说明">
        <div class="spec-body">
          <p class="spec-lead">
            扩展包 = 一个 JSON 对象,顶层三段:<code>meta</code>(元信息,必填) +
            <code>realtime.appendUnits</code>(实时追加单元,可选) + <code>commands</code>(私有命令,可选)。
          </p>

          <h4>1. meta 元信息</h4>
          <table class="spec-table">
            <tbody>
              <tr><td class="k">id</td><td>包唯一标识。2~64 位小写字母/数字/连字符;同 id 导入覆盖旧包</td></tr>
              <tr><td class="k">label</td><td>显示名,非空</td></tr>
              <tr><td class="k">vendor</td><td>厂商名,可选,仅展示</td></tr>
              <tr><td class="k">baseVersion</td><td>协议基准版本,只能 <code>"2016"</code> 或 <code>"2025"</code></td></tr>
              <tr><td class="k">scope</td><td>应用范围数组:<code>"client"</code>(客户端模拟) / <code>"parser"</code>(报文解析);缺省视为 <code>["client"]</code></td></tr>
            </tbody>
          </table>

          <h4>2. realtime.appendUnits 实时追加单元</h4>
          <p class="spec-p">拼在标准 0x02 实时报文尾部的私有 TLV(单元码 1B + 长度 2B + 数据),随实时/补发/周期上报一起发送。</p>
          <table class="spec-table">
            <tbody>
              <tr><td class="k">key / title</td><td>组键(同包唯一,禁冒号) / 表单显示标题</td></tr>
              <tr><td class="k">unitCode</td><td>单元码,只允许 <code>0x80~0xFE</code>(128~254),同包唯一;标准码位禁止占用</td></tr>
              <tr><td class="k">enabled</td><td>默认是否启用(false 时表单不勾选、不参与编码)</td></tr>
              <tr><td class="k">multiple / maxRows</td><td>true 时表单多行,<strong>每行编码为一个独立 TLV</strong>;maxRows 上限</td></tr>
              <tr><td class="k">fields</td><td>字段表,至少一项,见下节 DSL</td></tr>
            </tbody>
          </table>

          <h4>3. fields 字段 DSL</h4>
          <table class="spec-table">
            <tbody>
              <tr><td class="k">u8 / u16 / u32</td><td>无符号整数,1 / 2 / 4 字节</td></tr>
              <tr><td class="k">i8 / i16 / i32</td><td>有符号整数,1 / 2 / 4 字节</td></tr>
              <tr><td class="k">f32</td><td>IEEE 754 浮点,4 字节(不支持 scale/offset)</td></tr>
              <tr><td class="k">bits</td><td>位段开关:<code>bits: [{ "index": 0~31, "label": "…" }]</code>,宽度按最大位号自动取 1/2/4 字节</td></tr>
              <tr><td class="k">bytes</td><td>定长原始字节,<code>length</code> 1~255,表单填 hex(如 1A2B)</td></tr>
              <tr><td class="k">tail</td><td>变长尾部(hex 原样追加),建议放字段表末尾</td></tr>
            </tbody>
          </table>
          <p class="spec-p">每个字段:<code>key</code>(组内唯一)、<code>label</code>、<code>type</code>、<code>unit</code>(仅展示)。
            数值变换契约:<strong>物理值 = 线值 × scale + offset</strong>(scale 默认 1、offset 默认 0;scale 须大于 0 且换算结果须为整数线值)。
            例:<code>{ "type": "u16", "scale": 0.1 }</code> 填 12.3 V → 线值 123;温度惯例 <code>{ "type": "i8", "offset": 40 }</code>。</p>

          <h4>4. commands 私有命令(可选)</h4>
          <table class="spec-table">
            <tbody>
              <tr><td class="k">key / label</td><td>命令键(同包唯一,禁冒号、禁与标准组键同名) / 显示名</td></tr>
              <tr><td class="k">code</td><td>命令码,上行预留区:2016 为 <code>0x09~0x7F</code>,2025 为 <code>0x0C~0x7F</code></td></tr>
              <tr><td class="k">direction / trigger</td><td><code>"up"</code> / <code>manual</code> · <code>periodic</code> · <code>manual+periodic</code></td></tr>
              <tr><td class="k">body.type</td><td><code>fields</code> 平铺字段表(单行);<code>realtimeLike</code> 与实时报文同构(6B 时间 + TLV 单元,单元结构同第 2 节);<code>remoteAck</code> 0x8A 远控应答模板(仅 2016,详见完整文档)</td></tr>
            </tbody>
          </table>

          <h4>5. 常见校验规则</h4>
          <ul class="spec-list">
            <li>unitCode 与命令码同包唯一;unitCode 只能用 0x80~0xFE 自定义区</li>
            <li>key 禁含冒号 <code>:</code>;禁用保留键(vehicle / motor / fuelcell / engine / location / extremum / alarm / voltage / temperature 等)</li>
            <li>导入时做静态校验 + 编码干跑,错误带 JSON 路径(如 <code>realtime.appendUnits[0].fields[1].scale</code>),照提示修改即可</li>
          </ul>

          <h4>6. 最小完整示例</h4>
          <pre class="spec-example">{{ SPEC_EXAMPLE }}</pre>
          <div class="spec-actions">
            <a-button size="small" type="primary" @click="fillExample">填入示例并去粘贴页</a-button>
            <a-button size="small" @click="copyExample">复制示例</a-button>
          </div>
          <p class="tab-hint">完整规范、字段类型取值表与错误解读见仓库 docs/extpack-guide.md。</p>
        </div>
      </a-tab-pane>
    </a-tabs>

    <a-alert
      v-if="jsonError"
      type="error"
      show-icon
      message="导入失败"
      :description="jsonError"
      class="import-alert"
    />

    <div class="modal-footer">
      <a-button @click="importOpen = false">取消</a-button>
      <a-button v-if="importTab === 'file'" type="primary" :loading="importing" @click="importFromFile">选择文件…</a-button>
      <a-button v-else-if="importTab === 'json'" type="primary" :loading="importing" @click="importFromJSON">导入</a-button>
      <a-button v-else type="primary" @click="fillExample">填入示例</a-button>
    </div>
  </a-modal>
</template>

<style scoped>
/* ---------- 导入弹窗(沿用 PackManager 样式) ---------- */
.tab-hint {
  margin: 0 0 8px;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.json-input :deep(textarea) {
  font-family: var(--font-mono);
  font-size: 12px;
}

.import-alert {
  margin-top: 8px;
}

.import-alert :deep(.ant-alert-description) {
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 140px;
  overflow: auto;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.spec-body {
  max-height: 430px;
  overflow-y: auto;
  padding-right: 4px;
}

.spec-lead {
  margin: 4px 0 8px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-primary);
}

.spec-body h4 {
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.spec-body code {
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 0 4px;
}

.spec-p {
  margin: 0 0 6px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-secondary);
}

.spec-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.spec-table td {
  border: 1px solid var(--border-subtle);
  padding: 4px 8px;
  line-height: 1.6;
  color: var(--text-secondary);
  vertical-align: top;
}

.spec-table td.k {
  width: 130px;
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: nowrap;
}

.spec-list {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-secondary);
}

.spec-example {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 10px 12px;
  max-height: 250px;
  overflow: auto;
  white-space: pre;
}

.spec-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
</style>
