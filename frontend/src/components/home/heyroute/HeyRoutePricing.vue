<script setup lang="ts">
import { computed } from 'vue'

import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import type { HomePricingSummary } from '@/composables/useHomePricingPreview'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['pricing']
  summaries: HomePricingSummary[]
  loading: boolean
  failed: boolean
  modelPlazaEnabled: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  retry: []
}>()

const rows = computed(() => {
  const openAI = props.summaries.find((item) => item.platform === 'openai')
  const anthropic = props.summaries.find((item) => item.platform === 'anthropic')
  const grok = props.summaries.find((item) => item.platform === 'grok')
  const defaults = [
    { summary: openAI, rateMultiplier: 0.08, groupName: 'Codex - 福利池 - 【尽量不断】', modelCount: 4 },
    { summary: null, rateMultiplier: 0.7, groupName: 'CC-Max - 满血- 【首选】', modelCount: 16 },
    { summary: grok, rateMultiplier: 0.15, groupName: 'Grok - 正品会员 号池', modelCount: 11 },
    { summary: anthropic, rateMultiplier: 0.1, groupName: 'Claude-Kiro-90缓', modelCount: 16 },
  ]

  return props.copy.fallbacks.map((fallback, index) => {
    const row = defaults[index]
    const summary = row?.summary
    return {
      ...fallback,
      featuredModel: fallback.model,
      rateMultiplier: summary?.rateMultiplier ?? row?.rateMultiplier ?? null,
      groupName: summary?.groupName || row?.groupName || props.copy.unavailable,
      modelCount: summary?.modelCount ?? row?.modelCount ?? null,
    }
  })
})

function formatRate(value: number | null): string {
  if (value === null) return '...'
  return `x${new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 }).format(value)}`
}
</script>

