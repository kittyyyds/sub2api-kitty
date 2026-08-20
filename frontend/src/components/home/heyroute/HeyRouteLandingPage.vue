<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef } from 'vue'

import type { HomePricingSummary } from '@/composables/useHomePricingPreview'
import { useHeyRoutePointerEffects } from '@/composables/useHeyRoutePointerEffects'
import { useHeyRouteSmoothScroll } from '@/composables/useHeyRouteSmoothScroll'
import HeyRouteCommunityDialog from './HeyRouteCommunityDialog.vue'
import HeyRouteFaq from './HeyRouteFaq.vue'
import HeyRouteFinalCta from './HeyRouteFinalCta.vue'
import HeyRouteFooter from './HeyRouteFooter.vue'
import HeyRouteGlassCursor from './HeyRouteGlassCursor.vue'
import HeyRouteHeader from './HeyRouteHeader.vue'
import HeyRouteHero from './HeyRouteHero.vue'
import HeyRouteIntegrations from './HeyRouteIntegrations.vue'
import HeyRouteModelWall from './HeyRouteModelWall.vue'
import HeyRoutePricing from './HeyRoutePricing.vue'
import HeyRouteScenarios from './HeyRouteScenarios.vue'
import HeyRouteSecurity from './HeyRouteSecurity.vue'
import HeyRouteStats from './HeyRouteStats.vue'
import HeyRouteSteps from './HeyRouteSteps.vue'
import HeyRouteSupportDialog from './HeyRouteSupportDialog.vue'
import HeyRouteSupportWidget from './HeyRouteSupportWidget.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy
  siteName: string
  siteLogo: string
  docUrl: string
  apiBaseUrl: string
  isDark: boolean
  isAuthenticated: boolean
  dashboardPath: string
  modelPlazaEnabled: boolean
  pricingSummaries: HomePricingSummary[]
  pricingLoading: boolean
  pricingFailed: boolean
  groupCount: number
  modelCount: number
}

const props = defineProps<Props>()

const emit = defineEmits<{
  toggleTheme: []
  retryPricing: []
}>()

const rootRef = shallowRef<HTMLElement | null>(null)
const activeSection = shallowRef('hr-hero')
const communityDialogOpen = shallowRef(false)
const supportDialogOpen = shallowRef(false)
const isHeroActive = computed(() => activeSection.value === 'hr-hero')
const dashboardTarget = computed(() => props.isAuthenticated ? props.dashboardPath : '/login')
const ratios = new Map<string, number>()
let observer: IntersectionObserver | null = null
let visibilityObserver: IntersectionObserver | null = null
let desktopQuery: MediaQueryList | null = null

useHeyRoutePointerEffects(rootRef)
useHeyRouteSmoothScroll(rootRef)

function updateActiveSection(): void {
  let bestId = activeSection.value
  let bestRatio = 0
  ratios.forEach((ratio, id) => {
    if (ratio > bestRatio) {
      bestRatio = ratio
      bestId = id
    }
  })
  if (bestRatio > 0.08) activeSection.value = bestId
}

function setupSectionObserver(): void {
  observer?.disconnect()
  ratios.clear()
  const root = rootRef.value
  if (!root) return

  observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => ratios.set((entry.target as HTMLElement).id, entry.intersectionRatio))
      updateActiveSection()
    },
    {
      root: desktopQuery?.matches ? root : null,
      rootMargin: '-15% 0px -38% 0px',
      threshold: [0, 0.08, 0.2, 0.4, 0.6, 0.8],
    },
  )

  root.querySelectorAll<HTMLElement>('[data-hr-section]').forEach((section) => {
    ratios.set(section.id, 0)
    observer?.observe(section)
  })
}

function setupVisibilityObserver(): void {
  visibilityObserver?.disconnect()
  const root = rootRef.value
  if (!root) return

  visibilityObserver = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        const section = entry.target as HTMLElement
        section.dataset.hrVisible = entry.isIntersecting ? 'true' : 'false'
      })
    },
    {
      root: desktopQuery?.matches ? root : null,
      rootMargin: '20% 0px',
      threshold: 0.01,
    },
  )

  root.querySelectorAll<HTMLElement>('[data-hr-section]').forEach((section) => {
    visibilityObserver?.observe(section)
  })
}

function navigate(sectionId: string): void {
  const section = rootRef.value?.querySelector<HTMLElement>(`#${sectionId}`)
  if (!section) return
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  section.scrollIntoView({ behavior: reduceMotion ? 'auto' : 'smooth', block: 'start' })
}

onMounted(() => {
  desktopQuery = window.matchMedia('(min-width: 901px)')
  desktopQuery.addEventListener('change', setupSectionObserver)
  desktopQuery.addEventListener('change', setupVisibilityObserver)
  setupSectionObserver()
  setupVisibilityObserver()
})

