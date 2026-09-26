<script setup lang="ts">
import { shallowRef, toRef } from 'vue'

import { useHeyRouteFluidCanvas } from '@/composables/useHeyRouteFluidCanvas'
import { primaryHeyRouteTools } from './heyRouteTools'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['hero']
  siteName: string
  siteLogo: string
  isDark: boolean
}

const props = defineProps<Props>()
const canvasRef = shallowRef<HTMLCanvasElement | null>(null)
const { isReady } = useHeyRouteFluidCanvas(canvasRef, toRef(props, 'isDark'))
</script>

<template>
  <section id="hr-hero" class="hr-hero hr-stage" data-hr-section>
    <canvas ref="canvasRef" class="hr-fluid-canvas" :class="{ ready: isReady }" aria-hidden="true" />
    <div class="hr-hero-fallback" aria-hidden="true" />
    <div class="hr-hero-grain" aria-hidden="true" />

    <div class="hr-hero-content">
      <h1>
        <span class="hr-title-line"><span>{{ copy.line1 }}</span></span>
        <span class="hr-title-line"><span>{{ copy.line2 }}</span></span>
      </h1>

      <div class="hr-hero-bottom">
        <div class="hr-compatible-row">
          <strong>{{ copy.compatible }}</strong>
          <div class="hr-tool-list" aria-hidden="true">
            <span v-for="tool in primaryHeyRouteTools" :key="tool.slug" class="hr-tool-logo" :data-tool="tool.slug">
              <img :src="tool.src" :alt="tool.name" />
            </span>
          </div>
          <span>{{ copy.moreTools }}</span>
        </div>
        <p>{{ copy.description }}</p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-hero.hr-stage {
  position: relative;
  display: flex;
  min-height: calc(100dvh - 20px);
  align-items: stretch;
  overflow: hidden;
  margin: 10px;
  border-radius: 28px;
  color: var(--hr-hero-ink);
  background: #c9c3aa;
}

.hr-fluid-canvas,
.hr-hero-fallback,
.hr-hero-grain {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.hr-fluid-canvas {
  z-index: 1;
  opacity: 0;
  transition: opacity 500ms ease;
}

.hr-fluid-canvas.ready {
  opacity: 1;
}

.hr-hero-fallback {
  z-index: 0;
  background:
    linear-gradient(125deg, rgba(242, 173, 147, 0.8), rgba(228, 211, 156, 0.88) 34%, rgba(147, 194, 192, 0.76) 67%, rgba(223, 155, 174, 0.72));
}

.hr-hero-grain {
  z-index: 2;
  pointer-events: none;
  opacity: 0.065;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.72' numOctaves='3'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  mix-blend-mode: soft-light;
}

.hr-hero-content {
  position: relative;
  z-index: 3;
  display: flex;
  width: 100%;
  flex-direction: column;
  justify-content: space-between;
  padding: clamp(142px, 17vh, 190px) 30px 40px;
}

.hr-hero h1 {
  max-width: 14ch;
  margin: 0;
  color: var(--hr-hero-ink);
  font-size: 6.75rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.94;
  text-wrap: balance;
}

.hr-title-line {
  display: block;
  overflow: hidden;
  padding-bottom: 0.05em;
}

.hr-title-line > span {
  display: block;
  animation: hr-title-rise 900ms cubic-bezier(.16, 1, .3, 1) both;
}

.hr-title-line:nth-child(2) > span {
  animation-delay: 110ms;
}

.hr-hero-bottom {
  display: grid;
  gap: 12px;
  max-width: min(1120px, calc(100% - 20px));
  animation: hr-copy-rise 700ms cubic-bezier(.16, 1, .3, 1) 260ms both;
}

.hr-compatible-row,
.hr-tool-list {
  display: flex;
  align-items: center;
}

.hr-compatible-row {
  flex-wrap: wrap;
  gap: 8px;
  color: rgba(246, 248, 244, 0.88);
  font-size: 13px;
  font-weight: 620;
}

.hr-tool-list {
  gap: 6px;
}

.hr-tool-logo {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 1px solid rgba(246, 248, 244, 0.62);
  border-radius: 8px;
  background: rgba(246, 248, 244, 0.3);
  box-shadow: inset 0 0 0 1px rgba(246, 248, 244, 0.12), 0 8px 18px rgba(50, 33, 28, 0.12);
  backdrop-filter: blur(12px) saturate(140%);
}

.hr-tool-logo img {
  display: block;
  width: 22px;
  height: 22px;
  object-fit: contain;
}

.hr-tool-logo[data-tool='cursor'] img,
.hr-tool-logo[data-tool='hermes'] img {
  border-radius: 4px;
}

.hr-hero-bottom p {
  max-width: 1080px;
  margin: 0;
  color: rgba(246, 248, 244, 0.9);
  font-size: 1.25rem;
  font-weight: 430;
  letter-spacing: 0;
  line-height: 1.45;
  text-shadow: 0 2px 14px rgba(37, 22, 22, 0.18);
}

@keyframes hr-title-rise {
  from { transform: translateY(112%); }
  to { transform: translateY(0); }
}

@keyframes hr-copy-rise {
  from { opacity: 0; transform: translateY(18px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (max-width: 1200px) and (min-width: 821px) {
  .hr-hero h1 {
    font-size: 5.25rem;
  }
}

@media (max-width: 820px) {
  .hr-hero.hr-stage {
    min-height: calc(100dvh - 20px);
    margin: 10px;
    border-radius: 28px;
  }

  .hr-hero-content {
    padding: 132px 10px 28px;
  }

  .hr-hero h1 {
    max-width: none;
    font-size: 2.05rem;
    line-height: 0.96;
  }

  .hr-hero-bottom {
    max-width: 100%;
    gap: 14px;
  }

  .hr-compatible-row {
    align-items: flex-start;
  }

  .hr-compatible-row > strong,
  .hr-compatible-row > span:last-child {
    flex-basis: 100%;
  }

  .hr-hero-bottom p {
    font-size: 1.03rem;
    line-height: 1.5;
  }

}

@media (prefers-reduced-motion: reduce) {
  .hr-fluid-canvas,
  .hr-title-line > span,
  .hr-hero-bottom {
    animation: none;
    transition: none;
    transform: none;
  }
}
</style>
