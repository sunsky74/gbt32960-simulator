<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  // 左侧 Panel 的像素宽度(父级持有,本组件只负责拖拽计算)
  minPx: number
  // 左侧宽度上限(不传则只受容器宽度约束)
  maxPx?: number
  // 右侧 Panel 最小保留宽度(默认与 minPx 相同,保证右侧不被挤没)
  rightMinPx?: number
}>()

const emit = defineEmits<{
  (e: 'drag', leftPx: number): void
  (e: 'dragend'): void
}>()

const dividerEl = ref<HTMLElement | null>(null)
const dragging = ref(false)

function startDrag(e: PointerEvent) {
  const divider = dividerEl.value
  const container = divider?.parentElement
  if (!divider || !container || e.button !== 0) return

  dragging.value = true
  divider.setPointerCapture(e.pointerId)
  // 锁定全局光标与文字选中(style.css 既有 body.dragging-ew)
  document.body.classList.add('dragging-ew')

  const onMove = (ev: PointerEvent) => {
    const rect = container.getBoundingClientRect()
    const cs = getComputedStyle(container)
    const padLeft = parseFloat(cs.paddingLeft) || 0
    const padRight = parseFloat(cs.paddingRight) || 0
    const contentLeft = rect.left + padLeft
    const contentW = rect.width - padLeft - padRight - divider.offsetWidth
    const min = props.minPx
    const max = Math.min(
      props.maxPx ?? Number.POSITIVE_INFINITY,
      contentW - (props.rightMinPx ?? props.minPx),
    )
    if (max <= min) return
    const leftPx = Math.min(Math.max(ev.clientX - contentLeft, min), max)
    emit('drag', Math.round(leftPx))
  }

  const onUp = (ev: PointerEvent) => {
    dragging.value = false
    divider.releasePointerCapture(ev.pointerId)
    document.body.classList.remove('dragging-ew')
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    emit('dragend')
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}
</script>

<template>
  <div
    ref="dividerEl"
    class="resizable-divider-col"
    :class="{ dragging }"
    role="separator"
    aria-orientation="vertical"
    aria-label="拖动调整左右区域宽度"
    @pointerdown="startDrag"
  >
    <span class="divider-grip" />
  </div>
</template>