onBeforeUnmount(() => {
  observer?.disconnect()
  visibilityObserver?.disconnect()
  desktopQuery?.removeEventListener('change', setupSectionObserver)
  desktopQuery?.removeEventListener('change', setupVisibilityObserver)
})
</script>

<template>
  <div
    ref="rootRef"
    class="heyroute-preview"
    :data-theme="isDark ? 'dark' : 'light'"
  >
    <a class="hr-skip-link" href="#hr-main">Skip to content</a>
    <HeyRouteHeader
      :copy="copy.nav"
      :site-name="siteName"
      :site-logo="siteLogo"
      :doc-url="docUrl"
      :active-section="activeSection"
      :is-dark="isDark"
      :is-authenticated="isAuthenticated"
      :dashboard-path="dashboardPath"
      :model-plaza-enabled="modelPlazaEnabled"
      :is-hero-active="isHeroActive"
      @navigate="navigate"
      @toggle-theme="emit('toggleTheme')"
    />

    <main id="hr-main">
      <HeyRouteHero :copy="copy.hero" :site-name="siteName" :site-logo="siteLogo" :is-dark="isDark" />
      <HeyRouteModelWall :copy="copy.models" />
      <HeyRouteStats :copy="copy.stats" />
      <HeyRoutePricing
        :copy="copy.pricing"
        :summaries="pricingSummaries"
        :loading="pricingLoading"
        :failed="pricingFailed"
        :model-plaza-enabled="modelPlazaEnabled"
        @retry="emit('retryPricing')"
      />
      <HeyRouteIntegrations :copy="copy.integrations" :site-logo="siteLogo" :site-name="siteName" />
      <HeyRouteSteps :copy="copy.steps" :api-base-url="apiBaseUrl" />
      <HeyRouteSecurity :copy="copy.security" :dashboard-path="dashboardTarget" />
      <HeyRouteScenarios :copy="copy.scenarios" />
      <HeyRouteFaq :copy="copy.faq" :doc-url="docUrl" />
      <HeyRouteFinalCta :copy="copy.cta" :dashboard-path="dashboardTarget" />
    </main>

    <HeyRouteFooter
      :copy="copy.footer"
      :site-name="siteName"
      :site-logo="siteLogo"
      :doc-url="docUrl"
      :model-plaza-enabled="modelPlazaEnabled"
      :dashboard-path="dashboardTarget"
      @open-community="communityDialogOpen = true; supportDialogOpen = false"
      @open-support="supportDialogOpen = true; communityDialogOpen = false"
    />
    <HeyRouteCommunityDialog
      :open="communityDialogOpen"
      :copy="copy.footer"
      @close="communityDialogOpen = false"
    />
    <HeyRouteSupportDialog
      :open="supportDialogOpen"
      :copy="copy.support"
      @close="supportDialogOpen = false"
    />
    <HeyRouteSupportWidget :copy="copy.support" :doc-url="docUrl" />
    <HeyRouteGlassCursor />
  </div>
</template>

<style scoped>
.heyroute-preview {
  --hr-bg: #f2f1ec;
  --hr-panel: #f7f7f3;
  --hr-panel-soft: rgba(247, 247, 243, 0.76);
  --hr-panel-translucent: rgba(244, 246, 242, 0.62);
  --hr-ink: #171b1a;
  --hr-muted: #606662;
  --hr-line: rgba(39, 46, 43, 0.11);
  --hr-line-strong: rgba(39, 46, 43, 0.24);
  --hr-glass-border: rgba(246, 248, 244, 0.58);
  --hr-hover: rgba(35, 43, 40, 0.07);
  --hr-nav-bg: rgba(241, 242, 235, 0.72);
  --hr-shadow: rgba(71, 58, 49, 0.2);
  --hr-shadow-soft: rgba(71, 58, 49, 0.11);
  --hr-focus: #176f69;
  --hr-hero-ink: #f5f7f3;
  --hr-footer-bg: #e8e4da;
  position: relative;
  min-height: 100dvh;
  overflow-x: clip;
  color: var(--hr-ink);
  background: var(--hr-bg);
  font-family: 'Montserrat Variable', Montserrat, 'Noto Sans SC', 'Microsoft YaHei', 'PingFang SC', system-ui, sans-serif;
  font-size: 16px;
  -webkit-font-smoothing: antialiased;
}

