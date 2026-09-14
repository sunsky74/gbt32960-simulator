<script setup lang="ts">
import { computed, ref } from 'vue'
import type { parser as parserNs } from '../../../wailsjs/go/models'
import ByteGridView, { type ByteHover, type ByteIssueRange, type ByteRange } from './ByteGridView.vue'
import FieldTableView from './FieldTableView.vue'
import { useSplitter } from '../../composables/useSplitter'

type ParsedField = parserNs.Field

// ---------- 左右两栏分析区:字节视图 ↔ 解析结果(含 vbar 左右拖拽与刷新淡入动画) ----------
// 联动高亮状态(悬停/钉住)由页面持有,此处仅透传 props/事件。
defineProps<{
  normalizedHex: string
  active: ByteRange | null
  activeSource: 'byte' | 'field' | null
  activeByte: number | null
  issues?: ByteIssueRange[]
  fields: ParsedField[]
}>()

const emit = defineEmits<{
  (e: 'byteHover', h: ByteHover | null): void
  (e: 'bytePin', r: ByteRange | null): void
  (e: 'fieldHover', r: ByteRange | null, pos?: { x: number; y: number }): void
  (e: 'fieldPin', r: ByteRange | null): void
}>()

const rootEl = ref<HTMLElement | null>(null)

// 左右拖拽: 字节视图 ↔ 解析结果
// 左栏最小 360px(地址列 + 每行 8 字节 + 内边距),右栏最小 480px(字段表可读宽度)
const LEFT_MIN = 360
const RIGHT_MIN = 480

const { size: leftW, onDown: onSideBarDown, reset: resetSide } = useSplitter({
  axis: 'x',
  bodyClass: 'dragging-ew',
  min: LEFT_MIN,
  trackSize: () => rootEl.value?.clientWidth ?? 1000,
  startSize: () => rootEl.value?.querySelector<HTMLElement>('.duo-left')?.offsetWidth ?? null,
  maxSize: (total) => Math.max(LEFT_MIN, total - RIGHT_MIN - 12),
})

// 固定左右 Grid:默认 45fr/55fr(按自由空间分配,不会因 6px 分割条/间距溢出容器);
// 左右均设最小宽度,窗口不足时工作区内部横向滚动,绝不被压缩成竖条
const duoStyle = computed(() => {
  const left = leftW.value !== null ? leftW.value + 'px' : 'minmax(360px, 45fr)'
  const right = leftW.value !== null ? 'minmax(480px, 1fr)' : 'minmax(480px, 55fr)'
  return { gridTemplateColumns: `${left} 6px ${right}` }
})

// 重新解析成功后:两栏内容平滑淡入刷新(由页面在重复解析成功时调用)
function flash() {
  const el = rootEl.value
  if (!el) return
  el.classList.remove('content-refresh')
  void el.offsetWidth // 强制重排,确保连续解析时动画可重新触发
  el.classList.add('content-refresh')
}

// 清空时恢复默认 45fr/55fr(由页面调用)
function resetWidth() {
  resetSide()
}

defineExpose({ flash, resetWidth })
</script>

<template>
  <div ref="rootEl" class="duo-zone" :style="duoStyle">
    <div class="zone duo-left">
      <div class="zone-body duo-body">
        <p class="section-label">报文字节视图(悬停字节查看字段信息)</p>
        <ByteGridView
          :normalized-hex="normalizedHex"
          :active="active"
          :active-source="activeSource"
          :active-byte="activeByte"
          :issues="issues ?? []"
          @hover="emit('byteHover', $event)"
          @pin="emit('bytePin', $event)"
        />
      </div>
    </div>

    <div
      class="vbar"
      title="拖拽调整左右宽度,双击恢复"
      @pointerdown="onSideBarDown"
      @dblclick="resetSide"
    ></div>

    <div class="zone duo-right">
      <div class="zone-body duo-body">
        <p class="section-label">解析结果({{ fields.length }} 个字段 · 悬停/点击联动字节)</p>
        <FieldTableView
          :fields="fields"
          :active="active"
          @hover="(r, pos) => emit('fieldHover', r, pos)"
          @pin="emit('fieldPin', $event)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ---------- 左右两栏:固定 Grid(默认 45fr/55fr,拖拽后左固定 px 右 1fr);
   左右最小宽度由 minmax 保证,窗口不足时工作区内部横向滚动而非压缩 ---------- */
.duo-zone {
  display: grid;
  gap: 8px;
  align-items: stretch;
  min-height: 0;
  flex: 1;
  overflow: auto;
}

/* 重新解析成功:两栏内容淡入刷新(仅工作台内重复解析触发) */
.duo-zone.content-refresh {
  animation: content-refresh 0.2s ease-out;
}

@keyframes content-refresh {
  from {
    opacity: 0.45;
  }
  to {
    opacity: 1;
  }
}

.vbar {
  width: 6px;
  border-radius: 3px;
  cursor: col-resize;
  background: transparent;
  transition: background-color 0.12s ease;
}

.vbar:hover {
  background: var(--primary-hover-bg);
}

/* 面板标题:与表格滚动高度测算配合,禁止浏览器默认 p 上下外边距 */
.section-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 0 10px;
}

.duo-body {
  display: flex;
  flex-direction: column;
  flex: 1; /* 填满 .zone 面板高度:左右内容区等高,表格/字节区滚动区不塌陷 */
  min-height: 0;
}

.duo-left :deep(.byte-grid) {
  flex: 1;
  min-height: 200px;
}

/* 解析结果面板:首次出现时从右侧轻微滑入 + 淡入(180~220ms,克制不夸张) */
.duo-right {
  animation: duo-right-in 0.22s ease-out;
}

@keyframes duo-right-in {
  from {
    opacity: 0;
    transform: translateX(8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

.duo-right :deep(.field-table) {
  flex: 1;
  min-height: 0;
}
</style>
