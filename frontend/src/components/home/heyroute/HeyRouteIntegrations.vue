<script setup lang="ts">
import { heyRouteTools } from './heyRouteTools'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['integrations']
  siteLogo: string
  siteName: string
}

defineProps<Props>()

const flowPoints = [
  [50, 8],
  [79.7, 20.3],
  [92, 50],
  [79.7, 79.7],
  [50, 92],
  [20.3, 79.7],
  [8, 50],
  [20.3, 20.3],
] as const

const flowPaths = flowPoints.map(([x, y]) => {
  const deltaX = 50 - x
  const deltaY = 50 - y
  return `M ${x} ${y} C ${x + deltaX * 0.42} ${y + deltaY * 0.12}, ${x + deltaX * 0.62} ${50 - deltaY * 0.12}, 50 50`
})
</script>

<template>
  <section id="hr-integrations" class="hr-integrations hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-integrations-shell">
      <header class="hr-integration-copy">
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
      </header>

      <div class="hr-center-flow" role="region" :aria-label="copy.title">
        <div class="hr-center-flow-haze" aria-hidden="true" />

        <svg class="hr-center-flow-svg" viewBox="0 0 100 100" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
          <defs>
            <linearGradient id="hr-center-flow-pulse" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stop-color="var(--hr-flow-pulse-soft)" stop-opacity="0" />
              <stop offset="48%" stop-color="var(--hr-flow-pulse)" stop-opacity="0.76" />
              <stop offset="100%" stop-color="var(--hr-flow-pulse-bright)" stop-opacity="1" />
            </linearGradient>
            <filter id="hr-center-flow-glow" x="-80%" y="-80%" width="260%" height="260%">
              <feGaussianBlur stdDeviation="1.8" />
            </filter>
            <filter id="hr-center-flow-core-glow" x="-120%" y="-120%" width="340%" height="340%">
              <feGaussianBlur stdDeviation="4.8" />
            </filter>
          </defs>

          <g class="hr-center-flow-base-lines">
            <path v-for="(path, index) in flowPaths" :key="`base-${index}`" :d="path" pathLength="100" />
          </g>
          <g class="hr-center-flow-pulse-lines">
            <template v-for="(path, index) in flowPaths" :key="`pulse-${index}`">
              <path
                class="hr-center-flow-pulse-blur"
                :d="path"
                pathLength="100"
                :style="{ '--hr-pulse-delay': `${index * -0.62}s` }"
                filter="url(#hr-center-flow-glow)"
              />
              <path
                class="hr-center-flow-pulse"
                :d="path"
                pathLength="100"
                :style="{ '--hr-pulse-delay': `${index * -0.62}s` }"
                stroke="url(#hr-center-flow-pulse)"
              />
            </template>
          </g>
          <circle
            class="hr-center-flow-core-glow"
            cx="50"
            cy="50"
            r="13"
            filter="url(#hr-center-flow-core-glow)"
          />
        </svg>

        <div class="hr-center-flow-center" aria-hidden="true">
          <span class="hr-center-flow-center-ring" />
          <img :src="siteLogo || '/logo.svg'" :alt="siteName" />
        </div>

        <a
          v-for="(tool, index) in heyRouteTools"
          :key="tool.slug"
          class="hr-center-flow-node"
          :data-tool="tool.slug"
          :href="`#hr-steps`"
          :style="{
            '--hr-node-x': `${flowPoints[index][0]}%`,
            '--hr-node-y': `${flowPoints[index][1]}%`,
            '--hr-node-delay': `${index * 0.14}s`,
          }"
          :aria-label="`${tool.name}: ${copy.description}`"
          :title="tool.name"
        >
          <span class="hr-center-flow-node-surface">
            <img :src="tool.src" alt="" loading="lazy" />
            <span class="hr-center-flow-node-dot" aria-hidden="true" />
          </span>
        </a>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-integrations {
  background:
    radial-gradient(circle at 70% 55%, rgba(83, 177, 169, 0.18), transparent 26%),
    radial-gradient(circle at 77% 39%, rgba(232, 155, 119, 0.17), transparent 24%),
    radial-gradient(circle at 91% 76%, rgba(104, 158, 202, 0.16), transparent 27%),
    var(--hr-bg);
}

.hr-integrations-shell {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(580px, 620px);
  gap: clamp(48px, 6.4vw, 110px);
  align-items: center;
}

