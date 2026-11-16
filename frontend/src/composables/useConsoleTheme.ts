import { onMounted, onUnmounted, toValue, watch } from 'vue'
import type { MaybeRefOrGetter, WatchStopHandle } from 'vue'

// Portaled dialogs must inherit the same tokens as the console. Reference
// counting also keeps the theme stable when two layouts overlap in a transition.
let activeLayouts = 0

export function useConsoleTheme(enabled: MaybeRefOrGetter<boolean> = true): void {
  let ownsTheme = false
  let stop: WatchStopHandle | undefined
  function syncOwner(active: boolean): void {
    if (active === ownsTheme) return
    ownsTheme = active
    activeLayouts = Math.max(0, activeLayouts + (active ? 1 : -1))
    document.documentElement.classList.toggle('kitty-console-theme', activeLayouts > 0)
  }
  onMounted(() => {
    stop = watch(() => toValue(enabled), syncOwner, { immediate: true })
  })
  onUnmounted(() => {
    stop?.()
    syncOwner(false)
  })
}
