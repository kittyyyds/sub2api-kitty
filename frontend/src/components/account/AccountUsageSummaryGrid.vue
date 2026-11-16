<template>
  <div
    v-if="props.loading && !props.stats"
    class="grid min-w-[204px] grid-cols-2 gap-x-2 text-[9px] leading-4"
    data-testid="account-usage-summary-loading"
  >
    <section v-for="group in 2" :key="group" class="space-y-1">
      <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div v-for="line in 4" :key="line" class="flex items-center justify-between gap-1">
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
    </section>
  </div>

  <div v-else-if="props.stats" class="grid min-w-[204px] grid-cols-2 gap-x-2 text-[9px] leading-4" data-testid="account-usage-summary">
    <section class="min-w-0" data-testid="today-stats">
      <div class="mb-0.5 font-semibold text-emerald-600 dark:text-emerald-400">
        {{ t('admin.accounts.stats.todayGroup') }}
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.summaryRequests') }}</span>
        <span class="font-medium text-gray-900 dark:text-gray-100">{{ formatRequests(props.stats.requests) }}</span>
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.summaryTokens') }}</span>
        <span class="font-medium text-gray-900 dark:text-gray-100">{{ formatTokens(props.stats.tokens) }}</span>
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400" :title="t('usage.accountBilled')">{{ t('admin.accounts.stats.accountBilled') }}</span>
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ formatCost('A', props.stats.cost) }}</span>
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400" :title="t('usage.userBilled')">{{ t('admin.accounts.stats.userBilled') }}</span>
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ formatCost('U', props.stats.user_cost) }}</span>
      </div>
    </section>

    <section class="min-w-0 border-l border-gray-200 pl-2 dark:border-gray-700" data-testid="lifetime-stats">
      <div class="mb-0.5 font-semibold text-gray-600 dark:text-gray-300">
        {{ t('admin.accounts.stats.totalGroup') }}
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.summaryRequests') }}</span>
        <span class="font-medium text-gray-900 dark:text-gray-100">{{ formatRequests(props.stats.lifetime_requests) }}</span>
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.summaryTokens') }}</span>
        <span class="font-medium text-gray-900 dark:text-gray-100">{{ formatTokens(props.stats.lifetime_tokens) }}</span>
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400" :title="t('usage.accountBilled')">{{ t('admin.accounts.stats.accountBilled') }}</span>
        <span class="font-medium text-gray-900 dark:text-gray-100">{{ formatCost('A', props.stats.lifetime_cost) }}</span>
      </div>
      <div class="flex items-center justify-between gap-1 whitespace-nowrap">
        <span class="text-gray-500 dark:text-gray-400" :title="t('usage.userBilled')">{{ t('admin.accounts.stats.userBilled') }}</span>
        <span class="font-medium text-gray-900 dark:text-gray-100">{{ formatCost('U', props.stats.lifetime_user_cost) }}</span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'

const props = withDefaults(defineProps<{ stats?: WindowStats | null; loading?: boolean }>(), {
  stats: null,
  loading: false
})
const { t } = useI18n()

function formatRequests(value?: number | null): string {
  if (value == null) return '—'
  return `${formatCompactNumber(value, { allowBillions: false })} req`
}

function formatTokens(value?: number | null): string {
  return value == null ? '—' : formatCompactNumber(value)
}

function formatCost(prefix: 'A' | 'U', value?: number | null): string {
  return value == null ? '—' : `${prefix} $${value.toFixed(2)}`
}
</script>
