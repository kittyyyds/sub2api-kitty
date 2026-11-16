<template>
  <section class="card flex min-w-0 flex-col overflow-hidden p-0" data-testid="recharge-ranking-panel" :aria-busy="loading">
    <header class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-4 dark:border-dark-700">
      <div class="min-w-0"><h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.operations.rechargeRanking.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ ranking ? t('admin.operations.rechargeRanking.window', { date: ranking.date, timezone: ranking.site_timezone }) : t('admin.operations.rechargeRanking.hint') }}</p></div>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.operations.rechargeRanking.top50') }}</span>
    </header>
    <div class="grid grid-cols-3 gap-2 p-4 text-xs text-gray-500 dark:text-gray-400">
      <div><p>{{ t('admin.operations.rechargeRanking.totalAmount') }}</p><p class="mt-1 break-all text-sm font-semibold tabular-nums text-emerald-700 dark:text-emerald-400" data-testid="recharge-ranking-total">{{ ranking ? formatMoney(ranking.total_amount) : '—' }}</p></div>
      <div><p>{{ t('admin.operations.rechargeRanking.totalUsers') }}</p><p class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white" data-testid="recharge-ranking-users">{{ ranking ? ranking.total_users : '—' }}</p></div>
      <div><p>{{ t('admin.operations.rechargeRanking.totalCount') }}</p><p class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white" data-testid="recharge-ranking-count">{{ ranking ? ranking.total_count : '—' }}</p></div>
    </div>
    <div class="relative min-h-40 max-h-[360px] flex-1 overflow-y-auto overscroll-contain" data-testid="recharge-ranking-scroll">
      <div v-if="loading" class="absolute inset-0 z-10 flex min-h-40 items-center justify-center bg-white/75 dark:bg-dark-800/75" role="status"><LoadingSpinner size="md" /></div>
      <div v-if="error && !loading" class="px-4 py-4 text-center text-sm text-gray-500 dark:text-gray-400" role="alert"><p>{{ t('admin.operations.rechargeRanking.loadFailed') }}</p><button type="button" class="mt-1 text-primary-600 hover:underline" @click="$emit('retry')">{{ t('admin.operations.retry') }}</button></div>
      <div v-for="(entry, index) in ranking?.users ?? []" :key="entry.user_id" class="flex items-center gap-3 border-b border-gray-100 px-4 py-3 last:border-b-0 dark:border-dark-700/70" :data-ranking-user="entry.user_id">
        <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold" :class="index < 3 ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : 'bg-gray-100 text-gray-500 dark:bg-dark-700'">{{ index + 1 }}</span>
        <span class="min-w-0 flex-1 truncate text-sm text-gray-900 dark:text-white" :title="entry.email">{{ entry.email }}</span>
        <div class="shrink-0 text-right"><p class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatMoney(entry.amount) }}</p><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.operations.rechargeRanking.count', { count: entry.recharge_count }) }}</p></div>
      </div>
      <p v-if="!error && !loading && !ranking?.users.length" class="px-4 py-12 text-center text-sm text-gray-400">{{ t('admin.operations.rechargeRanking.empty') }}</p>
    </div>
  </section>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { OperationsRechargeRankingResponse } from '@/api/admin/dashboard'
import { formatMoney } from '@/views/admin/operationsUtils'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
defineProps<{ ranking: OperationsRechargeRankingResponse | null; loading: boolean; error: boolean }>()
defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>
