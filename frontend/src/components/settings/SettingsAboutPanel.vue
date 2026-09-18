<script setup lang="ts">
// 关于:版本展示、更新检查与下载/校验、安装并重启闭环。
import { computed, onMounted, ref } from 'vue'
import { Modal, message } from 'ant-design-vue'
import { GithubOutlined } from '@ant-design/icons-vue'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import * as UpdaterService from '../../../wailsjs/go/bridge/UpdaterService'
import * as ConnectionService from '../../../wailsjs/go/bridge/ConnectionService'
import * as ServerService from '../../../wailsjs/go/bridge/ServerService'
import { updater } from '../../../wailsjs/go/models'
import { formatBytes, isDevVersion, skipVersion } from '../../composables/useUpdater'
import { onWailsEvent, type UpdateProgressEvent } from '../../api/events'
import SettingRow from './SettingRow.vue'

const REPO_URL = 'https://github.com/sunsky74/gbt32960-simulator'

const current = ref('')
const devBuild = ref(false)
const checking = ref(false)
const info = ref<updater.UpdateInfo | null>(null)
const downloading = ref(false)
const progress = ref<UpdateProgressEvent | null>(null)
const ready = ref<updater.DownloadResult | null>(null)
const applying = ref(false)
let offProgress: (() => void) | null = null

// errText 统一解包:Wails 拒绝值为 Error(Go error → new Error(msg));直接 String(e) 会带 "Error: " 前缀
function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

onMounted(async () => {
  try {
    current.value = await UpdaterService.CurrentVersion()
  } catch {
    current.value = ''
  }
  devBuild.value = isDevVersion(current.value)
})

async function check() {
  checking.value = true
  ready.value = null
  try {
    info.value = await UpdaterService.CheckUpdate()
  } catch (e) {
    message.error(errText(e))
  } finally {
    checking.value = false
  }
}

async function download() {
  downloading.value = true
  progress.value = null
  offProgress = onWailsEvent('update:progress', (p) => {
    progress.value = p
  })
  try {
    ready.value = await UpdaterService.DownloadUpdate()
  } catch (e) {
    if (errText(e) !== '已取消下载') message.error(errText(e))
  } finally {
    downloading.value = false
    offProgress?.()
    offProgress = null
    progress.value = null
  }
}

async function cancelDownload() {
  try {
    await UpdaterService.CancelDownload()
  } catch {
    // 下载收尾由 DownloadUpdate 的拒绝统一处理
  }
}

const progressText = computed(() => {
  if (!progress.value) return '正在连接…'
  if (progress.value.phase === 'verifying') return '正在校验…'
  const { received, total } = progress.value
  return total > 0 ? `${formatBytes(received)} / ${formatBytes(total)}` : `已接收 ${formatBytes(received)}`
})

function skip() {
  const latest = info.value?.latest
  if (!latest) return
  skipVersion(latest)
  message.success(`已跳过 ${latest},更高版本发布后将再次提示`)
}

function openRepo() {
  BrowserOpenURL(REPO_URL)
}

// 运行态中断文案:仅声明两种影响,四种组合逐一精确(AC-13)
function interruptionHint(connOnline: boolean, serverRunning: boolean): string {
  const parts: string[] = []
  if (connOnline) parts.push('断开连接')
  if (serverRunning) parts.push('停止服务')
  return parts.length === 0 ? '安装过程中将退出。' : `安装过程中将${parts.join('、')}并退出。`
}

// 安装并重启:二次确认框按当前运行态动态追加中断提示
async function apply() {
  const [connState, serverStatus] = await Promise.all([ConnectionService.State(), ServerService.Status()])
  Modal.confirm({
    title: '安装并重启',
    content: `将安装 ${ready.value?.tag ?? ''}。${interruptionHint(connState === 'online', serverStatus.running)}`,
    okText: '安装并重启',
    cancelText: '取消',
    onOk: async () => {
      try {
        await UpdaterService.ApplyUpdate()
        applying.value = true
      } catch (e) {
        message.error(errText(e)) // 失败不退出应用,保持就绪态可重试
        throw e
      }
    },
  })
}

// 查看发布说明:打开新版本对应的 GitHub Release 页(检查通道不再返回说明正文)
function openReleaseNotes() {
  const latest = info.value?.latest
  if (!latest) return
  BrowserOpenURL(`${REPO_URL}/releases/tag/${latest}`)
}

const hasUpdate = computed(() => info.value?.hasUpdate === true)
const upToDate = computed(() => info.value !== null && !info.value.hasUpdate && !info.value.devBuild)
const platformUnsupported = computed(() => hasUpdate.value && !info.value?.assetName)
</script>

<template>
  <div class="row-group">
    <SettingRow title="当前版本" description="版本由发布流程注入;本地开发构建显示为 dev">
      <template #action>
        <span class="about-version">{{ devBuild ? '开发构建' : current || '未知' }}</span>
      </template>
    </SettingRow>

    <SettingRow title="检查更新">
      <template #description>
        <div class="about-result">
          <span v-if="devBuild" class="about-hint">开发构建不参与更新检查(发布版本以 v 开头的版本号显示)</span>
          <span v-else-if="checking" class="about-hint">正在检查…</span>
          <span v-else-if="!info" class="about-hint">从 GitHub Releases 查询最新版本</span>
          <template v-else-if="hasUpdate">
            <span class="about-new"
              >发现新版本 <b>{{ info.latest }}</b></span
            >
            <span v-if="platformUnsupported" class="about-hint">当前平台暂不支持自动更新</span>

            <template v-if="applying">
              <span class="about-new">正在安装并重启,应用将在数秒内退出…</span>
            </template>
            <template v-else-if="ready">
              <span class="about-new">更新包已就绪:{{ ready.tag }}(下载与校验完成)</span>
              <div class="about-actions">
                <a-button size="small" type="primary" @click="apply">安装并重启</a-button>
              </div>
            </template>
            <template v-else-if="downloading">
              <div class="about-progress">
                <a-progress :percent="progress?.percent ?? 0" :show-info="false" size="small" />
                <span class="about-hint">{{ progressText }}</span>
                <a-button size="small" @click="cancelDownload">取消下载</a-button>
              </div>
            </template>
            <template v-else>
              <div class="about-actions">
                <a-button size="small" type="primary" :disabled="platformUnsupported" @click="download"
                  >下载更新</a-button
                >
                <a-button size="small" type="link" @click="openReleaseNotes">查看发布说明</a-button>
                <a-button size="small" @click="skip">跳过此版本</a-button>
              </div>
            </template>
          </template>
          <span v-else-if="upToDate" class="about-hint">已是最新版本</span>
        </div>
      </template>
      <template #action>
        <a-button size="small" :loading="checking" :disabled="devBuild || applying" @click="check">检查更新</a-button>
      </template>
    </SettingRow>

    <SettingRow title="GitHub 仓库" description="查看发布记录与源码">
      <template #action>
        <a-button size="small" @click="openRepo">
          <template #icon><GithubOutlined /></template>
          打开仓库
        </a-button>
      </template>
    </SettingRow>
  </div>
</template>

<style scoped>
.about-version {
  font-family: var(--font-mono);
  font-size: var(--fs-12);
  color: var(--text-primary);
}

.about-result {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.about-hint {
  color: var(--text-tertiary);
}

.about-new {
  color: var(--text-primary);
}

.about-actions {
  display: flex;
  gap: 8px;
}

.about-progress {
  display: flex;
  align-items: center;
  gap: 8px;
}

.about-progress :deep(.ant-progress) {
  width: 160px;
  margin: 0;
}
</style>
