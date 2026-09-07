<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  ApiOutlined, CodeOutlined, DatabaseOutlined, ExperimentOutlined, GlobalOutlined,
  SettingOutlined, SearchOutlined, ToolOutlined,
} from '@ant-design/icons-vue'
import { markRaw, type Component } from 'vue'
import { appSettings, resetAppSettings } from '../composables/useAppSettings'
import * as SystemService from '../../wailsjs/go/bridge/SystemService'
import SettingRow from '../components/settings/SettingRow.vue'

// ---------- 分类注册表:左侧导航 + 右侧内容一一对应 ----------
interface SettingsCategory {
  key: string
  title: string
  icon: Component
}

const categories: SettingsCategory[] = [
  { key: 'common', title: '常用', icon: markRaw(SettingOutlined) },
  { key: 'appearance', title: '外观', icon: markRaw(ApiOutlined) },
  { key: 'editor', title: '编辑器', icon: markRaw(CodeOutlined) },
  { key: 'parser', title: '报文解析', icon: markRaw(SearchOutlined) },
  { key: 'storage', title: '数据与存储', icon: markRaw(DatabaseOutlined) },
  { key: 'network', title: '网络', icon: markRaw(GlobalOutlined) },
  { key: 'keys', title: '快捷键', icon: markRaw(ToolOutlined) },
  { key: 'advanced', title: '高级', icon: markRaw(ExperimentOutlined) },
]

const activeCategory = ref('appearance')
const currentCategory = computed(() => categories.find((c) => c.key === activeCategory.value))

const PLANNED = '规划中'
const CTRL_W = '180px' // 下拉/输入类控件统一宽度,禁止被 flex 拉伸

// ---------- 数据与存储:后端只读路径 ----------
const storagePaths = ref<Record<string, string> | null>(null)

onMounted(async () => {
  try {
    storagePaths.value = await SystemService.StoragePaths()
  } catch {
    storagePaths.value = null
  }
})

async function openDir(path?: string) {
  if (!path) return
  try {
    await SystemService.OpenDirectory(path)
  } catch (e) {
    message.error(String(e))
  }
}

// ---------- 恢复默认(危险操作,二次确认) ----------
function confirmReset() {
  Modal.confirm({
    title: '恢复默认设置?',
    content: '将重置外观、常用等应用设置(不影响连接档案、扩展包与报文配置)。此操作不可撤销。',
    okText: '恢复默认',
    okType: 'danger',
    cancelText: '取消',
    onOk() {
      resetAppSettings()
    },
  })
}

const shortcuts = [
  { action: '解析报文', key: 'Ctrl / ⌘ + Enter' },
  { action: '清空报文', key: 'Ctrl / ⌘ + L' },
  { action: '复制 HEX', key: 'Ctrl / ⌘ + Shift + C' },
  { action: '保存配置', key: 'Ctrl / ⌘ + S' },
  { action: '切换页面', key: 'Ctrl / ⌘ + 1~5' },
]
</script>

