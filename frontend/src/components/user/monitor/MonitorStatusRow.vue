<template>
  <article class="status-row" :data-testid="`monitor-status-row-${item.id}`">
    <button type="button" class="status-row-head" :aria-label="item.name" @click="emit('click')">
      <span class="status-row-dot" :class="statusDotClass" :title="cardStatusLabel"></span>
      <span class="status-row-title"><strong>{{ item.name }}</strong><span>{{ formatMonitorModel(item.primary_model) }}</span></span>
      <span class="status-row-figures">
        <span v-if="rateMultiplier != null" class="status-row-rate">{{ formatMultiplier(rateMultiplier) }}x</span>
        <span class="status-row-chip" :class="statusChipClass">{{ cardStatusLabel }}</span>
        <span class="status-row-uptime" :style="availabilityStyle">{{ availabilityText }}</span>
      </span>
    </button>
    <div class="status-row-latency">
      <span>{{ t('monitorCommon.dialogLatency') }} <strong>{{ formatLatency(item.primary_latency_ms) }}ms</strong></span>
      <span>{{ t('monitorCommon.endpointPing') }} <strong>{{ formatLatency(item.primary_ping_latency_ms) }}ms</strong></span>
      <span>{{ t('monitorCommon.checksCount', { n: TIMELINE_LENGTH }) }}</span>
    </div>
    <MonitorTimeline :buckets="item.timeline" :length="TIMELINE_LENGTH" />
    <MonitorQuotaView v-if="quotaVisible" :snapshot="item.latest_quota" class="mt-3" />
  </article>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { formatMultiplier } from '@/utils/formatters'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import {
  STATUS_DEGRADED,
  STATUS_FAILED,
  STATUS_OPERATIONAL,
} from '@/constants/channelMonitor'

import MonitorTimeline from './MonitorTimeline.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'

const TIMELINE_LENGTH = 60

const props = defineProps<{
  item: UserMonitorView
  availabilityValue: number | null
  rateMultiplier?: number | null
  window?: '7d' | '15d' | '30d'
  countdownSeconds?: number
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()
const {
  formatLatency,
  formatMonitorModel,
} = useChannelMonitorFormat()

const quotaVisible = computed(
  () => isChannelMonitorQuotaVisible() && !!props.item.latest_quota,
)

const statusChipClass = computed(() => {
  switch (props.item.primary_status) {
    case STATUS_OPERATIONAL:
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
    case STATUS_DEGRADED:
      return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
    case 'error':
    case STATUS_FAILED:
      return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
  }
})

const statusDotClass = computed(() => {
  switch (props.item.primary_status) {
    case STATUS_OPERATIONAL:
      return 'bg-emerald-500'
    case STATUS_DEGRADED:
      return 'bg-amber-500'
    case 'error':
    case STATUS_FAILED:
      return 'bg-red-500'
    default:
      return 'bg-gray-400'
  }
})

const cardStatusLabel = computed(() => {
  switch (props.item.primary_status) {
    case STATUS_OPERATIONAL:
      return t('monitorCommon.cardStatus.operational')
    case STATUS_DEGRADED:
      return t('monitorCommon.cardStatus.degraded')
    case 'error':
    case STATUS_FAILED:
      return t('monitorCommon.cardStatus.failed')
    default:
      return t('monitorCommon.cardStatus.error')
  }
})

const availabilityText = computed(() => {
  if (props.availabilityValue == null || Number.isNaN(props.availabilityValue)) {
    return t('monitorCommon.latencyEmpty')
  }
  return `${Number(props.availabilityValue.toFixed(2))}%`
})

const availabilityStyle = computed(() => {
  const value = props.availabilityValue
  const color = value == null || !Number.isFinite(value) ? 'var(--kc-muted)'
    : value >= 99 ? 'var(--kc-success)' : value >= 95 ? 'var(--kc-warning)' : 'var(--kc-danger)'
  return { color }
})
</script>

<style scoped>
.status-row { padding: 18px 20px; min-width: 0; border-top: 1px solid var(--kc-line); }
.status-row-head { display: flex; align-items: center; gap: 10px; width: 100%; text-align: left; border-radius: 6px; }
.status-row-head:hover .status-row-title strong { color: var(--kc-accent); }
.status-row-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.status-row-title { flex: 1; min-width: 0; display: grid; gap: 3px; }
.status-row-title strong { font-size: 13px; color: var(--kc-ink); overflow-wrap: anywhere; }
.status-row-title > span { font-size: 10px; color: var(--kc-muted); overflow-wrap: anywhere; }
.status-row-figures { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 8px; font-variant-numeric: tabular-nums; }
.status-row-chip,.status-row-rate { font-size: 10px; padding: 3px 6px; border-radius: 6px; }
.status-row-rate { background: var(--kc-panel-muted); color: var(--kc-muted); }
.status-row-uptime { font-size: 13px; font-weight: 700; }
.status-row-latency { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 5px 12px; margin: 12px 0 8px; color: var(--kc-muted); font-size: 10px; }
.status-row-latency strong { color: var(--kc-ink); font-weight: 500; }
@media(max-width: 480px) { .status-row { padding: 16px 14px; } .status-row-head { flex-wrap: wrap; } .status-row-title { flex-basis: calc(100% - 20px); } .status-row-figures { width: 100%; justify-content: flex-start; padding-left: 18px; } }
</style>
