<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'

import type { AIIntelligencePoint } from '@/api/aiIntelligence'
import Icon from '@/components/icons/Icon.vue'
import {
  passRate,
  sortRecommendationRows,
  type AIRecommendationSortKey,
  type SortDirection,
} from '@/features/customer-console/aiIntelligence'
import AIEffortBadge from './AIEffortBadge.vue'

const props = defineProps<{
  rows: readonly AIIntelligencePoint[]
}>()

const { t, locale } = useI18n()
const sortKey = shallowRef<AIRecommendationSortKey>('recommendation_score')
const sortDirection = shallowRef<SortDirection>('desc')

const columns = computed<Array<{ key: AIRecommendationSortKey; label: string }>>(() => [
  { key: 'model', label: t('native.radar.table.model') },
  { key: 'effort', label: t('native.radar.table.effort') },
  { key: 'iq', label: 'IQ' },
  { key: 'average_minutes', label: t('native.radar.table.duration') },
  { key: 'average_price_usd', label: t('native.radar.table.cost') },
  { key: 'pass_rate', label: t('native.radar.table.pass') },
  { key: 'recommendation_score', label: t('native.radar.table.score') },
])

const sortedRows = computed(() => sortRecommendationRows(
  props.rows,
  sortKey.value,
  sortDirection.value,
))

const numberFormatter = computed(() => new Intl.NumberFormat(
  locale.value === 'zh' ? 'zh-CN' : 'en-US',
  { maximumFractionDigits: 2 },
))

function toggleSort(key: AIRecommendationSortKey): void {
  if (sortKey.value === key) {
    sortDirection.value = sortDirection.value === 'desc' ? 'asc' : 'desc'
    return
  }
  sortKey.value = key
  sortDirection.value = key === 'model' || key === 'effort' ? 'asc' : 'desc'
}

function ariaSort(key: AIRecommendationSortKey): 'ascending' | 'descending' | 'none' {
  if (sortKey.value !== key) return 'none'
  return sortDirection.value === 'asc' ? 'ascending' : 'descending'
}
</script>

<template>
  <section class="recommendation-table-panel">
    <header class="table-heading">
      <h2>{{ t('native.radar.table.title') }}</h2>
      <span>{{ t('native.radar.table.note') }}</span>
    </header>

    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th
              v-for="column in columns"
              :key="column.key"
              :aria-sort="ariaSort(column.key)"
            >
              <button type="button" @click="toggleSort(column.key)">
                {{ column.label }}
                <Icon
                  v-if="sortKey === column.key"
                  :name="sortDirection === 'desc' ? 'chevronDown' : 'chevronUp'"
                  size="xs"
                  :stroke-width="2"
                />
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, index) in sortedRows"
            :key="`${row.model}:${row.effort}`"
            :data-rank="index < 3 ? index + 1 : undefined"
          >
            <td><strong>{{ row.model }}</strong></td>
            <td><AIEffortBadge :effort="row.effort" /></td>
            <td>{{ numberFormatter.format(row.iq) }}</td>
            <td>{{ t('native.radar.units.minutes', { value: numberFormatter.format(row.average_minutes) }) }}</td>
            <td>${{ row.average_price_usd.toFixed(2) }}</td>
            <td>
              <strong>{{ Math.round(passRate(row)) }}%</strong>
              <small>{{ Math.round(row.passed) }}/{{ Math.round(row.valid_tasks) }}</small>
            </td>
            <td class="score-cell">
              <span v-if="index < 3">{{ index + 1 }}</span>
              <strong>{{ row.recommendation_score.toFixed(1) }}</strong>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.recommendation-table-panel {
  min-width: 0;
  border: 1px solid var(--ai-line);
  border-radius: 8px;
  background: var(--ai-surface);
  box-shadow: var(--ai-shadow);
}

.table-heading {
  display: flex;
  min-height: 46px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 16px;
  border-bottom: 1px solid var(--ai-line);
}

.table-heading h2 {
  margin: 0;
  color: var(--ai-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0;
}

.table-heading span {
  color: var(--ai-faint);
  font-size: 9px;
}

.table-scroll {
  overflow-x: auto;
}

table {
  width: 100%;
  min-width: 760px;
  border-collapse: collapse;
  color: var(--ai-muted);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

th,
td {
  height: 44px;
  padding: 7px 11px;
  border-bottom: 1px solid var(--ai-line);
  text-align: right;
  white-space: nowrap;
}

th:first-child,
td:first-child {
  text-align: left;
}

th:nth-child(2),
td:nth-child(2) {
  text-align: center;
}

th {
  height: 38px;
  color: var(--ai-faint);
  font-weight: 560;
}

th button {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  border-radius: 4px;
  color: inherit;
  font: inherit;
}

th:first-child button {
  justify-content: flex-start;
}

th button:hover,
th[aria-sort]:not([aria-sort="none"]) button {
  color: var(--ai-cyan);
}

th button:focus-visible {
  outline: 2px solid var(--ai-cyan);
  outline-offset: 2px;
}

tbody tr:last-child td {
  border-bottom: 0;
}

tbody tr[data-rank="1"] {
  background: color-mix(in srgb, var(--ai-gold-soft) 78%, transparent);
}

tbody tr[data-rank="2"] {
  background: color-mix(in srgb, var(--ai-blue-soft) 54%, transparent);
}

tbody tr[data-rank="3"] {
  background: color-mix(in srgb, var(--ai-copper-soft) 50%, transparent);
}

td > strong {
  color: var(--ai-ink);
  font-weight: 660;
}

td small {
  margin-left: 4px;
  color: var(--ai-faint);
}

.score-cell span {
  display: inline-grid;
  width: 18px;
  height: 18px;
  margin-right: 6px;
  place-items: center;
  border-radius: 50%;
  color: #fff;
  background: var(--ai-gold);
  font-family: var(--ai-mono);
  font-size: 9px;
}

tr[data-rank="2"] .score-cell span {
  background: var(--ai-silver);
}

tr[data-rank="3"] .score-cell span {
  background: var(--ai-bronze);
}

.score-cell strong {
  color: var(--ai-copper);
  font-family: var(--ai-mono);
}

@media (max-width: 560px) {
  .table-heading span {
    display: none;
  }
}
</style>
