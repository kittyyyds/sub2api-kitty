import { onBeforeUnmount, onMounted } from 'vue'
import type { ShallowRef } from 'vue'

interface SpringValue {
  current: number
  target: number
  velocity: number
}

const clamp = (value: number): number => Math.min(1, Math.max(0, value))

const smoothStep = (value: number, start: number, end: number): number => {
  const progress = clamp((value - start) / (end - start))
  return progress * progress * progress * (progress * (progress * 6 - 15) + 10)
}

export function useHeyRouteStepSequence(
  sectionRef: ShallowRef<HTMLElement | null>,
  gridRef: ShallowRef<HTMLOListElement | null>,
): void {
  let scrollFrame = 0
  let springFrame = 0
  let springStartedAt = 0
  let hasMeasuredProgress = false
  let disposed = false
  let resizeObserver: ResizeObserver | null = null
  let pageRoot: HTMLElement | null = null

  const springs: SpringValue[] = Array.from({ length: 3 }, () => ({
    current: 0,
    target: 0,
    velocity: 0,
  }))

  const renderSpringValues = (steps: HTMLElement[]): void => {
    springs.forEach((spring, index) => {
      steps[index]?.style.setProperty('--hr-step-progress', clamp(spring.current).toFixed(4))
    })
  }

  const animateSprings = (time: number, steps: HTMLElement[]): void => {
    springFrame = 0
    if (!springStartedAt) springStartedAt = time

    const delta = Math.min(Math.max((time - springStartedAt) / 1000, 0), 1 / 30)
    springStartedAt = time
    let shouldContinue = false

    springs.forEach((spring) => {
      const offset = spring.current - spring.target
      const decay = Math.exp(-11.5 * delta)
      const displacement = (spring.velocity + 11.5 * offset) * delta

      spring.current = spring.target + (offset + displacement) * decay
      spring.velocity = (spring.velocity - 11.5 * displacement) * decay

      if (Math.abs(spring.current - spring.target) < 0.0008 && Math.abs(spring.velocity) < 0.008) {
        spring.current = spring.target
        spring.velocity = 0
      } else {
        shouldContinue = true
      }
    })

    renderSpringValues(steps)
    if (shouldContinue) {
      springFrame = window.requestAnimationFrame((nextTime) => animateSprings(nextTime, steps))
    } else {
      springStartedAt = 0
    }
  }

  const setSpringTargets = (values: number[], steps: HTMLElement[], immediate: boolean): void => {
    springs.forEach((spring, index) => {
      spring.target = clamp(values[index] ?? 0)
      if (immediate) {
        spring.current = spring.target
        spring.velocity = 0
      }
    })

    if (immediate) {
      window.cancelAnimationFrame(springFrame)
      springFrame = 0
      springStartedAt = 0
      renderSpringValues(steps)
      return
    }

    if (!springFrame) {
      springFrame = window.requestAnimationFrame((time) => animateSprings(time, steps))
    }
  }

  const update = (): void => {
    scrollFrame = 0
    const grid = gridRef.value
    if (!grid) return

    const steps = Array.from(grid.querySelectorAll<HTMLElement>('.hr-step-item'))
    const nodes = steps.map((step) => step.querySelector<HTMLElement>('.hr-step-index'))
    if (steps.length !== 3 || nodes.some((node) => !node)) return

    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const mobile = window.innerWidth <= 900
    const rootElement = !mobile ? pageRoot : null
    const viewportHeight = rootElement?.clientHeight || window.innerHeight
    const rootTop = rootElement?.getBoundingClientRect().top || 0

    grid.dataset.scrollMotion = mobile ? 'mobile' : 'desktop'

    if (reduceMotion) {
      grid.style.setProperty('--hr-step-line-progress', '1')
      grid.style.setProperty('--hr-step-line-clip', '0%')
      setSpringTargets([1, 1, 1], steps, true)
      steps.forEach((step) => step.style.setProperty('--hr-step-node-progress', '1'))
      hasMeasuredProgress = true
      return
    }

    const resolvedNodes = nodes as HTMLElement[]

    if (mobile) {
      const centers = resolvedNodes.map((node, index) => (
        steps[index].offsetTop
        + (node.parentElement?.offsetTop ?? 0)
        + node.offsetTop
        + node.offsetHeight / 2
      ))
      const firstCenter = centers[0]
      const lastCenter = centers[2]
      const tail = Math.min(96, viewportHeight * 0.12)
      const lineHeight = Math.max(lastCenter - firstCenter + tail, 1)
      const secondStop = clamp((centers[1] - firstCenter) / lineHeight)
      const thirdStop = clamp((lastCenter - firstCenter) / lineHeight)
      const fadeStart = thirdStop + (1 - thirdStop) * 0.6
      const lineX = steps[0].offsetLeft
        + (resolvedNodes[0].parentElement?.offsetLeft ?? 0)
        + resolvedNodes[0].offsetLeft
        + resolvedNodes[0].offsetWidth / 2

      grid.style.setProperty('--hr-step-line-x', `${lineX}px`)
      grid.style.setProperty('--hr-step-line-top', `${firstCenter}px`)
      grid.style.setProperty('--hr-step-line-height', `${lineHeight}px`)
      grid.style.setProperty('--hr-step-line-fade-start', `${(fadeStart * 100).toFixed(3)}%`)

      const stepProgress = steps.map((step) => smoothStep(
        viewportHeight - (step.getBoundingClientRect().top - rootTop),
        viewportHeight * 0.08,
        viewportHeight * 0.38,
      ))
      setSpringTargets(stepProgress, steps, !hasMeasuredProgress)
      stepProgress.forEach((progress, index) => {
        steps[index].style.setProperty('--hr-step-node-progress', progress.toFixed(4))
      })

      let lineProgress = 0
      if (stepProgress[1] > 0) lineProgress = secondStop * smoothStep(stepProgress[1], 0, 0.72)
      if (stepProgress[2] > 0) {
        lineProgress = secondStop + (thirdStop - secondStop) * smoothStep(stepProgress[2], 0, 0.72)
      }
      if (stepProgress[2] > 0.72) {
        lineProgress = thirdStop + (1 - thirdStop) * smoothStep(stepProgress[2], 0.72, 1)
      }

      const resolvedLineProgress = clamp(lineProgress)
      grid.style.setProperty('--hr-step-line-progress', resolvedLineProgress.toFixed(4))
      grid.style.setProperty('--hr-step-line-clip', `${((1 - resolvedLineProgress) * 100).toFixed(3)}%`)
      hasMeasuredProgress = true
      return
    }

    const centers = resolvedNodes.map((node, index) => (
      steps[index].offsetLeft
      + (node.parentElement?.offsetLeft ?? 0)
      + node.offsetLeft
      + node.offsetWidth / 2
    ))
    const firstCenter = centers[0]
    const gridRect = grid.getBoundingClientRect()
    const rootRight = rootElement?.getBoundingClientRect().right ?? window.innerWidth
    const rightExtension = Math.max(0, rootRight - gridRect.right)
    const lineLength = Math.max(grid.clientWidth + rightExtension - firstCenter, 1)
    const secondStop = clamp((centers[1] - firstCenter) / lineLength)
    const thirdStop = clamp((centers[2] - firstCenter) / lineLength)
    const fadeStart = thirdStop + (1 - thirdStop) * 0.6
    const lineY = steps[0].offsetTop
      + (resolvedNodes[0].parentElement?.offsetTop ?? 0)
      + resolvedNodes[0].offsetTop
      + resolvedNodes[0].offsetHeight / 2

    grid.style.setProperty('--hr-step-line-left', `${firstCenter}px`)
    grid.style.setProperty('--hr-step-line-right', `${-rightExtension}px`)
    grid.style.setProperty('--hr-step-line-y', `${lineY}px`)
    grid.style.setProperty('--hr-step-line-fade-start', `${(fadeStart * 100).toFixed(3)}%`)

    const firstNodeViewportY = grid.getBoundingClientRect().top - rootTop + lineY
    const sectionProgress = clamp((viewportHeight - firstNodeViewportY) / (viewportHeight * 0.34))
    setSpringTargets([
      smoothStep(sectionProgress, 0, 0.24),
      smoothStep(sectionProgress, 0.28, 0.56),
      smoothStep(sectionProgress, 0.6, 0.88),
    ], steps, !hasMeasuredProgress)

    let lineProgress = 0
    if (sectionProgress >= 0.08 && sectionProgress < 0.38) {
      lineProgress = secondStop * smoothStep(sectionProgress, 0.08, 0.38)
    } else if (sectionProgress >= 0.38 && sectionProgress < 0.43) {
      lineProgress = secondStop
    } else if (sectionProgress >= 0.43 && sectionProgress < 0.73) {
      lineProgress = secondStop + (thirdStop - secondStop) * smoothStep(sectionProgress, 0.43, 0.73)
    } else if (sectionProgress >= 0.73 && sectionProgress < 0.78) {
      lineProgress = thirdStop
    } else if (sectionProgress >= 0.78) {
      lineProgress = thirdStop + (1 - thirdStop) * smoothStep(sectionProgress, 0.78, 0.98)
    }

    const resolvedLineProgress = clamp(lineProgress)
    grid.style.setProperty('--hr-step-line-progress', resolvedLineProgress.toFixed(4))
    grid.style.setProperty('--hr-step-line-clip', `${((1 - resolvedLineProgress) * 100).toFixed(3)}%`)

    const nodeProgress = [
      smoothStep(sectionProgress, 0, 0.08),
      smoothStep(resolvedLineProgress, secondStop - 0.025, secondStop),
      smoothStep(resolvedLineProgress, thirdStop - 0.025, thirdStop),
    ]
    nodeProgress.forEach((progress, index) => {
      steps[index].style.setProperty('--hr-step-node-progress', progress.toFixed(4))
    })
    hasMeasuredProgress = true
  }

  const scheduleUpdate = (): void => {
    if (!scrollFrame) scrollFrame = window.requestAnimationFrame(update)
  }

  const measure = (): void => {
    hasMeasuredProgress = false
    window.cancelAnimationFrame(scrollFrame)
    scrollFrame = 0
    update()
  }

  onMounted(() => {
    const section = sectionRef.value
    const grid = gridRef.value
    if (!section || !grid) return

    pageRoot = section.closest<HTMLElement>('.heyroute-preview')
    pageRoot?.addEventListener('scroll', scheduleUpdate, { passive: true })
    window.addEventListener('scroll', scheduleUpdate, { passive: true })
    window.addEventListener('resize', measure)

    resizeObserver = new ResizeObserver(measure)
    resizeObserver.observe(grid)
    measure()

    void document.fonts?.ready.then(() => {
      if (!disposed) measure()
    })
  })

  onBeforeUnmount(() => {
    disposed = true
    pageRoot?.removeEventListener('scroll', scheduleUpdate)
    window.removeEventListener('scroll', scheduleUpdate)
    window.removeEventListener('resize', measure)
    resizeObserver?.disconnect()
    window.cancelAnimationFrame(scrollFrame)
    window.cancelAnimationFrame(springFrame)
  })
}
