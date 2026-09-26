<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { AIIntelligencePoint } from '@/api/aiIntelligence'
import { rankingProgress } from '@/features/customer-console/aiIntelligence'
import AIEffortBadge from './AIEffortBadge.vue'

const props = defineProps<{
  kind: 'intelligence' | 'speed'
  items: readonly AIIntelligencePoint[]
}>()

const { t, locale } = useI18n()

const values = computed(() => props.items.map((item) => (
  props.kind === 'intelligence' ? item.iq : item.average_minutes
)))
const numberFormatter = computed(() => new Intl.NumberFormat(
  locale.value === 'zh' ? 'zh-CN' : 'en-US',
  { maximumFractionDigits: 2 },
))

function metric(point: AIIntelligencePoint): string {
  if (props.kind === 'intelligence') return numberFormatter.value.format(point.iq)
  return t('native.radar.units.minutes', { value: numberFormatter.value.format(point.average_minutes) })
}

function progress(point: AIIntelligencePoint): string {
  const value = props.kind === 'intelligence' ? point.iq : point.average_minutes
  return `${rankingProgress(value, values.value, props.kind === 'speed')}%`
}
</script>

<template>
  <section class="ranking-list" :data-kind="kind">
    <header>
      <h2>{{ t(`native.radar.ranking.${kind}.title`) }}</h2>
      <span>{{ t(`native.radar.ranking.${kind}.metric`) }}</span>
    </header>

    <ol>
      <li v-for="(item, index) in items" :key="`${item.model}:${item.effort}`">
        <div class="ranking-line">
          <span class="rank" :data-medal="index < 3 ? index + 1 : undefined">{{ index + 1 }}</span>
          <strong>{{ item.model }}</strong>
          <AIEffortBadge :effort="item.effort" />
          <b>{{ metric(item) }}</b>
        </div>
        <div class="ranking-track" aria-hidden="true">
          <span :style="{ width: progress(item) }"></span>
        </div>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.ranking-list {
  min-width: 0;
  border: 1px solid var(--ai-line);
  border-radius: 8px;
  background: var(--ai-surface);
  box-shadow: var(--ai-shadow);
}

.ranking-list > header {
  display: flex;
  min-height: 46px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 0 14px;
  border-bottom: 1px solid var(--ai-line);
}

.ranking-list h2 {
  margin: 0;
  color: var(--ai-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0;
}

.ranking-list header > span {
  color: var(--ai-faint);
  font-family: var(--ai-mono);
  font-size: 8px;
  text-transform: uppercase;
}

.ranking-list ol {
  display: grid;
  gap: 0;
  margin: 0;
  padding: 8px 13px 10px;
  list-style: none;
}

.ranking-list li {
  display: grid;
  gap: 7px;
  min-width: 0;
  padding: 10px 0;
  border-bottom: 1px solid var(--ai-line);
}

.ranking-list li:last-child {
  border-bottom: 0;
}

.ranking-line {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr) auto minmax(40px, auto);
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.rank {
  color: var(--ai-faint);
  font-family: var(--ai-mono);
  font-size: 9px;
  text-align: center;
}

.rank[data-medal] {
  display: grid;
  width: 18px;
  height: 18px;
  place-items: center;
  border-radius: 50%;
  color: #fff;
  background: var(--ai-gold);
  font-weight: 700;
}

.rank[data-medal="2"] {
  background: var(--ai-silver);
}

.rank[data-medal="3"] {
  background: var(--ai-bronze);
}

.ranking-line > strong {
  min-width: 0;
  overflow: hidden;
  color: var(--ai-ink);
  font-size: 10px;
  font-weight: 660;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ranking-line > b {
  color: var(--ai-ink);
  font-family: var(--ai-mono);
  font-size: 10px;
  text-align: right;
  white-space: nowrap;
}

.ranking-track {
  height: 5px;
  margin-left: 26px;
  overflow: hidden;
  border-radius: 3px;
  background: var(--ai-track);
}

.ranking-track span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--ai-success);
  transition: width 360ms ease;
}

.ranking-list[data-kind="speed"] .ranking-track span {
  background: var(--ai-cyan);
}

.ranking-list li:nth-child(n+7) .ranking-track span {
  background: var(--ai-warning);
}

.ranking-list li:nth-child(n+10) .ranking-track span {
  background: var(--ai-copper);
}

@media (prefers-reduced-motion: reduce) {
  .ranking-track span {
    transition: none;
  }
}
</style>
