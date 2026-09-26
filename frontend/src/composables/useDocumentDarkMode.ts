import { onMounted, onUnmounted, readonly, shallowRef } from 'vue'

function detectDocumentDarkMode(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}

export function useDocumentDarkMode() {
  const isDarkMode = shallowRef(detectDocumentDarkMode())
  let observer: MutationObserver | null = null

  onMounted(() => {
    isDarkMode.value = detectDocumentDarkMode()
    if (typeof document === 'undefined' || typeof MutationObserver === 'undefined') return

    observer = new MutationObserver(() => {
      isDarkMode.value = detectDocumentDarkMode()
    })
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class']
    })
  })

  onUnmounted(() => {
    observer?.disconnect()
    observer = null
  })

  return {
    isDarkMode: readonly(isDarkMode)
  }
}