.hr-integration-copy {
  display: grid;
  max-width: 680px;
  gap: 24px;
}

.hr-integration-copy h2 {
  max-width: 10em;
  margin: 0;
  font-size: 4.8rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.93;
  text-wrap: balance;
}

.hr-integration-copy p {
  max-width: 500px;
  margin: 0;
  color: var(--hr-muted);
  font-size: 17px;
  line-height: 1.68;
}

.hr-center-flow {
  --hr-flow-line: rgba(74, 67, 73, 0.18);
  --hr-flow-pulse-soft: #f2b4a3;
  --hr-flow-pulse: #3c9fa4;
  --hr-flow-pulse-bright: #d87869;
  --hr-flow-glow: rgba(60, 159, 164, 0.42);
  position: relative;
  isolation: isolate;
  width: min(100%, 620px);
  aspect-ratio: 1;
  justify-self: end;
}

.hr-center-flow-haze {
  position: absolute;
  inset: 3%;
  z-index: -1;
  border-radius: 50%;
  background:
    radial-gradient(circle at 50% 50%, rgba(255, 255, 255, 0.7), transparent 28%),
    radial-gradient(circle at 16% 24%, rgba(255, 191, 160, 0.3), transparent 34%),
    radial-gradient(circle at 82% 74%, rgba(125, 195, 224, 0.3), transparent 38%);
  filter: blur(18px);
  opacity: 0.8;
}

.hr-center-flow-svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

.hr-center-flow-base-lines path {
  fill: none;
  stroke: var(--hr-flow-line);
  stroke-linecap: round;
  stroke-width: 0.32;
}

.hr-center-flow-pulse,
.hr-center-flow-pulse-blur {
  fill: none;
  stroke-width: 1.15;
  stroke-linecap: round;
  stroke-dasharray: 13 87;
  stroke-dashoffset: 100;
  animation: hr-center-flow-pulse 4.8s cubic-bezier(.4, 0, .2, 1) var(--hr-pulse-delay) infinite;
}

.hr-center-flow-pulse-blur {
  stroke: var(--hr-flow-pulse);
  stroke-width: 2.6;
  opacity: 0.48;
}

.hr-center-flow-core-glow {
  fill: var(--hr-flow-glow);
  opacity: 0.36;
  transform-origin: 50px 50px;
  animation: hr-center-flow-core-breathe 4.8s ease-in-out infinite;
}

.hr-center-flow-center,
.hr-center-flow-node-surface {
  display: grid;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.72);
  background: rgba(255, 255, 255, 0.42);
  box-shadow: 0 18px 44px rgba(58, 45, 40, 0.1), inset 0 0 0 1px rgba(255, 255, 255, 0.34);
  backdrop-filter: blur(18px) saturate(112%);
}

.hr-center-flow-center {
  position: absolute;
  top: 50%;
  left: 50%;
  z-index: 4;
  width: clamp(116px, 20%, 142px);
  aspect-ratio: 1;
  border-radius: 30px;
  box-shadow: 0 24px 64px rgba(57, 44, 39, 0.15), inset 0 0 0 1px rgba(255, 255, 255, 0.42);
  transform: translate(-50%, -50%);
}

.hr-center-flow-center-ring {
  position: absolute;
  inset: -10px;
  border: 1px solid rgba(255, 255, 255, 0.4);
  border-radius: 36px;
  opacity: 0.66;
}

.hr-center-flow-center img {
  width: 44%;
  height: 44%;
  border-radius: 12%;
  object-fit: contain;
}

.hr-center-flow-node {
  position: absolute;
  top: var(--hr-node-y);
  left: var(--hr-node-x);
  z-index: 3;
  display: grid;
  padding: 0;
  color: inherit;
  background: none;
  border: 0;
  outline: none;
  text-decoration: none;
  transform: translate(-50%, -50%);
  animation: hr-center-flow-node-float 5.6s ease-in-out var(--hr-node-delay) infinite;
}

.hr-center-flow-node-surface {
  position: relative;
  width: clamp(58px, 10vw, 72px);
  height: clamp(58px, 10vw, 72px);
  border-radius: 20px;
  transition: transform 180ms cubic-bezier(.23, 1, .32, 1), background 180ms ease, box-shadow 180ms ease;
}

.hr-center-flow-node img {
  width: 68%;
  height: 68%;
  object-fit: contain;
}

