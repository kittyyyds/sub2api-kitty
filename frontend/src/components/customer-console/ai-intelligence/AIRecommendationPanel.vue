<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { AIIntelligencePoint } from '@/api/aiIntelligence'
import Icon from '@/components/icons/Icon.vue'
import { passRate } from '@/features/customer-console/aiIntelligence'
import AIEffortBadge from './AIEffortBadge.vue'

const props = defineProps<{
  recommended: AIIntelligencePoint
  smartest: AIIntelligencePoint
  bestValue: AIIntelligencePoint
  fastest: AIIntelligencePoint
}>()

const { t, locale } = useI18n()

interface RecommendationMetric {
  label: string
  value: string
  note?: string
}

const numberFormatter = computed(() => new Intl.NumberFormat(
  locale.value === 'zh' ? 'zh-CN' : 'en-US',
  { maximumFractionDigits: 2 },
))

const metrics = computed<RecommendationMetric[]>(() => [
  { label: 'IQ', value: numberFormatter.value.format(props.recommended.iq) },
  { label: t('native.radar.metric.duration'), value: t('native.radar.units.minutes', { value: numberFormatter.value.format(props.recommended.average_minutes) }) },
  { label: t('native.radar.metric.cost'), value: `$${props.recommended.average_price_usd.toFixed(2)}` },
  {
    label: t('native.radar.metric.pass'),
    value: `${Math.round(passRate(props.recommended))}%`,
    note: `${Math.round(props.recommended.passed)}/${Math.round(props.recommended.valid_tasks)}`,
  },
])

const choices = computed(() => [
  { key: 'smartest', icon: 'brain' as const, point: props.smartest, metric: props.smartest.iq.toFixed(2) },
  { key: 'bestValue', icon: 'dollar' as const, point: props.bestValue, metric: props.bestValue.recommendation_score.toFixed(2) },
  { key: 'fastest', icon: 'bolt' as const, point: props.fastest, metric: t('native.radar.units.minutes', { value: numberFormatter.value.format(props.fastest.average_minutes) }) },
])
</script>

<template>
  <section class="recommendation-panel">
    <header class="panel-heading">
      <h2>{{ t('native.radar.today.title') }}</h2>
      <span>{{ t('native.radar.today.formula') }}</span>
    </header>

    <div class="recommendation-body">
      <div class="primary-choice">
        <div class="choice-title">
          <span class="star-mark" aria-hidden="true">
            <Icon name="sparkles" size="sm" :stroke-width="1.8" />
          </span>
          <strong>{{ recommended.model }}</strong>
          <AIEffortBadge :effort="recommended.effort" />
          <span class="recommendation-score">
            <b>{{ recommended.recommendation_score.toFixed(1) }}</b>
            {{ t('native.radar.today.score') }}
          </span>
        </div>

        <div class="metric-grid">
          <div v-for="metric in metrics" :key="metric.label">
            <span>{{ metric.label }}</span>
            <strong>{{ metric.value }}</strong>
            <small v-if="metric.note">{{ metric.note }}</small>
          </div>
        </div>
      </div>

      <div class="choice-list">
        <div v-for="choice in choices" :key="choice.key" class="choice-row">
          <span class="choice-icon" :data-choice="choice.key" aria-hidden="true">
            <Icon :name="choice.icon" size="sm" :stroke-width="1.8" />
          </span>
          <strong>{{ t(`native.radar.choice.${choice.key}`) }}</strong>
          <span class="choice-model">{{ choice.point.model }}</span>
          <AIEffortBadge :effort="choice.point.effort" />
          <b>{{ choice.metric }}</b>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.recommendation-panel {
  min-width: 0;
  border: 1px solid var(--ai-line);
  border-radius: 8px;
  background: var(--ai-surface);
  box-shadow: var(--ai-shadow);
}

.panel-heading,
.choice-title,
.choice-row {
  display: flex;
  align-items: center;
}

.panel-heading {
  justify-content: space-between;
  gap: 16px;
  min-height: 46px;
  padding: 0 16px;
  border-bottom: 1px solid var(--ai-line);
}

.panel-heading h2 {
  margin: 0;
  color: var(--ai-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0;
}

.panel-heading span {
  color: var(--ai-faint);
  font-size: 9px;
}

.recommendation-body {
  display: grid;
  grid-template-columns: minmax(360px, 1.05fr) minmax(360px, 1fr);
  min-width: 0;
}

.primary-choice {
  min-width: 0;
  padding: 16px;
  border-right: 1px solid var(--ai-line);
}

.choice-title {
  min-width: 0;
  gap: 9px;
}

.choice-title > strong {
  overflow: hidden;
  color: var(--ai-ink);
  font-size: 15px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.star-mark,
.choice-icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 6px;
}

.star-mark {
  width: 32px;
  height: 32px;
  color: var(--ai-copper);
  background: var(--ai-copper-soft);
}

.recommendation-score {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  margin-left: auto;
  color: var(--ai-faint);
  font-size: 9px;
  white-space: nowrap;
}

.recommendation-score b {
  color: var(--ai-copper);
  font-family: var(--ai-mono);
  font-size: 22px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-top: 16px;
  border-top: 1px solid var(--ai-line);
}

.metric-grid > div {
  display: grid;
  min-width: 0;
  gap: 4px;
  padding: 14px 12px 2px;
  border-left: 1px solid var(--ai-line);
}

.metric-grid > div:first-child {
  padding-left: 0;
  border-left: 0;
}

.metric-grid span,
.metric-grid small {
  color: var(--ai-faint);
  font-size: 9px;
}

.metric-grid strong {
  color: var(--ai-ink);
  font-family: var(--ai-mono);
  font-size: 12px;
}

.choice-list {
  display: grid;
  min-width: 0;
  align-content: center;
  padding: 8px 16px;
}

.choice-row {
  display: grid;
  grid-template-columns: 30px minmax(64px, 0.7fr) minmax(90px, 1fr) auto minmax(48px, auto);
  gap: 8px;
  min-width: 0;
  min-height: 48px;
  border-bottom: 1px solid var(--ai-line);
}

.choice-row:last-child {
  border-bottom: 0;
}

.choice-icon {
  width: 26px;
  height: 26px;
  color: var(--ai-blue);
  background: var(--ai-blue-soft);
}

.choice-icon[data-choice="bestValue"] {
  color: var(--ai-success);
  background: var(--ai-success-soft);
}

.choice-icon[data-choice="fastest"] {
  color: var(--ai-copper);
  background: var(--ai-copper-soft);
}

.choice-row > strong {
  color: var(--ai-ink);
  font-size: 10px;
  font-weight: 680;
}

.choice-model {
  overflow: hidden;
  color: var(--ai-muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.choice-row > b {
  color: var(--ai-ink);
  font-family: var(--ai-mono);
  font-size: 10px;
  text-align: right;
  white-space: nowrap;
}

@media (max-width: 1120px) {
  .recommendation-body {
    grid-template-columns: 1fr;
  }

  .primary-choice {
    border-right: 0;
    border-bottom: 1px solid var(--ai-line);
  }
}

@media (max-width: 560px) {
  .panel-heading span {
    display: none;
  }

  .choice-title {
    flex-wrap: wrap;
  }

  .recommendation-score {
    width: 100%;
    margin-left: 41px;
  }

  .metric-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .metric-grid > div:nth-child(3) {
    padding-left: 0;
    border-left: 0;
  }

  .choice-row {
    grid-template-columns: 28px minmax(62px, 0.8fr) minmax(90px, 1fr) auto;
  }

  .choice-row > b {
    display: none;
  }
}
</style>
