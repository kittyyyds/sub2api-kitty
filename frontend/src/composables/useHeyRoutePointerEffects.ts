import { onBeforeUnmount, onMounted } from 'vue'
import type { ShallowRef } from 'vue'

interface CardBinding {
  element: HTMLElement
  cancelFrame: () => void
  removeListeners: () => void
}

export function useHeyRoutePointerEffects(rootRef: ShallowRef<HTMLElement | null>): void {
  const bindings: CardBinding[] = []

  onMounted(() => {
    const root = rootRef.value
    if (!root) return

    const finePointer = window.matchMedia('(hover: hover) and (pointer: fine)').matches
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (!finePointer || reduceMotion) return

    root.querySelectorAll<HTMLElement>('[data-hr-tilt]').forEach((element) => {
      let frameId = 0
      let latestEvent: PointerEvent | null = null

      const reset = (): void => {
        latestEvent = null
        element.classList.remove('is-pointer-active')
        element.style.setProperty('--hr-card-x', '50%')
        element.style.setProperty('--hr-card-y', '50%')
        element.style.setProperty('--hr-card-rotate-x', '0deg')
        element.style.setProperty('--hr-card-rotate-y', '0deg')
        element.style.setProperty('--hr-card-glare', '0')
      }

      const render = (): void => {
        frameId = 0
        const event = latestEvent
        if (!event) return

        const rect = element.getBoundingClientRect()
        const x = Math.min(1, Math.max(0, (event.clientX - rect.left) / Math.max(rect.width, 1)))
        const y = Math.min(1, Math.max(0, (event.clientY - rect.top) / Math.max(rect.height, 1)))
        const maxTilt = Number(element.dataset.hrTilt) || 4

        element.classList.add('is-pointer-active')
        element.style.setProperty('--hr-card-x', `${(x * 100).toFixed(2)}%`)
        element.style.setProperty('--hr-card-y', `${(y * 100).toFixed(2)}%`)
        element.style.setProperty('--hr-card-rotate-x', `${((0.5 - y) * maxTilt).toFixed(3)}deg`)
        element.style.setProperty('--hr-card-rotate-y', `${((x - 0.5) * maxTilt).toFixed(3)}deg`)
        element.style.setProperty('--hr-card-glare', '1')
      }

      const handlePointerMove = (event: PointerEvent): void => {
        if (event.pointerType === 'touch') return
        latestEvent = event
        if (!frameId) frameId = window.requestAnimationFrame(render)
      }

      const handlePointerLeave = (): void => reset()

      element.addEventListener('pointermove', handlePointerMove, { passive: true })
      element.addEventListener('pointerleave', handlePointerLeave, { passive: true })
      bindings.push({
        element,
        cancelFrame: () => window.cancelAnimationFrame(frameId),
        removeListeners: () => {
          element.removeEventListener('pointermove', handlePointerMove)
          element.removeEventListener('pointerleave', handlePointerLeave)
        },
      })
    })
  })

  onBeforeUnmount(() => {
    bindings.forEach(({ element, cancelFrame, removeListeners }) => {
      cancelFrame()
      removeListeners()
      element.classList.remove('is-pointer-active')
    })
    bindings.length = 0
  })
}
