<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, shallowRef } from 'vue'

import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['stats']
}

const props = defineProps<Props>()

const statDefinitions = [
  { value: 99.998, decimals: 3, suffix: '%', tone: 'mint' },
  { value: 984, decimals: 0, suffix: 'ms', tone: 'lilac' },
  { value: 120, decimals: 0, suffix: 'B+', tone: 'coral' },
  { value: 98.4, decimals: 1, suffix: '%', tone: 'sky' },
]

const sectionRef = shallowRef<HTMLElement | null>(null)
const displayedValues = reactive(statDefinitions.map((stat) => (0).toFixed(stat.decimals)))
const stats = computed(() => statDefinitions.map((item, index) => ({
  ...item,
  ...props.copy.items[index],
})))

let observer: IntersectionObserver | null = null
let frameId = 0
let hasAnimated = false

function setFinalValues(): void {
  statDefinitions.forEach((stat, index) => {
    displayedValues[index] = stat.value.toFixed(stat.decimals)
  })
}

function startCountUp(): void {
  if (hasAnimated) return
  hasAnimated = true

  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    setFinalValues()
    return
  }

  const startTime = performance.now()
  const duration = 1320

  const draw = (now: number): void => {
    const progress = Math.min(Math.max((now - startTime) / duration, 0), 1)
    const eased = 1 - Math.pow(1 - progress, 4)

    statDefinitions.forEach((stat, index) => {
      displayedValues[index] = (stat.value * eased).toFixed(stat.decimals)
    })

    if (progress < 1) {
      frameId = window.requestAnimationFrame(draw)
    } else {
      setFinalValues()
      frameId = 0
    }
  }

  frameId = window.requestAnimationFrame(draw)
}

