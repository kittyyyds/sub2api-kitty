import Lenis from 'lenis'
import 'lenis/dist/lenis.css'
import { onBeforeUnmount, onMounted } from 'vue'
import type { ShallowRef } from 'vue'

export function useHeyRouteSmoothScroll(rootRef: ShallowRef<HTMLElement | null>): void {
  let lenis: Lenis | null = null
  let frameId = 0
  let idleFrames = 0
  let removeVirtualScrollListener = (): void => {}
  let removeScrollListener = (): void => {}

  function stopFrame(): void {
    window.cancelAnimationFrame(frameId)
    frameId = 0
  }

  function requestFrame(): void {
    if (frameId || !lenis || document.hidden) return
    frameId = window.requestAnimationFrame(animate)
  }

  function wake(): void {
    idleFrames = 0
    requestFrame()
  }

  function animate(time: number): void {
    frameId = 0
    const current = lenis
    if (!current || document.hidden) return

    current.raf(time)
    if (current.isScrolling) {
      idleFrames = 0
      requestFrame()
      return
    }
    if (idleFrames < 2) {
      idleFrames += 1
      requestFrame()
    }
  }

  function handleVisibilityChange(): void {
    if (document.hidden) stopFrame()
    else wake()
  }

  onMounted(() => {
    const root = rootRef.value
    if (!root) return

    const desktop = window.matchMedia('(min-width: 901px)').matches
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (!desktop || reduceMotion) return

    lenis = new Lenis({
      wrapper: root,
      content: root,
      duration: 1.6,
      smoothWheel: true,
      syncTouch: false,
    })

    removeVirtualScrollListener = lenis.on('virtual-scroll', wake)
    removeScrollListener = lenis.on('scroll', (current) => {
      if (current.isScrolling) wake()
    })
    document.addEventListener('visibilitychange', handleVisibilityChange)
    wake()
  })

  onBeforeUnmount(() => {
    stopFrame()
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    removeVirtualScrollListener()
    removeScrollListener()
    lenis?.destroy()
    lenis = null
  })
}
