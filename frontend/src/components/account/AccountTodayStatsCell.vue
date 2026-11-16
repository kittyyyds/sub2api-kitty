<template>
  <AccountUsageSummaryGrid v-if="stats || loading" :stats="stats" :loading="loading" />
  <span v-else class="text-xs text-gray-400">—</span>
</template>

<script setup lang="ts">
import type { WindowStats } from '@/types'
import AccountUsageSummaryGrid from './AccountUsageSummaryGrid.vue'
withDefaults(defineProps<{ stats?: WindowStats | null; loading?: boolean }>(), { stats: null, loading: false })
</script>
