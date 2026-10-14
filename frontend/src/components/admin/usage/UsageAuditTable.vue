<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { AdminUsageLog, UsageAuditStatus } from '@/types'

type AuditRowStatus = UsageAuditStatus | 'missing'

withDefaults(defineProps<{
  data: AdminUsageLog[]
  loading?: boolean
}>(), {
  loading: false,
})

const emit = defineEmits<{
  inspect: [row: AdminUsageLog]
  userClick: [userId: number, email?: string]
}>()

const { t } = useI18n()

const getAuditStatus = (row: AdminUsageLog): AuditRowStatus => row.usage_audit?.status ?? 'missing'

const statusLabel = (row: AdminUsageLog) => row.usage_audit?.reasons?.includes('local_estimate_disabled')
  ? t('admin.usage.audit.localEstimateDisabled')
  : t(`admin.usage.audit.status.${getAuditStatus(row)}`)

const statusClass = (status: AuditRowStatus) => {
  switch (status) {
    case 'anomaly':
      return 'bg-rose-100 text-rose-700 ring-rose-200 dark:bg-rose-500/20 dark:text-rose-300 dark:ring-rose-500/30'
    case 'warning':
      return 'bg-amber-100 text-amber-700 ring-amber-200 dark:bg-amber-500/20 dark:text-amber-300 dark:ring-amber-500/30'
    case 'pass':
      return 'bg-emerald-100 text-emerald-700 ring-emerald-200 dark:bg-emerald-500/20 dark:text-emerald-300 dark:ring-emerald-500/30'
    case 'unverifiable':
      return 'bg-slate-100 text-slate-700 ring-slate-200 dark:bg-slate-700 dark:text-slate-300 dark:ring-slate-600'
    case 'not_applicable':
      return 'bg-sky-100 text-sky-700 ring-sky-200 dark:bg-sky-500/20 dark:text-sky-300 dark:ring-sky-500/30'
    default:
      return 'bg-gray-100 text-gray-700 ring-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:ring-dark-600'
  }
}

const formatTokens = (value: number | null | undefined) => value == null ? '-' : value.toLocaleString()

const formatSigned = (value: number | null | undefined) => {
  if (value == null) return '-'
  return `${value > 0 ? '+' : ''}${value.toLocaleString()}`
}

const formatDeviation = (value: number | null | undefined) => {
  if (value == null || !Number.isFinite(value)) return '-'
  return `${value > 0 ? '+' : ''}${value.toFixed(1)}%`
}

const upstreamInput = (row: AdminUsageLog) => {
  const audit = row.usage_audit
  return audit?.upstream.total_input_tokens ?? audit?.upstream.input_tokens
}

const localInput = (row: AdminUsageLog) => {
  const audit = row.usage_audit
  return audit?.local.total_input_tokens ?? audit?.local.input_tokens
}

const upstreamOutput = (row: AdminUsageLog) => row.usage_audit?.upstream.output_tokens
const localOutput = (row: AdminUsageLog) => row.usage_audit?.local.output_tokens

const confidenceLabel = (row: AdminUsageLog) => {
  const confidence = row.usage_audit?.confidence
  return confidence ? t(`admin.usage.audit.confidence.${confidence}`) : '-'
}

const evidenceLabel = (row: AdminUsageLog) => {
  const source = row.usage_audit?.evidence_source
  return source ? t(`admin.usage.audit.evidence.${source}`) : t('admin.usage.audit.noRawUsage')
}
</script>

