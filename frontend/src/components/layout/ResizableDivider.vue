<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  // 上部 Panel 的像素高度(父级持有,本组件只负责拖拽计算)
  minPx: number
}>()

const emit = defineEmits<{
  (e: 'drag', topPx: number): void
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
  // 锁定全局光标与文字选中(style.css 既有 body.dragging-ns)
  document.body.classList.add('dragging-ns')

  const onMove = (ev: PointerEvent) => {
    const rect = container.getBoundingClientRect()
    const cs = getComputedStyle(container)
    const padTop = parseFloat(cs.paddingTop) || 0
    const padBottom = parseFloat(cs.paddingBottom) || 0
    const contentTop = rect.top + padTop
    const contentH = rect.height - padTop - padBottom - divider.offsetHeight
    const min = props.minPx
    const max = contentH - props.minPx
    if (max <= min) return
    const topPx = Math.min(Math.max(ev.clientY - contentTop, min), max)
    emit('drag', Math.round(topPx))
  }

  const onUp = (ev: PointerEvent) => {
    dragging.value = false
    divider.releasePointerCapture(ev.pointerId)
    document.body.classList.remove('dragging-ns')
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
    class="resizable-divider"
    :class="{ dragging }"
    role="separator"
    aria-orientation="horizontal"
    aria-label="拖动调整上下区域高度"
    @pointerdown="startDrag"
  >
    <span class="divider-grip" />
  </div>
</template>
