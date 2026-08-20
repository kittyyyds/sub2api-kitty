<script setup lang="ts">
import { onBeforeUnmount, onMounted, shallowRef } from 'vue'

const cursorRef = shallowRef<HTMLElement | null>(null)
const transmissionRef = shallowRef<HTMLElement | null>(null)
const lensRef = shallowRef<HTMLElement | null>(null)

const uniqueId = Math.random().toString(36).slice(2, 9)
const clipId = `hr-glass-cursor-clip-${uniqueId}`
const filterId = `hr-glass-cursor-filter-${uniqueId}`
const pointCount = 34
const idleFrameDelay = 800

let cleanup = (): void => {}

onMounted(() => {
  const cursor = cursorRef.value
  const transmission = transmissionRef.value
  const lens = lensRef.value
  const host = cursor?.closest<HTMLElement>('.heyroute-preview')
  const clip = cursor?.querySelector<SVGClipPathElement>('.hr-glass-cursor-clip')
  const trailPath = clip?.querySelector<SVGPathElement>('path')
  const circles = clip ? Array.from(clip.querySelectorAll<SVGCircleElement>('circle')) : []
  const displacement = cursor?.querySelector<SVGFEDisplacementMapElement>('feDisplacementMap')
  const finePointer = window.matchMedia('(hover: hover) and (pointer: fine)').matches
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (!cursor || !transmission || !lens || !host || !trailPath || !displacement || !finePointer || reduceMotion) return

  const cursorElement: HTMLElement = cursor
  const hostElement: HTMLElement = host
  const transmissionElement: HTMLElement = transmission
  const lensElement: HTMLElement = lens
  const trailPathElement: SVGPathElement = trailPath
  const displacementElement: SVGFEDisplacementMapElement = displacement

  const points = Array.from({ length: pointCount }, () => ({ x: -9999, y: -9999 }))
  const target = { x: -9999, y: -9999 }
  const smooth = { x: -9999, y: -9999 }
  let visible = false
  let opacity = 0
  let frameId = 0
  let previousTime = performance.now()
  let previousX = -9999
  let previousY = -9999
  let velocityX = 0
  let velocityY = 0
  let angle = 205
  let targetAngle = 205
  let lastPointerMoveTime = 0

  const resetDom = (): void => {
    hostElement.classList.remove('hr-cursor-active')
    cursorElement.classList.remove('is-active')
    transmissionElement.style.opacity = '0'
    lensElement.style.width = '0'
    lensElement.style.height = '0'
    lensElement.style.opacity = '0'
    circles.forEach((circle) => circle.setAttribute('r', '0'))
    trailPathElement.setAttribute('d', '')
  }

  const suspend = (): void => {
    window.cancelAnimationFrame(frameId)
    frameId = 0
    visible = false
    opacity = 0
    target.x = -9999
    target.y = -9999
    smooth.x = -9999
    smooth.y = -9999
    previousX = -9999
    previousY = -9999
    velocityX = 0
    velocityY = 0
    points.forEach((point) => {
      point.x = -9999
      point.y = -9999
    })
    resetDom()
  }

  const hide = (): void => {
    target.x = -9999
    target.y = -9999
    if (!frameId) {
      previousTime = performance.now()
      frameId = window.requestAnimationFrame(draw)
    }
  }

  const buildTrail = (count: number, halfWidth: number): string => {
    const activePoints = points.slice(0, count)
    const left: Array<{ x: number; y: number }> = []
    const right: Array<{ x: number; y: number }> = []

    activePoints.forEach((point, index) => {
      const previous = activePoints[Math.max(0, index - 1)]
      const next = activePoints[Math.min(activePoints.length - 1, index + 1)]
      const dx = next.x - previous.x
      const dy = next.y - previous.y
      const length = Math.hypot(dx, dy) || 1
      const progress = index / Math.max(activePoints.length - 1, 1)
      const width = halfWidth * (0.16 + 0.84 * Math.pow(1 - progress, 1.9))
      const offsetX = -dy / length * width
      const offsetY = dx / length * width
      left.push({ x: point.x + offsetX, y: point.y + offsetY })
      right.push({ x: point.x - offsetX, y: point.y - offsetY })
    })

    const leftPath = left.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`).join(' ')
    const rightPath = right.slice().reverse().map((point) => `L ${point.x} ${point.y}`).join(' ')
    return `${leftPath} ${rightPath} Z`
  }

  function draw(now: number): void {
    frameId = 0
    const deltaSeconds = Math.min((now - previousTime) / 1000, 0.05)
    previousTime = now
    visible = target.x > -999

    if (visible) {
      if (smooth.x < -999) {
        smooth.x = target.x
        smooth.y = target.y
        points.forEach((point) => {
          point.x = smooth.x
          point.y = smooth.y
        })
      } else {
        const smoothing = 1 - Math.pow(0.04, Math.min(deltaSeconds * 60, 4))
        smooth.x += (target.x - smooth.x) * smoothing
        smooth.y += (target.y - smooth.y) * smoothing
        const distance = Math.hypot(smooth.x - points[0].x, smooth.y - points[0].y)
        const steps = Math.min(pointCount, Math.max(1, Math.ceil(distance / 4.5)))
        const headX = points[0].x
        const headY = points[0].y
        for (let index = pointCount - 1; index >= steps; index -= 1) {
          points[index].x = points[index - steps].x
          points[index].y = points[index - steps].y
        }
        for (let index = 0; index < steps; index += 1) {
          const progress = (steps - index) / steps
          points[index].x = headX + (smooth.x - headX) * progress
          points[index].y = headY + (smooth.y - headY) * progress
        }
      }

      if (previousX > -999) {
        const inverseDelta = 1 / Math.max(deltaSeconds, 1 / 240)
        const rawVelocityX = (smooth.x - previousX) * inverseDelta
        const rawVelocityY = (smooth.y - previousY) * inverseDelta
        const response = 1 - Math.exp(-10 * deltaSeconds)
        velocityX += (rawVelocityX - velocityX) * response
        velocityY += (rawVelocityY - velocityY) * response
        if (Math.hypot(velocityX, velocityY) > 24) {
          targetAngle = Math.atan2(velocityY, velocityX) * 180 / Math.PI + 120
        }
      }
      previousX = smooth.x
      previousY = smooth.y
    }

    const opacityTarget = visible ? 1 : 0
    const opacityResponse = 1 - Math.exp(-(visible ? 8 : 3) * deltaSeconds)
    opacity += (opacityTarget - opacity) * opacityResponse
    if (opacity < 0.002) opacity = 0

    const angleDelta = (targetAngle - angle + 540) % 360 - 180
    angle += angleDelta * (1 - Math.exp(-4.5 * deltaSeconds))
    const radius = Math.min(window.innerHeight * 0.026 * 3.85, 32) * opacity
    const trailCount = visible ? pointCount : 0
    const halfWidth = Math.max(3, 32 * 2 * opacity) / 2

    circles.forEach((circle, index) => {
      if (index === 0 && opacity > 0) {
        circle.setAttribute('cx', String(points[0].x))
        circle.setAttribute('cy', String(points[0].y))
        circle.setAttribute('r', String(Math.min(window.innerHeight * 0.026, 32 / 3.85) * 3.85 * opacity))
      } else {
        circle.setAttribute('r', '0')
      }
    })

    trailPathElement.setAttribute('d', trailCount > 1 && opacity > 0 ? buildTrail(trailCount, halfWidth) : '')
    transmissionElement.style.opacity = String(opacity)
    displacementElement.setAttribute('scale', String(18 + Math.sin(now * 0.0016) * 1.8))

    const diameter = Math.max(0, radius * 2 * (1 + Math.sin(now * 0.00215) * 0.014))
    lensElement.style.left = `${smooth.x}px`
    lensElement.style.top = `${smooth.y}px`
    lensElement.style.width = `${diameter}px`
    lensElement.style.height = `${diameter}px`
    lensElement.style.opacity = String(opacity)
    lensElement.style.setProperty('--hr-cursor-angle', `${angle}deg`)

    if (visible && opacity > 0.35 && diameter > 24) {
      hostElement.classList.add('hr-cursor-active')
      cursorElement.classList.add('is-active')
    }

    if (!visible && opacity === 0) {
      smooth.x = -9999
      smooth.y = -9999
      previousX = -9999
      previousY = -9999
      velocityX = 0
      velocityY = 0
      points.forEach((point) => {
        point.x = -9999
        point.y = -9999
      })
      resetDom()
      return
    }

    const distanceToTarget = visible ? Math.hypot(target.x - smooth.x, target.y - smooth.y) : Number.POSITIVE_INFINITY
    const isIdle = visible
      && now - lastPointerMoveTime >= idleFrameDelay
      && distanceToTarget < 0.2
      && Math.abs(angleDelta) < 0.2
      && opacity > 0.995
    if (isIdle) return

    frameId = window.requestAnimationFrame(draw)
  }

  const handlePointerMove = (event: PointerEvent): void => {
    if (event.pointerType && event.pointerType !== 'mouse' && event.pointerType !== 'pen') return
    if (event.target instanceof Element && event.target.closest('.hr-header')) {
      hide()
      return
    }
    target.x = event.clientX
    target.y = event.clientY
    lastPointerMoveTime = performance.now()
    if (!frameId) {
      previousTime = performance.now()
      frameId = window.requestAnimationFrame(draw)
    }
  }

  const handleVisibility = (): void => {
    if (document.hidden) suspend()
  }

  window.addEventListener('pointermove', handlePointerMove, { passive: true })
  document.documentElement.addEventListener('pointerleave', hide, { passive: true })
  window.addEventListener('blur', hide)
  document.addEventListener('visibilitychange', handleVisibility)

  cleanup = () => {
    suspend()
    window.removeEventListener('pointermove', handlePointerMove)
    document.documentElement.removeEventListener('pointerleave', hide)
    window.removeEventListener('blur', hide)
    document.removeEventListener('visibilitychange', handleVisibility)
    resetDom()
  }
})

onBeforeUnmount(() => cleanup())
</script>

<template>
  <div ref="cursorRef" class="hr-glass-cursor" aria-hidden="true">
    <svg class="hr-glass-cursor-defs" width="0" height="0">
      <defs>
        <filter :id="filterId" x="-12%" y="-12%" width="124%" height="124%" color-interpolation-filters="sRGB">
          <feTurbulence type="fractalNoise" baseFrequency="0.008 0.012" numOctaves="2" seed="7" stitchTiles="stitch" result="noise" />
          <feDisplacementMap in="SourceGraphic" in2="noise" scale="18" xChannelSelector="R" yChannelSelector="B" result="distorted" />
          <feGaussianBlur in="distorted" stdDeviation="0.34" />
        </filter>
        <clipPath :id="clipId" class="hr-glass-cursor-clip" clipPathUnits="userSpaceOnUse">
          <circle v-for="index in pointCount" :key="index" r="0" />
          <path fill="#fff" />
        </clipPath>
      </defs>
    </svg>
    <div
      ref="transmissionRef"
      class="hr-glass-cursor-transmission"
      :style="{ clipPath: `url(#${clipId})`, backdropFilter: `url(#${filterId}) blur(0.55px) saturate(1.16) contrast(1.045) brightness(1.018)` }"
    />
    <div ref="lensRef" class="hr-glass-cursor-lens">
      <span class="hr-glass-cursor-surface" />
      <span class="hr-glass-cursor-rim" />
      <span class="hr-glass-cursor-glint" />
    </div>
  </div>
</template>

<style scoped>
.hr-glass-cursor,
.hr-glass-cursor-transmission {
  position: fixed;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.hr-glass-cursor {
  z-index: 100;
  overflow: hidden;
}

.hr-glass-cursor-defs {
  position: fixed;
  width: 0;
  height: 0;
  overflow: hidden;
}

.hr-glass-cursor-transmission {
  opacity: 0;
  background: rgba(255, 255, 255, 0.018);
}

.hr-glass-cursor-lens {
  position: fixed;
  top: 0;
  left: 0;
  width: 0;
  height: 0;
  border-radius: 50%;
  filter: drop-shadow(0 3px 10px rgba(25, 48, 58, 0.11));
  isolation: isolate;
  opacity: 0;
  transform: translate(-50%, -50%);
  pointer-events: none;
}

.hr-glass-cursor.is-active .hr-glass-cursor-transmission {
  will-change: clip-path;
}

.hr-glass-cursor.is-active .hr-glass-cursor-lens {
  will-change: width, height, transform, opacity;
}

.hr-glass-cursor-lens > span {
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
}

.hr-glass-cursor-surface {
  background:
    radial-gradient(circle at 31% 25%, rgba(255, 255, 255, 0.2), transparent 36%),
    radial-gradient(circle at 70% 76%, rgba(28, 66, 81, 0.12), transparent 50%),
    radial-gradient(circle at 50% 50%, transparent 58%, rgba(255, 255, 255, 0.08) 76%, rgba(255, 255, 255, 0.24) 96%, transparent 100%),
    rgba(255, 255, 255, 0.018);
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.34),
    inset 3px 4px 11px rgba(255, 255, 255, 0.16),
    inset -4px -5px 12px rgba(35, 68, 80, 0.1);
}

.hr-glass-cursor-rim {
  background: conic-gradient(
    from var(--hr-cursor-angle, 205deg),
    rgba(255, 255, 255, 0.2) 0deg,
    rgba(143, 224, 255, 0.72) 42deg,
    rgba(255, 255, 255, 0.9) 82deg,
    rgba(255, 190, 216, 0.52) 116deg,
    rgba(255, 233, 171, 0.24) 154deg,
    rgba(255, 255, 255, 0.12) 214deg,
    rgba(114, 211, 236, 0.42) 286deg,
    rgba(255, 255, 255, 0.2) 360deg
  );
  mask: radial-gradient(farthest-side, transparent calc(100% - 3.2px), #000 calc(100% - 2.45px), #000 calc(100% - 0.55px), transparent 100%);
  opacity: 0.92;
  filter: saturate(1.12);
}

.hr-glass-cursor-glint {
  inset: 1px !important;
  background: conic-gradient(
    from var(--hr-cursor-angle, 205deg),
    transparent 0deg 30deg,
    rgba(189, 236, 255, 0.24) 48deg,
    rgba(255, 255, 255, 0.92) 68deg,
    rgba(255, 255, 255, 0.18) 90deg,
    transparent 112deg 360deg
  );
  mask: radial-gradient(farthest-side, transparent calc(100% - 2px), #000 calc(100% - 1.15px), transparent 100%);
  opacity: 0.94;
  filter: blur(0.2px);
}

:global(.heyroute-preview.hr-cursor-active),
:global(.heyroute-preview.hr-cursor-active *) {
  cursor: none !important;
}

:global(.heyroute-preview.hr-cursor-active .hr-header),
:global(.heyroute-preview.hr-cursor-active .hr-header *) {
  cursor: auto !important;
}

@media (hover: none), (pointer: coarse), (prefers-reduced-motion: reduce) {
  .hr-glass-cursor {
    display: none;
  }

  :global(.heyroute-preview.hr-cursor-active),
  :global(.heyroute-preview.hr-cursor-active *) {
    cursor: auto !important;
  }
}
</style>
