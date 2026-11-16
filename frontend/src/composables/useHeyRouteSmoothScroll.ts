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

  let desktopQuery: MediaQueryList | null = null
  let motionQuery: MediaQueryList | null = null

  function start(): void {
    const root = rootRef.value
    if (!root || lenis) return

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
    wake()
  }

  function stop(): void {
    stopFrame()
    removeVirtualScrollListener()
    removeScrollListener()
    removeVirtualScrollListener = () => {}
    removeScrollListener = () => {}
    lenis?.destroy()
    lenis = null
  }

  // Below 901px the window scrolls, not the root; a Lenis left running there swallows wheel events.
  function sync(): void {
    if (desktopQuery?.matches && !motionQuery?.matches) start()
    else stop()
  }

  onMounted(() => {
    desktopQuery = window.matchMedia('(min-width: 901px)')
    motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
    desktopQuery.addEventListener('change', sync)
    motionQuery.addEventListener('change', sync)
    document.addEventListener('visibilitychange', handleVisibilityChange)
    sync()
  })

  onBeforeUnmount(() => {
    desktopQuery?.removeEventListener('change', sync)
    motionQuery?.removeEventListener('change', sync)
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    stop()
  })
}
