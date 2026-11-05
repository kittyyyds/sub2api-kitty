<template>
  <BaseDialog :show="show" :title="t('optimizations.auditTitle')" width="extra-wide" @close="emit('close')">
    <div class="space-y-4">
      <p class="rounded-lg bg-primary-50 p-3 text-sm text-primary-800 dark:bg-primary-900/20 dark:text-primary-200">{{ t('optimizations.auditNotice') }}</p>
      <div class="flex flex-wrap items-center gap-3">
        <button class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
        <select v-model="status" class="input w-auto" :aria-label="t('common.status')">
          <option value="">{{ t('optimizations.auditAll') }}</option>
          <option v-for="s in statuses" :key="s" :value="s">{{ t(`optimizations.auditStatus.${s}`) }} ({{ report?.rows.filter(r => r.status === s).length ?? 0 }})</option>
        </select>
        <span v-if="report" class="text-xs text-gray-500">{{ formatDateTime(report.queried_at) }}</span>
      </div>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="report?.truncated" role="alert" class="text-sm text-amber-600">{{ t('optimizations.auditTruncated') }}</p>
      <div class="max-h-[55vh] overflow-auto">
        <table class="w-full text-left text-sm">
          <thead><tr class="border-b dark:border-dark-700"><th class="p-2">{{ t('keys.group') }}</th><th class="p-2">{{ t('usage.model') }}</th><th class="p-2">{{ t('common.status') }}</th><th class="p-2">{{ t('optimizations.auditBasis') }}</th></tr></thead>
          <tbody><tr v-for="row in pageRows" :key="`${row.group_id}:${row.model}`" class="border-b dark:border-dark-700">
            <td class="p-2">#{{ row.group_id }} {{ row.group_name }}</td><td class="break-all p-2 font-mono text-xs">{{ row.model }}</td>
            <td class="p-2" :class="row.status === 'missing' || row.status === 'invalid' ? 'text-red-600' : row.status === 'review' ? 'text-amber-600' : 'text-emerald-600'">{{ t(`optimizations.auditStatus.${row.status}`) }}</td>
            <td class="p-2 text-xs text-gray-500">{{ t(`optimizations.auditReason.${row.reason}`) }}<br>{{ row.source }} {{ row.mode }}</td>
          </tr></tbody>
        </table>
        <p v-if="loading" role="status" class="p-4 text-center">{{ t('common.loading') }}</p>
      </div>
      <Pagination :total="rows.length" :page="page" :page-size="50" :show-page-size-selector="false" show-jump @update:page="page = $event" />
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { apiClient } from '@/api/client'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
interface AuditRow { group_id: number; group_name: string; model: string; status: string; source: string; mode: string; reason: string }
interface AuditReport { queried_at: string; read_only: boolean; truncated: boolean; rows: AuditRow[] }
const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const report = ref<AuditReport>()
const loading = ref(false)
const error = ref('')
const status = ref('')
const page = ref(1)
const statuses = ['missing', 'invalid', 'review', 'free', 'priced']
const rows = computed(() => (report.value?.rows ?? []).filter(r => !status.value || r.status === status.value))
const pageRows = computed(() => rows.value.slice((page.value - 1) * 50, page.value * 50))
let request: AbortController | undefined
async function load() {
  request?.abort(); const controller = new AbortController(); request = controller
  loading.value = true; error.value = ''
  try {
    const result = await apiClient.get<AuditReport>('/admin/channels/pricing-audit', { signal: controller.signal })
    if (!controller.signal.aborted) { report.value = result.data; page.value = 1 }
  } catch (e) { if (!controller.signal.aborted) error.value = extractApiErrorMessage(e, t('common.error')) }
  finally { if (!controller.signal.aborted) loading.value = false }
}
watch(status, () => { page.value = 1 })
watch(() => props.show, show => { if (show) void load(); else request?.abort() }, { immediate: true })
onBeforeUnmount(() => request?.abort())
</script>
