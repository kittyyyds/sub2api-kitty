<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { AIServiceStatus, AIServiceStatusValue } from '@/api/aiIntelligence'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  status: AIServiceStatus
}>()

const { t, locale } = useI18n()

const overallLabel = computed(() => statusLabel(props.status.overall_status))
const heading = computed(() => t('native.radar.service.heading', { status: overallLabel.value }))
const windowSummary = computed(() => t('native.radar.service.windowSummary', {
  window: props.status.window_days,
  free: props.status.event_free_days,
}))

function statusLabel(status: AIServiceStatusValue): string {
  return t(`native.radar.service.status.${status}`)
}

function componentLabel(key: string): string {
  return t(`native.radar.service.component.${key}`)
}

function formatDay(value: string): string {
  const date = new Date(`${value}T00:00:00Z`)
  if (Number.isNaN(date.valueOf())) return value
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: '2-digit',
    day: '2-digit',
    timeZone: 'UTC',
  }).format(date)
}

function formatIncidentDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return value
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: '2-digit',
    day: '2-digit',
  }).format(date)
}
</script>

<template>
  <section class="service-status" :data-status="status.overall_status">
    <div class="service-summary">
      <span class="service-mark" aria-hidden="true">
        <Icon name="shield" size="sm" :stroke-width="2" />
      </span>
      <div class="service-copy">
        <div class="service-heading-row">
          <h1>{{ heading }}</h1>
          <div class="component-list" :aria-label="t('native.radar.service.componentsAria')">
            <span
              v-for="component in status.components"
              :key="component.key"
              class="component-chip"
              :data-status="component.status"
              :title="statusLabel(component.status)"
            >
              <i aria-hidden="true"></i>
              {{ componentLabel(component.key) }}
            </span>
          </div>
        </div>
        <p>{{ t('native.radar.service.description') }}</p>
      </div>
    </div>

    <div class="service-history">
      <div class="history-row">
        <div class="history-bars" :aria-label="windowSummary">
          <span
            v-for="day in status.days"
            :key="day.date"
            :data-status="day.status"
            :title="`${formatDay(day.date)} · ${statusLabel(day.status)}`"
          ></span>
        </div>
        <strong>{{ windowSummary }}</strong>
      </div>
      <p v-if="status.recent_incident" class="recent-incident">
        <i :data-status="status.recent_incident.status" aria-hidden="true"></i>
        {{ t('native.radar.service.recentIncident', {
          date: formatIncidentDate(status.recent_incident.occurred_at),
          title: status.recent_incident.title,
        }) }}
      </p>
      <p v-else class="recent-incident is-clear">
        <i data-status="operational" aria-hidden="true"></i>
        {{ t('native.radar.service.noRecentIncident') }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.service-status {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(330px, 0.62fr);
  align-items: center;
  gap: 24px;
  min-width: 0;
  padding: 16px 18px;
  border: 1px solid color-mix(in srgb, var(--ai-success) 35%, var(--ai-line));
  border-radius: 8px;
  color: var(--ai-ink);
  background: color-mix(in srgb, var(--ai-success-soft) 68%, var(--ai-surface));
}

.service-status[data-status="degraded_performance"],
.service-status[data-status="under_maintenance"] {
  border-color: color-mix(in srgb, var(--ai-warning) 42%, var(--ai-line));
  background: color-mix(in srgb, var(--ai-warning-soft) 64%, var(--ai-surface));
}

.service-status[data-status="partial_outage"],
.service-status[data-status="major_outage"] {
  border-color: color-mix(in srgb, var(--ai-danger) 38%, var(--ai-line));
  background: color-mix(in srgb, var(--ai-danger-soft) 64%, var(--ai-surface));
}

.service-summary,
.service-heading-row,
.component-list,
.history-row,
.recent-incident {
  display: flex;
  align-items: center;
}

.service-summary {
  min-width: 0;
  gap: 12px;
}

.service-mark {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 7px;
  color: #fff;
  background: var(--ai-success);
}

.service-status:not([data-status="operational"]) .service-mark {
  background: var(--ai-warning);
}

.service-copy {
  min-width: 0;
}

.service-heading-row {
  flex-wrap: wrap;
  gap: 10px 14px;
}

.service-heading-row h1 {
  margin: 0;
  font-size: 14px;
  font-weight: 720;
  letter-spacing: 0;
}

.component-list {
  flex-wrap: wrap;
  gap: 6px;
}

.component-chip {
  display: inline-flex;
  height: 24px;
  align-items: center;
  gap: 6px;
  padding: 0 9px;
  border: 1px solid var(--ai-line);
  border-radius: 999px;
  color: var(--ai-muted);
  background: color-mix(in srgb, var(--ai-surface) 78%, transparent);
  font-size: 10px;
  white-space: nowrap;
}

.component-chip i,
.recent-incident i {
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--ai-success);
}

.component-chip[data-status="degraded_performance"] i,
.component-chip[data-status="under_maintenance"] i,
.recent-incident i[data-status="degraded_performance"],
.recent-incident i[data-status="under_maintenance"] {
  background: var(--ai-warning);
}

.component-chip[data-status="partial_outage"] i,
.component-chip[data-status="major_outage"] i,
.recent-incident i[data-status="partial_outage"],
.recent-incident i[data-status="major_outage"] {
  background: var(--ai-danger);
}

.component-chip[data-status="unknown"] i {
  background: var(--ai-muted);
}

.service-copy > p {
  margin: 7px 0 0;
  color: var(--ai-muted);
  font-size: 10px;
  line-height: 1.55;
}

.service-history {
  display: grid;
  min-width: 0;
  gap: 7px;
}

.history-row {
  justify-content: flex-end;
  gap: 12px;
}

.history-row strong {
  color: var(--ai-muted);
  font-size: 10px;
  font-weight: 560;
  white-space: nowrap;
}

.history-bars {
  display: grid;
  grid-template-columns: repeat(15, 5px);
  gap: 2px;
}

.history-bars span {
  width: 5px;
  height: 22px;
  border-radius: 2px;
  background: var(--ai-success);
}

.history-bars span[data-status="degraded_performance"],
.history-bars span[data-status="under_maintenance"] {
  background: var(--ai-warning);
}

.history-bars span[data-status="partial_outage"],
.history-bars span[data-status="major_outage"] {
  background: var(--ai-danger);
}

.history-bars span[data-status="unknown"] {
  background: var(--ai-line-strong);
}

.recent-incident {
  justify-content: flex-end;
  gap: 7px;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: var(--ai-muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 980px) {
  .service-status {
    grid-template-columns: 1fr;
  }

  .history-row,
  .recent-incident {
    justify-content: flex-start;
  }
}

@media (max-width: 560px) {
  .service-status {
    padding: 14px;
  }

  .service-summary {
    align-items: flex-start;
  }

  .history-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 8px;
  }
}
</style>
