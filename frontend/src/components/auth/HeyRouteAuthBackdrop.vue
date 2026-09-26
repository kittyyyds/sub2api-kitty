<script setup lang="ts">
import { onActivated, onDeactivated, shallowRef } from 'vue'

import { useHeyRouteFluidCanvas } from '@/composables/useHeyRouteFluidCanvas'
import { useTheme } from '@/composables/useTheme'

const canvasRef = shallowRef<HTMLCanvasElement | null>(null)
const isActive = shallowRef(true)
const { isDark } = useTheme()
const { isReady } = useHeyRouteFluidCanvas(canvasRef, isDark, isActive)

onActivated(() => {
  isActive.value = true
})

onDeactivated(() => {
  isActive.value = false
})
</script>

<template>
  <div class="hr-auth-backdrop" :data-theme="isDark ? 'dark' : 'light'" aria-hidden="true">
    <div class="hr-auth-backdrop__fallback" />
    <canvas ref="canvasRef" class="hr-auth-backdrop__canvas" :class="{ ready: isReady }" />
    <div class="hr-auth-backdrop__grain" />
  </div>
</template>

<style scoped>
.hr-auth-backdrop {
  position: fixed;
  z-index: 0;
  inset: 0;
  overflow: hidden;
  background: #e8c5ae;
  pointer-events: none;
}

.hr-auth-backdrop__fallback,
.hr-auth-backdrop__canvas,
.hr-auth-backdrop__grain {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.hr-auth-backdrop__fallback {
  background: linear-gradient(125deg, #ffad91, #f3d66f 34%, #69c7c3 67%, #ed8fac);
}

.hr-auth-backdrop__canvas {
  opacity: 0;
  transition: opacity 360ms ease;
}

.hr-auth-backdrop__canvas.ready {
  opacity: 1;
}

.hr-auth-backdrop__canvas:not(.ready) {
  transition-duration: 0s;
}

.hr-auth-backdrop__grain {
  opacity: 0.035;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.72' numOctaves='3'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  mix-blend-mode: soft-light;
}

.hr-auth-backdrop[data-theme='dark'] {
  background: #171b27;
}

.hr-auth-backdrop[data-theme='dark'] .hr-auth-backdrop__fallback {
  background: linear-gradient(125deg, #334b82, #804b80 36%, #168e9a 70%, #bd6455);
}

@media (prefers-reduced-motion: reduce) {
  .hr-auth-backdrop__canvas {
    transition: none;
  }
}
</style>
