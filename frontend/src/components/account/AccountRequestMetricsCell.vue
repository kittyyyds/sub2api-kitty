<template>
  <button type="button" class="grid grid-cols-2 gap-x-3 gap-y-0.5 text-left text-[11px] text-gray-500 hover:text-primary-600 dark:text-gray-400" :title="t('accountPerformance.semantics')" @click="$emit('configure')">
    <span v-if="!stats" class="col-span-2">{{ loading ? t('common.loading') : t('accountPerformance.unavailable') }}</span>
    <template v-else>
      <span>{{ t('accountPerformance.success_rate') }} <b class="font-medium tabular-nums">{{ percent(stats.success_rate) }}</b></span>
      <span>{{ t('accountPerformance.cache_rate') }} <b class="font-medium tabular-nums">{{ percent(stats.cache_rate) }}</b></span>
      <span>{{ t('accountPerformance.ttft') }} <b class="font-medium tabular-nums">{{ duration(stats.avg_first_token_ms) }}</b></span>
      <span>{{ t('accountPerformance.duration') }} <b class="font-medium tabular-nums">{{ duration(stats.avg_duration_ms) }}</b></span>
    </template>
  </button>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { OpsAccountRequestStats } from '@/api/admin/accountRequestStats'
defineProps<{ stats?: OpsAccountRequestStats; loading: boolean }>()
defineEmits<{ configure: [] }>()
const { t } = useI18n()
const percent = (value: number | null) => value == null ? '—' : `${(value * 100).toFixed(1)}%`
const duration = (value: number | null) => value == null ? '—' : value < 1000 ? `${Math.round(value)}ms` : `${(value / 1000).toFixed(2)}s`
</script>
