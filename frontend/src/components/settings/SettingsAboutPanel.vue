<script setup lang="ts">
// 关于:版本展示与更新检查入口(Phase 1 仅检查;下载/安装后续 Phase 接入)。
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { GithubOutlined } from '@ant-design/icons-vue'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import * as UpdaterService from '../../../wailsjs/go/bridge/UpdaterService'
import { updater } from '../../../wailsjs/go/models'
import { formatBytes, isDevVersion, skipVersion } from '../../composables/useUpdater'
import SettingRow from './SettingRow.vue'

const REPO_URL = 'https://github.com/sunsky74/gbt32960-simulator'

const current = ref('')
const devBuild = ref(false)
const checking = ref(false)
const info = ref<updater.UpdateInfo | null>(null)
const notesExpanded = ref(false)

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
  notesExpanded.value = false
  try {
    info.value = await UpdaterService.CheckUpdate()
  } catch (e) {
    message.error(String(e))
  } finally {
    checking.value = false
  }
}

function skip() {
  const latest = info.value?.latest
  if (!latest) return
  skipVersion(latest)
  message.success(`已跳过 ${latest},更高版本发布后将再次提示`)
}

function openRepo() {
  BrowserOpenURL(REPO_URL)
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
            <span class="about-new">
              发现新版本 <b>{{ info.latest }}</b>
              <template v-if="info.assetSize">({{ formatBytes(info.assetSize) }})</template>
            </span>
            <span v-if="platformUnsupported" class="about-hint">当前平台暂不支持自动更新</span>
            <div v-if="info.notes" class="about-notes" :class="{ expanded: notesExpanded }">
              <pre>{{ info.notes }}</pre>
            </div>
            <div class="about-actions">
              <a-button v-if="info.notes" size="small" type="link" @click="notesExpanded = !notesExpanded">
                {{ notesExpanded ? '收起说明' : '展开说明' }}
              </a-button>
              <a-button size="small" @click="skip">跳过此版本</a-button>
            </div>
          </template>
          <span v-else-if="upToDate" class="about-hint">已是最新版本</span>
        </div>
      </template>
      <template #action>
        <a-button size="small" :loading="checking" :disabled="devBuild" @click="check">检查更新</a-button>
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
  font-size: 12px;
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

.about-notes {
  max-height: 132px;
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 6px 10px;
  background: var(--bg-elevated);
}

.about-notes.expanded {
  max-height: none;
}

.about-notes pre {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
}

.about-actions {
  display: flex;
  gap: 8px;
}

/* 分组:组内末行去分隔线 */
.row-group :deep(.setting-row:last-child) {
  border-bottom: none;
}
</style>
