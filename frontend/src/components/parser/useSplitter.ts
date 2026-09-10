import { onBeforeUnmount, ref } from 'vue'

// ---------- 拖拽分割条共享逻辑(上下 hbar / 左右 vbar) ----------
// 按下时读取目标元素当前尺寸,拖动期间按轴增量钳位更新 size,松开/卸载时清理监听与 body 类。
// 数值公式(最小值、最大值计算)由调用方以闭包注入,保证与原实现逐字等价。
export interface SplitterOptions {
  /** 拖拽轴:'x' = 宽度(左右分割), 'y' = 高度(上下分割) */
  axis: 'x' | 'y'
  /** 拖拽期间加到 body 上的类(全局样式切换光标/禁选) */
  bodyClass: string
  /** 尺寸下限(px) */
  min: number
  /** 容器可用尺寸(计算最大值用);元素缺失时返回兜底值 */
  trackSize: () => number
  /** 按下时读取目标当前尺寸;不可拖拽时返回 null(不启动) */
  startSize: () => number | null
  /** 给定容器尺寸,计算尺寸上限(px) */
  maxSize: (track: number) => number
}

export function useSplitter(opts: SplitterOptions) {
  // null = 自然尺寸(未拖拽或已双击恢复)
  const size = ref<number | null>(null)

  let drag: { start: number; startSize: number } | null = null

  function onMove(e: PointerEvent) {
    if (!drag) return
    const next = drag.startSize + (opts.axis === 'x' ? e.clientX : e.clientY) - drag.start
    size.value = Math.round(Math.min(Math.max(next, opts.min), opts.maxSize(opts.trackSize())))
  }

  function onDown(e: PointerEvent) {
    const startSize = opts.startSize()
    if (startSize === null) return
    drag = { start: opts.axis === 'x' ? e.clientX : e.clientY, startSize }
    document.body.classList.add(opts.bodyClass)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  function onUp() {
    drag = null
    document.body.classList.remove(opts.bodyClass)
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }

  function reset() {
    size.value = null
  }

  // 组件卸载时兜底清理监听与 body 类
  onBeforeUnmount(onUp)

  return { size, onDown, onUp, reset }
}