<template>
  <!-- 左右分栏:左侧分类导航 / 右侧设置内容(各自独立滚动);
       不复用 .page-root(其 flex-direction: column 会把分栏压成上下堆叠) -->
  <div class="settings-page">
    <aside class="settings-nav">
      <div class="sn-head">
        <span class="sn-title">设置</span>
      </div>
      <div class="sn-scroll">
        <div
          v-for="c in categories"
          :key="c.key"
          class="sn-item"
          :class="{ active: activeCategory === c.key }"
          @click="activeCategory = c.key"
        >
          <component :is="c.icon" class="sn-icon" />
          <span>{{ c.title }}</span>
        </div>
      </div>
    </aside>

    <section class="settings-body">
      <div class="sb-header">
        <h1 class="sb-title">{{ currentCategory?.title }}</h1>
      </div>
      <div class="sb-scroll">
        <!-- 限宽内容容器:大屏不横向无限拉伸(VS Code 设置页风格) -->
        <div class="sb-content">

          <!-- ================ 常用 ================ -->
          <template v-if="activeCategory === 'common'">
            <div class="row-group">
              <SettingRow
                title="启动时恢复上次工作区"
                description="启动后自动打开上次使用的页面(客户端模拟 / 报文解析等)"
              >
                <template #action>
                  <a-switch v-model:checked="appSettings.restoreLastPage" size="small" />
                </template>
              </SettingRow>
              <SettingRow
                title="启动时自动连接上次档案" badge="规划中" disabled
                description="应用启动后自动使用上次连接档案发起连接"
              >
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="自动保存配置" badge="规划中" disabled description="表单修改后自动持久化,无需手动保存">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="启动时检查更新" badge="规划中" disabled description="启动后后台检查新版本并提示">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="显示欢迎页" badge="规划中" disabled description="启动时显示版本说明与快速入口">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
            </div>
          </template>

          <!-- ================ 外观 ================ -->
          <template v-else-if="activeCategory === 'appearance'">
            <div class="group-title">主题</div>
            <div class="row-group">
              <SettingRow title="应用程序主题" description="选择界面配色;「跟随系统」随操作系统外观实时切换">
                <template #action>
                  <a-radio-group v-model:value="appSettings.themeMode" button-style="solid" size="small">
                    <a-radio-button value="dark">深色</a-radio-button>
                    <a-radio-button value="light">浅色</a-radio-button>
                    <a-radio-button value="auto">跟随系统</a-radio-button>
                  </a-radio-group>
                </template>
              </SettingRow>
            </div>
            <div class="group-title">界面</div>
            <div class="row-group">
              <SettingRow title="启用界面动画" description="关闭后抑制全局面板过渡与动画,降低视觉噪声">
                <template #action>
                  <a-switch v-model:checked="appSettings.animations" size="small" />
                </template>
              </SettingRow>
              <SettingRow title="界面密度" badge="规划中" disabled description="紧凑 / 默认 / 宽松三档全局间距">
                <template #action>
                  <a-select size="small" disabled value="默认" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="字体大小" badge="规划中" disabled description="小 / 默认 / 大三档界面字号">
                <template #action>
                  <a-select size="small" disabled value="默认" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="显示侧边栏文字" badge="规划中" disabled description="关闭后侧边导航仅显示图标">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="自动折叠侧边栏" badge="规划中" disabled description="窗口较窄时自动收起侧边导航">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
            </div>
          </template>

          <!-- ================ 编辑器 ================ -->
          <template v-else-if="activeCategory === 'editor'">
            <p class="cat-hint">以下为编辑器 / 数据视图的预留配置,将随 IDE 化演进一步步接入,当前尚未生效。</p>
            <div class="row-group">
              <SettingRow title="编辑器字体" badge="规划中" disabled description="HEX / JSON 数据视图使用的等宽字体">
                <template #action>
                  <a-select size="small" disabled value="系统默认" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="编辑器字号" badge="规划中" disabled description="数据视图字号(11 ~ 16px)">
                <template #action>
                  <a-select size="small" disabled value="12" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="Tab 宽度" badge="规划中" disabled description="JSON 编辑缩进宽度">
                <template #action>
                  <a-select size="small" disabled value="2" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="自动换行" badge="规划中" disabled description="长行折行显示而非横向滚动">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="显示行号" badge="规划中" disabled description="数据编辑视图显示行号">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="JSON 自动格式化" badge="规划中" disabled description="粘贴 / 保存 JSON 时自动整理格式">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="HEX 显示方式" badge="规划中" disabled description="字节视图分组大小与大小写偏好">
                <template #action>
                  <a-select size="small" disabled value="大写 · 8 字节" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
            </div>
          </template>

          <!-- ================ 报文解析 ================ -->
          <template v-else-if="activeCategory === 'parser'">
            <p class="cat-hint">报文解析页当前自动识别报文版本并完整容忍空格 / 换行 / 0x 前缀;以下默认行为开关将在后续版本接入解析页。</p>
            <div class="row-group">
              <SettingRow title="默认协议版本" badge="规划中" disabled description="版本自动识别失败时按此版本解析">
                <template #action>
                  <a-select size="small" disabled value="GB/T 32960-2016" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="输入自动格式化" badge="规划中" disabled description="粘贴报文后自动按字节分组排版">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="解析后自动定位第一个字段" badge="规划中" disabled description="解析成功后自动选中并联动字节视图">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="默认显示字节视图" badge="规划中" disabled description="解析工作台的字节视图默认可见性">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="默认展开解析结果分组" badge="规划中" disabled description="字段表分组(报文头 / 数据单元 / BCC)的初始展开状态">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
            </div>
          </template>

          <!-- ================ 数据与存储 ================ -->
          <template v-else-if="activeCategory === 'storage'">
            <div class="group-title">存储位置</div>
            <div class="row-group">
              <SettingRow title="配置目录" mono-desc :description="storagePaths?.config ?? '读取中…'">
                <template #action>
                  <a-button size="small" :disabled="!storagePaths" @click="openDir(storagePaths?.config)">打开目录</a-button>
                </template>
              </SettingRow>
              <SettingRow title="扩展包目录" mono-desc :description="storagePaths?.packs ?? '读取中…'">
                <template #action>
                  <a-button size="small" :disabled="!storagePaths" @click="openDir(storagePaths?.packs)">打开目录</a-button>
                </template>
              </SettingRow>
              <SettingRow title="自定义存储位置" badge="规划中" disabled description="将配置 / 扩展包数据迁移到自定义目录">
                <template #action><a-button size="small" disabled>修改位置</a-button></template>
              </SettingRow>
            </div>
            <div class="group-title">历史与缓存</div>
            <div class="row-group">
              <SettingRow title="保存解析历史" badge="规划中" disabled description="记录最近解析的报文,便于快速重放">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="最大历史记录数量" badge="规划中" disabled description="超出后自动淘汰最旧记录">
                <template #action>
                  <a-select size="small" disabled value="100" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="清理缓存与历史数据" badge="规划中" disabled description="清除界面缓存与解析历史(不影响连接档案与扩展包)">
                <template #action><a-button size="small" disabled>清理</a-button></template>
              </SettingRow>
            </div>
          </template>

          <!-- ================ 网络 ================ -->
          <template v-else-if="activeCategory === 'network'">
            <p class="cat-hint">此处配置软件级默认行为;具体连接的地址 / 端口 / VIN / 心跳等按档案配置,见「客户端模拟 → 连接配置」。</p>
            <div class="row-group">
              <SettingRow title="连接测试超时" badge="规划中" disabled description="「测试连接」的默认超时时间">
                <template #action>
                  <a-select size="small" disabled value="3 秒" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="默认连接超时" badge="规划中" disabled description="新建档案的初始连接超时">
                <template #action>
                  <a-select size="small" disabled value="10 秒" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="自动重连" badge="规划中" disabled description="断线后默认是否自动重连(档案级开关的默认值)">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="重连次数 / 间隔" badge="规划中" disabled description="自动重连的尝试次数与间隔">
                <template #action>
                  <a-select size="small" disabled value="5 次 · 10 秒" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
              <SettingRow title="TLS 默认行为" badge="规划中" disabled description="新建档案的 TLS 初始配置(跳过证书验证等)">
                <template #action>
                  <a-select size="small" disabled value="标准校验" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
            </div>
          </template>

          <!-- ================ 快捷键 ================ -->
          <template v-else-if="activeCategory === 'keys'">
            <p class="cat-hint">以下为规划的快捷键方案;快捷键功能将在后续版本开放,当前尚未生效。</p>
            <table class="keys-table">
              <thead>
                <tr><th>操作</th><th>快捷键</th><th>状态</th></tr>
              </thead>
              <tbody>
                <tr v-for="s in shortcuts" :key="s.action">
                  <td>{{ s.action }}</td>
                  <td><span class="key-cap mono">{{ s.key }}</span></td>
                  <td><span class="row-badge">{{ PLANNED }}</span></td>
                </tr>
              </tbody>
            </table>
          </template>

          <!-- ================ 高级 ================ -->
          <template v-else-if="activeCategory === 'advanced'">
            <div class="group-title">扩展包</div>
            <div class="row-group">
              <SettingRow title="扩展包目录" mono-desc :description="storagePaths?.packs ?? '读取中…'">
                <template #action>
                  <a-button size="small" :disabled="!storagePaths" @click="openDir(storagePaths?.packs)">打开目录</a-button>
                </template>
              </SettingRow>
            </div>
            <div class="group-title">调试</div>
            <div class="row-group">
              <SettingRow title="Debug 模式" badge="规划中" disabled description="控制台输出协议编解码细节">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="显示协议原始日志" badge="规划中" disabled description="控制台显示未解析的原始帧 hex">
                <template #action><a-switch size="small" disabled /></template>
              </SettingRow>
              <SettingRow title="日志级别" badge="规划中" disabled description="运行日志输出级别">
                <template #action>
                  <a-select size="small" disabled value="INFO" :style="{ width: CTRL_W }" />
                </template>
              </SettingRow>
            </div>
            <div class="group-title danger-zone">危险操作</div>
            <div class="row-group">
              <SettingRow title="恢复默认设置" description="重置外观 / 常用等应用设置;不影响连接档案、扩展包与报文配置">
                <template #action>
                  <a-button size="small" danger @click="confirmReset">恢复默认</a-button>
                </template>
              </SettingRow>
            </div>
          </template>

        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* 左右分栏(显式 row,不复用 .page-root 的 column 方向) */