onMounted(() => {
  if (!sectionRef.value || typeof IntersectionObserver === 'undefined') {
    setFinalValues()
    return
  }

  observer = new IntersectionObserver(
    ([entry]) => {
      if (!entry?.isIntersecting) return
      startCountUp()
      observer?.disconnect()
      observer = null
    },
    { threshold: 0.32 },
  )
  observer.observe(sectionRef.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  window.cancelAnimationFrame(frameId)
})
</script>

<template>
  <section ref="sectionRef" id="hr-performance" class="hr-stats hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-stats-shell">
      <header>
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
      </header>

      <div class="hr-stat-grid">
        <article
          v-for="(stat, index) in stats"
          :key="stat.label"
          class="hr-stat-card"
          :class="[`tone-${stat.tone}`, { 'is-uptime': index === 0 }]"
        >
          <div class="hr-stat-value">
            <span class="sr-only">{{ `${stat.value.toFixed(stat.decimals)}${stat.suffix}` }}</span>
            <strong aria-hidden="true">{{ displayedValues[index] }}</strong><span aria-hidden="true">{{ stat.suffix }}</span>
          </div>
          <div>
            <h3>{{ stat.label }}</h3>
            <p>{{ stat.description }}</p>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-stats {
  background:
    radial-gradient(circle at 5% 22%, rgba(244, 121, 91, 0.22), transparent 30%),
    radial-gradient(circle at 88% 18%, rgba(152, 126, 218, 0.2), transparent 29%),
    radial-gradient(circle at 64% 92%, rgba(74, 171, 157, 0.17), transparent 31%),
    var(--hr-bg);
}

.hr-stats-shell {
  display: grid;
  gap: 48px;
}

.hr-stats header {
  display: grid;
  max-width: 900px;
  justify-items: center;
  gap: 18px;
  margin: 0 auto;
  text-align: center;
}

.hr-stats h2 {
  max-width: 12ch;
  margin: 0;
  font-size: 5rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.96;
  text-wrap: balance;
}

.hr-stats header > p {
  max-width: 640px;
  margin: 0;
  color: var(--hr-muted);
  font-size: 16px;
  line-height: 1.65;
}

.hr-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.hr-stat-card {
  position: relative;
  container-type: inline-size;
  display: flex;
  min-height: 360px;
  flex-direction: column;
  justify-content: space-between;
  overflow: hidden;
  border: 1px solid rgba(246, 248, 244, 0.68);
  border-radius: 26px;
  padding: 28px 26px 24px;
  box-shadow: 0 24px 46px var(--hr-shadow-soft), inset 0 0 0 1px rgba(246, 248, 244, 0.28);
}

.tone-mint { background: linear-gradient(145deg, rgba(224, 239, 219, 0.9), rgba(245, 241, 234, 0.78)); }
.tone-lilac { background: linear-gradient(145deg, rgba(226, 213, 246, 0.9), rgba(239, 241, 232, 0.78)); }
.tone-coral { background: linear-gradient(145deg, rgba(255, 217, 193, 0.92), rgba(241, 238, 223, 0.78)); }
.tone-sky { background: linear-gradient(145deg, rgba(205, 230, 242, 0.92), rgba(240, 237, 226, 0.78)); }

.hr-stat-value {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  color: #171b1a;
  white-space: nowrap;
}

/*
 * Numbers must never be clipped by the card's overflow: hidden. The min() caps each size by the
 * card's content width (100cqi): Montserrat 680 tabular digits are 0.7em, so "98.4" is 2.36em and
 * "99.998" 3.76em; the suffix ("ms" / "%" at 25px plus 7px padding) needs at most 48px / 30px.
 * Browsers without container units ignore the min() line and keep the rem size.
 */
.hr-stat-value strong {
  display: inline-block;
  font-size: 5.35rem;
  font-size: min(5.35rem, calc((100cqi - 48px) / 2.36));
  font-variant-numeric: tabular-nums lining-nums;
  font-weight: 680;
  letter-spacing: 0;
  line-height: 0.88;
}

.hr-stat-card.is-uptime .hr-stat-value strong {
  font-size: 4.35rem;
  font-size: min(4.35rem, calc((100cqi - 30px) / 3.76));
}

.hr-stat-value span:not(.sr-only) {
  margin-left: auto;
  padding-left: 7px;
  color: rgba(23, 27, 26, 0.56);
  font-size: 25px;
  font-weight: 600;
}

.hr-stat-card h3 {
  margin: 0 0 10px;
  color: #171b1a;
  font-size: 20px;
  font-weight: 720;
  letter-spacing: 0;
}

.hr-stat-card p {
  max-width: 270px;
  margin: 0;
  color: rgba(23, 27, 26, 0.62);
  font-size: 13px;
  line-height: 1.6;
}

:global(.heyroute-preview[data-theme='dark'] .hr-stat-card) {
  border-color: rgba(225, 230, 255, 0.16);
}

:global(.heyroute-preview[data-theme='dark'] .tone-mint) { background: linear-gradient(155deg, rgba(61, 102, 86, 0.72), rgba(24, 31, 46, 0.82)); }
:global(.heyroute-preview[data-theme='dark'] .tone-lilac) { background: linear-gradient(155deg, rgba(88, 73, 147, 0.7), rgba(24, 31, 46, 0.82)); }
:global(.heyroute-preview[data-theme='dark'] .tone-coral) { background: linear-gradient(155deg, rgba(136, 76, 57, 0.72), rgba(24, 31, 46, 0.82)); }
:global(.heyroute-preview[data-theme='dark'] .tone-sky) { background: linear-gradient(155deg, rgba(45, 91, 125, 0.72), rgba(24, 31, 46, 0.82)); }

:global(.heyroute-preview[data-theme='dark'] .hr-stat-value),
:global(.heyroute-preview[data-theme='dark'] .hr-stat-card h3) {
  color: #f4f5fb;
}

:global(.heyroute-preview[data-theme='dark'] .hr-stat-value span),
:global(.heyroute-preview[data-theme='dark'] .hr-stat-card p) {
  color: rgba(231, 234, 250, 0.62);
}

@media (max-width: 1399px) {
  .hr-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .hr-stats header {
    justify-items: start;
    margin: 0;
    text-align: left;
  }

  .hr-stats h2 {
    max-width: 10ch;
    font-size: 2.5rem;
  }

  .hr-stat-grid {
    grid-template-columns: 1fr;
  }

  .hr-stat-card {
    min-height: 300px;
    border-radius: 22px;
    padding: 26px 22px 22px;
  }

  .hr-stat-value strong {
    font-size: 4.8rem;
    font-size: min(4.8rem, calc((100cqi - 48px) / 2.36));
  }

  .hr-stat-card.is-uptime .hr-stat-value strong {
    font-size: 4.25rem;
    font-size: min(4.25rem, calc((100cqi - 30px) / 3.76));
  }
}
</style>
