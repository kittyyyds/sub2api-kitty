<script setup lang="ts">
import { shallowRef } from 'vue'
import UsageAuditSummary from '@/components/admin/usage/UsageAuditSummary.vue'
import UsageAuditTable from '@/components/admin/usage/UsageAuditTable.vue'
import UsageAuditDetailDialog from '@/components/admin/usage/UsageAuditDetailDialog.vue'
import type { UsageAuditStatsResponse } from '@/api/admin/usage'
import type { AdminUsageLog } from '@/types'

const props = withDefaults(defineProps<{
  stats: UsageAuditStatsResponse | null
  statsLoading?: boolean
  rows: AdminUsageLog[]
  loading?: boolean
}>(), {
  statsLoading: false,
  loading: false,
})

const emit = defineEmits<{
  userClick: [userId: number, email?: string]
}>()

const selectedRow = shallowRef<AdminUsageLog | null>(null)
const showDetail = shallowRef(false)

const openDetail = (row: AdminUsageLog) => {
  selectedRow.value = row
  showDetail.value = true
}

const closeDetail = () => {
  showDetail.value = false
}
</script>

<template>
  <div>
    <UsageAuditSummary :stats="props.stats" :loading="props.statsLoading" />
    <UsageAuditTable
      :data="props.rows"
      :loading="props.loading"
      @inspect="openDetail"
      @user-click="(...args) => emit('userClick', ...args)"
    />
    <UsageAuditDetailDialog :show="showDetail" :row="selectedRow" @close="closeDetail" />
  </div>
</template>