.heyroute-preview[data-theme='dark'] {
  --hr-bg: #111827;
  --hr-panel: #202b3b;
  --hr-panel-soft: rgba(32, 43, 59, 0.9);
  --hr-panel-translucent: rgba(29, 40, 55, 0.76);
  --hr-ink: #f7faf8;
  --hr-muted: #bec9c4;
  --hr-line: rgba(235, 242, 238, 0.15);
  --hr-line-strong: rgba(235, 242, 238, 0.31);
  --hr-glass-border: rgba(228, 239, 234, 0.23);
  --hr-hover: rgba(241, 247, 244, 0.12);
  --hr-nav-bg: rgba(31, 42, 57, 0.82);
  --hr-shadow: rgba(0, 6, 12, 0.3);
  --hr-shadow-soft: rgba(0, 6, 12, 0.18);
  --hr-focus: #86e0d6;
  --hr-hero-ink: #ffffff;
  --hr-footer-bg: #0d1420;
}

.hr-skip-link {
  position: fixed;
  top: 12px;
  left: 12px;
  z-index: 120;
  min-height: 44px;
  padding: 12px 16px;
  color: #f3f5f2;
  background: #1c211f;
  transform: translateY(-180%);
}

.hr-skip-link:focus {
  transform: translateY(0);
}

@media (min-width: 901px) {
  .heyroute-preview {
    height: 100dvh;
    overflow-y: auto;
    overscroll-behavior-y: contain;
    scrollbar-color: var(--hr-line-strong) transparent;
    scrollbar-width: thin;
  }
}
</style>

<style>
.heyroute-preview *,
.heyroute-preview *::before,
.heyroute-preview *::after {
  box-sizing: border-box;
}

.heyroute-preview .hr-stage {
  position: relative;
  display: flex;
  min-height: 100dvh;
  align-items: center;
}

.heyroute-preview .hr-content-stage {
  padding: 88px 0;
}

.heyroute-preview .hr-section-shell {
  width: min(calc(100% - 80px), 1680px);
  margin: 0 auto;
}

.heyroute-preview button,
.heyroute-preview select,
.heyroute-preview a {
  letter-spacing: 0;
}

.heyroute-preview [data-hr-tilt] {
  --hr-card-x: 50%;
  --hr-card-y: 50%;
  --hr-card-rotate-x: 0deg;
  --hr-card-rotate-y: 0deg;
  --hr-card-glare: 0;
  transform: perspective(1200px) rotateX(var(--hr-card-rotate-x)) rotateY(var(--hr-card-rotate-y));
  transform-origin: center;
  transform-style: preserve-3d;
  transition: transform 260ms cubic-bezier(.16, 1, .3, 1), box-shadow 260ms ease, border-color 220ms ease;
}

.heyroute-preview [data-hr-tilt]::after {
  position: absolute;
  inset: 0;
  z-index: 8;
  border-radius: inherit;
  background: radial-gradient(260px circle at var(--hr-card-x) var(--hr-card-y), rgba(255, 255, 255, 0.42), transparent 68%);
  content: '';
  opacity: var(--hr-card-glare);
  pointer-events: none;
  transition: opacity 220ms ease;
}

.heyroute-preview [data-hr-tilt].is-pointer-active {
  transform: perspective(1200px) rotateX(var(--hr-card-rotate-x)) rotateY(var(--hr-card-rotate-y)) translateY(-5px) scale(1.008);
}

.heyroute-preview [data-hr-section][data-hr-visible='false'],
.heyroute-preview [data-hr-section][data-hr-visible='false'] *,
.heyroute-preview [data-hr-section][data-hr-visible='false'] *::before,
.heyroute-preview [data-hr-section][data-hr-visible='false'] *::after {
  animation-play-state: paused !important;
}

@media (hover: hover) and (pointer: fine) {
  .heyroute-preview [data-hr-tilt]:hover,
  .heyroute-preview [data-hr-tilt].is-pointer-active {
    will-change: transform;
  }
}

.heyroute-preview button:not(:disabled),
.heyroute-preview a {
  transition-property: color, background-color, border-color, box-shadow, opacity, transform;
  transition-duration: 180ms;
  transition-timing-function: cubic-bezier(.16, 1, .3, 1);
}

.heyroute-preview a:active,
.heyroute-preview button:not(:disabled):active {
  transform: scale(0.975);
}

.heyroute-preview button,
.heyroute-preview select {
  font-family: inherit;
}

.heyroute-preview .sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
}

@media (max-width: 900px) {
  .heyroute-preview .hr-stage {
    min-height: auto;
    scroll-snap-align: none;
  }

  .heyroute-preview .hr-content-stage {
    padding: 82px 0;
  }

  .heyroute-preview .hr-section-shell {
    width: min(calc(100% - 40px), 680px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .heyroute-preview *,
  .heyroute-preview *::before,
  .heyroute-preview *::after {
    scroll-behavior: auto !important;
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }

  .heyroute-preview [data-hr-tilt],
  .heyroute-preview [data-hr-tilt].is-pointer-active {
    transform: none !important;
  }
}
</style>