.hr-center-flow-node[data-tool='cursor'] img,
.hr-center-flow-node[data-tool='hermes'] img {
  border-radius: 6px;
}

.hr-center-flow-node-dot {
  position: absolute;
  right: 8px;
  bottom: 8px;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--hr-flow-pulse);
  box-shadow: 0 0 0 3px rgba(60, 159, 164, 0.18), 0 0 14px var(--hr-flow-pulse);
}

.hr-center-flow-node:hover .hr-center-flow-node-surface,
.hr-center-flow-node:focus-visible .hr-center-flow-node-surface {
  background: rgba(255, 255, 255, 0.58);
  box-shadow: 0 22px 44px rgba(58, 45, 40, 0.15), inset 0 0 0 1px rgba(255, 255, 255, 0.58);
  transform: translateY(-4px) scale(1.05);
}

.hr-center-flow-node:focus-visible .hr-center-flow-node-surface {
  outline: 2px solid var(--hr-flow-pulse);
  outline-offset: 4px;
}

:global(.heyroute-preview[data-theme='dark'] .hr-center-flow) {
  --hr-flow-line: rgba(205, 213, 244, 0.25);
  --hr-flow-pulse-soft: #7dd5d8;
  --hr-flow-pulse: #a9b9ff;
  --hr-flow-pulse-bright: #d49cff;
  --hr-flow-glow: rgba(118, 143, 255, 0.52);
}

:global(.heyroute-preview[data-theme='dark'] .hr-center-flow-haze) {
  background:
    radial-gradient(circle at 50% 50%, rgba(117, 133, 247, 0.2), transparent 28%),
    radial-gradient(circle at 18% 22%, rgba(219, 111, 91, 0.2), transparent 34%),
    radial-gradient(circle at 82% 76%, rgba(57, 173, 190, 0.2), transparent 38%);
  opacity: 0.72;
}

:global(.heyroute-preview[data-theme='dark'] .hr-center-flow-center),
:global(.heyroute-preview[data-theme='dark'] .hr-center-flow-node-surface) {
  border-color: rgba(221, 232, 228, 0.16);
  background: rgba(20, 28, 59, 0.5);
  box-shadow: 0 18px 44px rgba(2, 7, 9, 0.32), inset 0 0 0 1px rgba(214, 222, 255, 0.08);
}

@keyframes hr-center-flow-pulse {
  0% { stroke-dashoffset: 100; opacity: 0; }
  8% { opacity: 1; }
  54% { opacity: 1; }
  68%, 100% { stroke-dashoffset: -13; opacity: 0; }
}

@keyframes hr-center-flow-core-breathe {
  0%, 100% { opacity: 0.2; transform: scale(0.9); }
  52% { opacity: 0.62; transform: scale(1.12); }
}

@keyframes hr-center-flow-node-float {
  0%, 100% { transform: translate(-50%, -50%) translateY(0); }
  50% { transform: translate(-50%, -50%) translateY(-3px); }
}

@media (max-width: 1100px) {
  .hr-integrations-shell {
    grid-template-columns: 1fr;
    gap: 42px;
  }

  .hr-integration-copy h2 {
    max-width: 14ch;
  }

  .hr-center-flow {
    width: min(100%, 560px);
    justify-self: center;
  }
}

@media (max-width: 720px) {
  .hr-integrations-shell {
    gap: 30px;
  }

  .hr-integration-copy {
    gap: 18px;
  }

  .hr-integration-copy h2 {
    max-width: 10ch;
    font-size: 2.5rem;
  }

  .hr-integration-copy p {
    font-size: 15px;
    line-height: 1.62;
  }

  .hr-center-flow {
    width: min(100%, 410px);
  }

  .hr-center-flow-center {
    width: 96px;
    border-radius: 25px;
  }

  .hr-center-flow-center-ring {
    inset: -8px;
    border-radius: 30px;
  }

  .hr-center-flow-node-surface {
    width: 54px;
    height: 54px;
    border-radius: 17px;
  }

  .hr-center-flow-node-dot {
    right: 6px;
    bottom: 6px;
    width: 4px;
    height: 4px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hr-center-flow-pulse,
  .hr-center-flow-pulse-blur,
  .hr-center-flow-core-glow,
  .hr-center-flow-node {
    animation: none;
  }

  .hr-center-flow-pulse,
  .hr-center-flow-pulse-blur {
    stroke-dashoffset: 0;
    opacity: 0.6;
  }
}
</style>