<template>
  <div class="overflow-x-auto">
    <table class="w-full min-w-[1040px] divide-y divide-gray-200 dark:divide-dark-700">
      <thead class="bg-gray-50 dark:bg-dark-800">
        <tr>
          <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.status') }}</th>
          <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.request') }}</th>
          <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.modelAccount') }}</th>
          <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.upstream') }}</th>
          <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.local') }}</th>
          <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.deviation') }}</th>
          <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.cache') }}</th>
          <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.evidence') }}</th>
          <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.columns.action') }}</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
        <template v-if="loading">
          <tr v-for="index in 6" :key="index">
            <td v-for="column in 9" :key="column" class="px-4 py-4"><div class="h-4 animate-pulse rounded bg-gray-100 dark:bg-dark-800" /></td>
          </tr>
        </template>
        <tr v-else-if="data.length === 0">
          <td colspan="9" class="px-4 py-14 text-center">
            <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.empty') }}</p>
          </td>
        </tr>
        <tr v-for="row in data" v-else :key="row.id" class="align-top transition-colors hover:bg-gray-50 dark:hover:bg-dark-800/60">
          <td class="px-4 py-3">
            <div class="flex flex-col items-start gap-1.5">
              <span :class="['inline-flex rounded px-2 py-0.5 text-xs font-semibold ring-1 ring-inset', statusClass(getAuditStatus(row))]">
                {{ statusLabel(row) }}
              </span>
              <span v-if="row.usage_audit" class="text-[11px] text-gray-400 dark:text-gray-500">
                {{ confidenceLabel(row) }}
              </span>
            </div>
          </td>
          <td class="max-w-[190px] px-4 py-3 text-xs">
            <p class="text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</p>
            <p class="mt-1 truncate font-mono text-gray-400 dark:text-gray-500" :title="row.request_id">{{ row.request_id || `#${row.id}` }}</p>
          </td>
          <td class="max-w-[220px] px-4 py-3 text-xs">
            <p class="truncate font-medium text-gray-800 dark:text-gray-200" :title="row.model">{{ row.model || '-' }}</p>
            <p class="mt-1 truncate text-gray-500 dark:text-gray-400" :title="row.upstream_model || undefined">↳ {{ row.upstream_model || row.model || '-' }}</p>
            <button
              v-if="row.user?.email"
              type="button"
              data-testid="audit-user-link"
              class="mt-1 max-w-full truncate text-left text-primary-600 hover:underline dark:text-primary-400"
              @click="emit('userClick', row.user_id, row.user.email)"
            >
              {{ row.user.email }}
            </button>
            <p v-else class="mt-1 text-gray-400 dark:text-gray-500">#{{ row.user_id }}</p>
            <p class="truncate text-gray-400 dark:text-gray-500">{{ row.account?.name || `Account #${row.account_id}` }}</p>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-right text-xs tabular-nums text-gray-700 dark:text-gray-300">
            <p>{{ t('admin.usage.audit.inputTokens', { value: formatTokens(upstreamInput(row)) }) }}</p>
            <p>{{ t('admin.usage.audit.outputTokens', { value: formatTokens(upstreamOutput(row)) }) }}</p>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-right text-xs tabular-nums text-gray-700 dark:text-gray-300">
            <p>{{ t('admin.usage.audit.inputTokens', { value: formatTokens(localInput(row)) }) }}</p>
            <p>{{ t('admin.usage.audit.outputTokens', { value: formatTokens(localOutput(row)) }) }}</p>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-right text-xs tabular-nums">
            <p :class="row.usage_audit?.deviation_pct.input_tokens != null ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400'">
              {{ formatSigned(row.usage_audit?.delta.input_tokens) }} / {{ formatDeviation(row.usage_audit?.deviation_pct.input_tokens) }}
            </p>
            <p :class="row.usage_audit?.deviation_pct.output_tokens != null ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400'">
              {{ formatSigned(row.usage_audit?.delta.output_tokens) }} / {{ formatDeviation(row.usage_audit?.deviation_pct.output_tokens) }}
            </p>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-gray-400">
            <p>{{ t('admin.usage.audit.cacheTokens', { value: formatTokens(row.usage_audit?.upstream.cache_read_tokens) }) }}</p>
            <p v-if="row.usage_audit?.cache_hit_rate != null" class="mt-1 text-sky-600 dark:text-sky-400">
              {{ t('admin.usage.audit.hitRate', { value: `${row.usage_audit.cache_hit_rate.toFixed(1)}%` }) }}
            </p>
          </td>
          <td class="max-w-[150px] px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
            <p class="truncate" :title="evidenceLabel(row)">{{ evidenceLabel(row) }}</p>
            <p v-if="row.usage_audit?.capture_truncated" class="mt-1 text-amber-600 dark:text-amber-400">{{ t('admin.usage.audit.truncated') }}</p>
          </td>
          <td class="px-4 py-3 text-right">
            <button
              type="button"
              data-testid="audit-inspect"
              class="inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/30"
              :title="t('admin.usage.audit.inspect')"
              @click="emit('inspect', row)"
            >
              <Icon name="eye" size="sm" />
              <span>{{ t('admin.usage.audit.inspect') }}</span>
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