.settings-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: row;
  gap: 12px;
  padding: 16px;
  overflow: hidden;
}

/* ---------- 左侧分类导航(IDE Settings 目录):固定宽 + 独立滚动 ---------- */
.settings-nav {
  width: 220px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.sn-head {
  flex-shrink: 0;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-subtle);
}

.sn-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.sn-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
}

.sn-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background-color 0.1s ease, color 0.1s ease;
}

.sn-item:hover {
  background: var(--item-hover-bg);
  color: var(--text-primary);
}

.sn-item.active {
  background: var(--primary-hover-bg);
  color: var(--primary);
  font-weight: 500;
}

.sn-icon {
  font-size: 14px;
}

/* ---------- 右侧设置内容:占满剩余空间 + 独立滚动 ---------- */
.settings-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.sb-header {
  flex-shrink: 0;
  padding: 12px 24px;
  border-bottom: 1px solid var(--border-subtle);
}

.sb-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.sb-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* 限宽内容容器:大屏禁止横向无限拉伸 */
.sb-content {
  max-width: 880px;
  margin: 0 auto;
  padding: 6px 24px 28px;
}

/* 分组:标题 + 行组;组内末行去分隔线 */
.group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin: 20px 0 2px;
}

.group-title:first-child {
  margin-top: 6px;
}

.group-title.danger-zone {
  color: var(--error-color, #ff4d4f);
}

.row-group > :deep(.setting-row:last-child) {
  border-bottom: none;
}

.cat-hint {
  margin: 10px 0 6px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-tertiary);
}

/* 快捷键表 */
.keys-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  margin-top: 6px;
}

.keys-table th {
  text-align: left;
  font-weight: 500;
  color: var(--text-tertiary);
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-subtle);
}

.keys-table td {
  padding: 8px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

.key-cap {
  font-size: 11px;
  color: var(--text-primary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 1px 6px;
}

.row-badge {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: 3px;
  color: var(--text-tertiary);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
}

.mono {
  font-family: var(--font-mono);
}
</style>
