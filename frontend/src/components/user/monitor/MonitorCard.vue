<template>
  <button
    type="button"
    class="group flex h-[168px] min-w-0 w-full flex-col overflow-hidden rounded-lg border border-gray-200 bg-white p-4 text-left shadow-sm transition-[border-color,box-shadow] hover:border-gray-300 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-dark-600"
    :aria-label="item.name"
    @click="emit('click')"
  >
    <div class="flex min-w-0 items-start justify-between gap-3">
      <div class="flex min-w-0 items-start gap-2.5">
        <span
          class="grid h-7 w-7 shrink-0 place-items-center rounded-md"
          :class="[providerBackgroundClass, providerTintClass]"
        >
          <ProviderIcon :provider="item.provider" :size="16" />
        </span>
        <div class="min-w-0">
          <div class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
            {{ item.name }}
          </div>
          <div class="mt-0.5 min-w-0">
            <span class="truncate font-mono text-[10px] text-gray-400">
              {{ formatMonitorModel(item.primary_model) }}
            </span>
          </div>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-1.5">
        <span
          v-if="rateMultiplier != null"
          class="inline-flex items-center rounded-md px-2 py-1 font-mono text-[10px] font-medium"
          :class="[providerBackgroundClass, providerTintClass]"
        >
          {{ t('monitorCommon.rateLabel') }}: {{ formatMultiplier(rateMultiplier) }}x
        </span>
        <span
          class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-[10px] font-medium"
          :class="statusChipClass"
        >
          <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass"></span>
          {{ cardStatusLabel }}
        </span>
      </div>
    </div>

    <div class="mt-3 flex min-w-0 items-baseline justify-between gap-3">
      <div class="flex min-w-0 items-baseline gap-5 text-[11px]">
        <span class="whitespace-nowrap text-gray-400">
          {{ t('monitorCommon.dialogLatency') }}
          <span class="ml-1 font-mono text-xs text-gray-900 dark:text-gray-100">
            {{ formatLatency(item.primary_latency_ms) }}ms
          </span>
        </span>
        <span class="whitespace-nowrap text-gray-400">
          {{ t('monitorCommon.endpointPing') }}
          <span class="ml-1 font-mono text-xs text-gray-900 dark:text-gray-100">
            {{ formatLatency(item.primary_ping_latency_ms) }}ms
          </span>
        </span>
      </div>
      <span
        class="origin-right shrink-0 scale-[1.5] font-mono text-sm font-semibold tabular-nums"
        :style="availabilityStyle"
      >
        {{ availabilityText }}
      </span>
    </div>

    <MonitorQuotaView
      v-if="quotaVisible"
      :snapshot="item.latest_quota"
      class="mt-2 max-h-8 overflow-hidden"
    />

    <div class="mt-auto">
      <div class="mb-1.5 text-[9px] text-gray-400">
        {{ t('monitorCommon.checksCount', { n: TIMELINE_LENGTH }) }}
      </div>
      <MonitorTimeline :buckets="item.timeline" :length="TIMELINE_LENGTH" />
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import { useChannelMonitorFormat, hslForPct } from '@/composables/useChannelMonitorFormat'
import { formatMultiplier } from '@/utils/formatters'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import {
  STATUS_DEGRADED,
  STATUS_FAILED,
  STATUS_OPERATIONAL,
} from '@/constants/channelMonitor'
import ProviderIcon from './ProviderIcon.vue'
import MonitorTimeline from './MonitorTimeline.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'

const TIMELINE_LENGTH = 60

const PROVIDER_TINT: Record<string, string> = {
  openai: 'text-emerald-600 dark:text-emerald-300',
  anthropic: 'text-orange-600 dark:text-orange-300',
  gemini: 'text-sky-600 dark:text-sky-300',
  grok: 'text-zinc-700 dark:text-zinc-200',
  antigravity: 'text-purple-600 dark:text-purple-300',
  kimi: 'text-pink-600 dark:text-pink-300',
  zhipu: 'text-indigo-600 dark:text-indigo-300',
  deepseek: 'text-teal-600 dark:text-teal-300',
  opencode_go: 'text-amber-700 dark:text-amber-300',
}

const PROVIDER_BACKGROUND: Record<string, string> = {
  openai: 'bg-emerald-50 dark:bg-emerald-500/10',
  anthropic: 'bg-orange-50 dark:bg-orange-500/10',
  gemini: 'bg-sky-50 dark:bg-sky-500/10',
  grok: 'bg-zinc-100 dark:bg-zinc-500/10',
}

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

const providerTintClass = computed(() =>
  PROVIDER_TINT[props.item.provider] ?? 'text-gray-500 dark:text-gray-300'
)

const providerBackgroundClass = computed(() =>
  PROVIDER_BACKGROUND[props.item.provider] ?? 'bg-gray-100 dark:bg-dark-700'
)

const quotaVisible = computed(
  () => isChannelMonitorQuotaVisible() && !!props.item.latest_quota,
)

const statusChipClass = computed(() => {
  switch (props.item.primary_status) {
    case STATUS_OPERATIONAL:
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
    case STATUS_DEGRADED:
      return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
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
  return `${props.availabilityValue.toFixed(0)}%`
})

const availabilityStyle = computed(() => {
  const color = hslForPct(props.availabilityValue)
  return { color: color ?? 'rgb(156 163 175)' }
})
</script>