<template>
  <section id="hr-pricing" class="hr-pricing hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-pricing-shell">
      <header>
        <h2>{{ copy.title }}</h2>
        <div class="hr-pricing-action">
          <p>{{ copy.prompt }}</p>
          <router-link :to="modelPlazaEnabled ? '/model-plaza' : '/login'">
            {{ copy.action }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </header>

      <div class="hr-pricing-table" :aria-busy="loading">
        <div class="hr-pricing-row hr-pricing-head" role="row">
          <span>{{ copy.model }}</span>
          <span>{{ copy.kittyRate }}<small>{{ copy.inputOutput }}</small></span>
          <span>{{ copy.group }}</span>
          <span>{{ copy.interface }}</span>
        </div>

        <div v-for="row in rows" :key="row.model" class="hr-pricing-row" role="row">
          <div class="hr-price-model">
            <span class="hr-model-icon"><ModelIcon :model="row.iconModel" size="24px" /></span>
            <span><strong>{{ row.featuredModel }}</strong><small>{{ row.provider }}</small></span>
          </div>
          <div class="hr-rate-value" :class="{ loading }">{{ formatRate(row.rateMultiplier) }}</div>
          <div class="hr-group-value">
            <strong>{{ row.groupName }}</strong>
            <small v-if="row.modelCount !== null">{{ row.modelCount }} models</small>
          </div>
          <div class="hr-interface-value">
            <span class="hr-live-dot" />
            <span>{{ row.rateMultiplier === null ? copy.unavailable : copy.live }}</span>
          </div>
        </div>
      </div>

      <footer>
        <p>{{ copy.note }}</p>
        <button v-if="failed" type="button" @click="emit('retry')">
          <Icon name="refresh" size="sm" />
          {{ copy.retry }}
        </button>
        <span v-else-if="loading">{{ copy.loading }}</span>
      </footer>
    </div>
  </section>
</template>

<style scoped>
.hr-pricing {
  background:
    radial-gradient(circle at 10% 20%, rgba(245, 154, 101, 0.16), transparent 28%),
    radial-gradient(circle at 90% 16%, rgba(147, 165, 218, 0.18), transparent 31%),
    radial-gradient(circle at 58% 92%, rgba(116, 176, 143, 0.15), transparent 32%),
    var(--hr-bg);
}

.hr-pricing-shell {
  display: grid;
  gap: 30px;
}

.hr-pricing header {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(260px, 0.65fr);
  gap: 50px;
  align-items: end;
}

.hr-pricing h2 {
  max-width: 10em;
  margin: 0;
  font-size: 5.2rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.94;
  text-wrap: balance;
}

.hr-pricing-action {
  display: grid;
  justify-items: start;
  gap: 20px;
  padding-bottom: 8px;
}

.hr-pricing-action p {
  margin: 0;
  color: var(--hr-muted);
  font-size: 16px;
}

.hr-pricing-action a {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid var(--hr-ink);
  color: var(--hr-ink);
  font-size: 14px;
  font-weight: 680;
  text-decoration: none;
}

.hr-pricing-table {
  overflow: hidden;
  border: 1px solid var(--hr-line);
  border-radius: 8px;
  background: var(--hr-panel-soft);
  box-shadow: 0 26px 54px var(--hr-shadow-soft);
}

.hr-pricing-row {
  display: grid;
  min-height: 90px;
  grid-template-columns: 1.5fr 0.95fr 0.95fr 0.75fr;
  border-bottom: 1px solid var(--hr-line);
}

.hr-pricing-row:not(.hr-pricing-head) {
  transition: background-color 180ms ease, transform 220ms cubic-bezier(.16, 1, .3, 1);
}

.hr-pricing-row:not(.hr-pricing-head):hover {
  z-index: 1;
  background: rgba(255, 255, 255, 0.34);
  transform: translateX(4px);
}

.hr-pricing-row:last-child {
  border-bottom: 0;
}

.hr-pricing-row > * {
  display: flex;
  min-width: 0;
  align-items: center;
  border-right: 1px solid var(--hr-line);
  padding: 18px 26px;
}

.hr-pricing-row > *:last-child {
  border-right: 0;
}

.hr-pricing-head {
  min-height: 74px;
}

.hr-pricing-head > span {
  display: grid;
  align-content: center;
  color: var(--hr-ink);
  font-size: 12px;
  font-weight: 680;
}

.hr-pricing-head small,
.hr-price-model small,
.hr-group-value small {
  display: block;
  margin-top: 3px;
  color: var(--hr-muted);
  font-size: 10px;
  font-weight: 540;
}

.hr-price-model {
  gap: 14px;
}

.hr-model-icon {
  display: grid;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid var(--hr-glass-border);
  border-radius: 8px;
  background: var(--hr-panel);
}

.hr-price-model strong,
.hr-group-value strong {
  display: block;
  overflow: hidden;
  font-size: 14px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hr-rate-value {
  font-size: 18px;
  font-weight: 760;
  font-variant-numeric: tabular-nums;
}

.hr-rate-value.loading {
  animation: hr-rate-pulse 1.1s ease-in-out infinite alternate;
}

.hr-group-value {
  display: grid;
  align-content: center;
}

.hr-interface-value {
  gap: 9px;
  color: var(--hr-muted);
  font-size: 12px;
  font-weight: 620;
}

.hr-live-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: #168c75;
  box-shadow: 0 0 0 4px rgba(22, 140, 117, 0.12);
}

.hr-pricing footer {
  display: flex;
  min-height: 30px;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  color: var(--hr-muted);
  font-size: 11px;
}

.hr-pricing footer p {
  margin: 0;
}

.hr-pricing footer button {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  gap: 8px;
  border: 0;
  color: var(--hr-ink);
  background: transparent;
  font-weight: 650;
  cursor: pointer;
}

.hr-pricing-action a:focus-visible,
.hr-pricing footer button:focus-visible {
  outline: 2px solid var(--hr-focus);
  outline-offset: 3px;
}

@keyframes hr-rate-pulse {
  from { opacity: 0.38; }
  to { opacity: 0.92; }
}

@media (max-width: 900px) {
  .hr-pricing header {
    grid-template-columns: 1fr;
    gap: 28px;
  }

  .hr-pricing h2 {
    max-width: 11ch;
    font-size: 2.5rem;
  }

  .hr-pricing-row {
    min-height: 0;
    grid-template-columns: 1fr 1fr;
  }

  .hr-pricing-head {
    display: none;
  }

  .hr-pricing-row > * {
    min-height: 78px;
    border-right: 0;
    border-bottom: 1px solid var(--hr-line);
    padding: 14px 16px;
  }

  .hr-group-value strong {
    overflow: visible;
    font-size: 12px;
    line-height: 1.3;
    overflow-wrap: anywhere;
    text-overflow: clip;
    white-space: normal;
  }

  .hr-pricing-row > *:nth-child(odd) {
    border-right: 1px solid var(--hr-line);
  }

  .hr-pricing-row > *:nth-last-child(-n + 2) {
    border-bottom: 0;
  }

  .hr-pricing footer {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hr-rate-value.loading {
    animation: none;
  }
}
</style>
