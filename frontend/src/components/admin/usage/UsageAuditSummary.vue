<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UsageAuditStatsResponse, UsageAuditDimensionStats } from '@/api/admin/usage'

type AuditStatusKey = 'anomaly' | 'warning' | 'pass' | 'unverifiable' | 'missing' | 'not_applicable'

const props = withDefaults(defineProps<{
  stats: UsageAuditStatsResponse | null
  loading?: boolean
}>(), {
  loading: false,
})

const { t } = useI18n()

const statusCards = computed(() => {
  const counts = props.stats?.counts
  return [
    {
      key: 'anomaly' as const,
      icon: 'exclamationTriangle' as const,
      count: counts?.anomaly ?? 0,
      iconClass: 'bg-rose-100 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400',
      valueClass: 'text-rose-600 dark:text-rose-400',
    },
    {
      key: 'warning' as const,
      icon: 'infoCircle' as const,
      count: counts?.warning ?? 0,
      iconClass: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
      valueClass: 'text-amber-600 dark:text-amber-400',
    },
    {
      key: 'pass' as const,
      icon: 'checkCircle' as const,
      count: counts?.pass ?? 0,
      iconClass: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400',
      valueClass: 'text-emerald-600 dark:text-emerald-400',
    },
    {
      key: 'unverifiable' as const,
      icon: 'questionCircle' as const,
      count: counts?.unverifiable ?? 0,
      iconClass: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
      valueClass: 'text-slate-600 dark:text-slate-400',
    },
  ]
})

const dimensions = computed(() => [
  { key: 'platform', items: props.stats?.by_platform ?? [] },
  { key: 'account', items: props.stats?.by_account ?? [] },
  { key: 'model', items: props.stats?.by_model ?? [] },
])

const formatCount = (value: number | null | undefined) => (value ?? 0).toLocaleString()

const formatPercent = (value: number | null | undefined) =>
  value == null || !Number.isFinite(value) ? '-' : `${value.toFixed(1)}%`

const statusLabel = (status: AuditStatusKey) => t(`admin.usage.audit.status.${status}`)

const statusClass = (status: AuditStatusKey) => {
  switch (status) {
    case 'anomaly':
      return 'bg-rose-100 text-rose-700 dark:bg-rose-500/20 dark:text-rose-300'
    case 'warning':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300'
    case 'pass':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300'
    case 'unverifiable':
      return 'bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-300'
    case 'not_applicable':
      return 'bg-sky-100 text-sky-700 dark:bg-sky-500/20 dark:text-sky-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

const dimensionLabel = (key: string) => t(`admin.usage.audit.dimensions.${key}`)

const dimensionStatusCount = (item: UsageAuditDimensionStats) =>
  item.counts.anomaly + item.counts.warning
</script>

<template>
  <section class="space-y-4 border-b border-gray-100 px-4 py-4 dark:border-dark-700/50 sm:px-6">
    <div v-if="loading" class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div v-for="index in 4" :key="index" class="h-24 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800" />
    </div>

    <template v-else>
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div
          v-for="card in statusCards"
          :key="card.key"
          class="flex min-w-0 items-center gap-3 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900"
        >
          <div :class="['rounded-lg p-2', card.iconClass]">
            <Icon :name="card.icon" size="md" />
          </div>
          <div class="min-w-0">
            <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ statusLabel(card.key) }}</p>
            <p :class="['text-xl font-bold tabular-nums', card.valueClass]">{{ formatCount(card.count) }}</p>
          </div>
        </div>
      </div>

      <div class="grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,2fr)]">
        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.usage.audit.coverage') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.usage.audit.auditedRecords', { audited: formatCount(stats?.audited_records), total: formatCount(stats?.total_records) }) }}
              </p>
            </div>
            <span class="text-2xl font-bold tabular-nums text-primary-600 dark:text-primary-400">
              {{ formatPercent(stats?.coverage_pct) }}
            </span>
          </div>
          <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div
              class="h-full rounded-full bg-primary-500 transition-[width] duration-300"
              :style="{ width: `${Math.min(100, Math.max(0, stats?.coverage_pct ?? 0))}%` }"
            />
          </div>
          <div class="mt-4 grid grid-cols-2 gap-3 text-xs">
            <div class="rounded-lg bg-gray-50 p-2.5 dark:bg-dark-800">
              <p class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.averageDeviation') }} · {{ t('admin.usage.audit.inputShort') }}</p>
              <p class="mt-1 font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ formatPercent(stats?.average_input_deviation_pct) }}</p>
            </div>
            <div class="rounded-lg bg-gray-50 p-2.5 dark:bg-dark-800">
              <p class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.averageDeviation') }} · {{ t('admin.usage.audit.outputShort') }}</p>
              <p class="mt-1 font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ formatPercent(stats?.average_output_deviation_pct) }}</p>
            </div>
          </div>
        </div>

        <div class="grid gap-4 md:grid-cols-3">
          <div
            v-for="dimension in dimensions"
            :key="dimension.key"
            class="min-w-0 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
          >
            <div class="flex items-center justify-between gap-2">
              <p class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ dimensionLabel(dimension.key) }}</p>
              <Icon name="chartBar" size="sm" class="text-gray-400" />
            </div>
            <div v-if="dimension.items.length === 0" class="mt-5 text-xs text-gray-400 dark:text-gray-500">
              {{ t('admin.usage.audit.dimensions.empty') }}
            </div>
            <div v-else class="mt-3 space-y-2.5">
              <div v-for="item in dimension.items.slice(0, 5)" :key="item.key" class="min-w-0">
                <div class="flex items-center justify-between gap-2 text-xs">
                  <span class="min-w-0 truncate text-gray-700 dark:text-gray-300" :title="item.name">{{ item.name }}</span>
                  <span class="shrink-0 tabular-nums text-gray-500 dark:text-gray-400">{{ formatCount(item.total) }}</span>
                </div>
                <div class="mt-1 flex flex-wrap gap-1">
                  <span
                    v-if="item.counts.anomaly > 0"
                    :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', statusClass('anomaly')]"
                  >{{ statusLabel('anomaly') }} {{ item.counts.anomaly }}</span>
                  <span
                    v-if="item.counts.warning > 0"
                    :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', statusClass('warning')]"
                  >{{ statusLabel('warning') }} {{ item.counts.warning }}</span>
                  <span v-if="dimensionStatusCount(item) === 0" :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', statusClass('pass')]">
                    {{ statusLabel('pass') }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>
