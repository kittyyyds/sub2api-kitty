<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { AdminUsageLog, UsageAuditStatus } from '@/types'

type AuditRowStatus = UsageAuditStatus | 'missing'

const props = defineProps<{
  show: boolean
  row: AdminUsageLog | null
}>()

const emit = defineEmits<{
  close: []
}>()

const { t, te } = useI18n()

const audit = computed(() => props.row?.usage_audit ?? null)
const status = computed<AuditRowStatus>(() => audit.value?.status ?? 'missing')

const statusLabel = computed(() => audit.value?.reasons?.includes('local_estimate_disabled')
  ? t('admin.usage.audit.localEstimateDisabled')
  : t(`admin.usage.audit.status.${status.value}`))

const statusClass = computed(() => {
  switch (status.value) {
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
})

interface ComparisonRow {
  key: string
  label: string
  upstream: number | null | undefined
  local: number | null | undefined
  delta: number | null | undefined
  deviation: number | null | undefined
}

const comparisonRows = computed<ComparisonRow[]>(() => {
  if (!audit.value) return []
  return [
    {
      key: 'input',
      label: t('admin.usage.audit.input'),
      upstream: audit.value.upstream.total_input_tokens ?? audit.value.upstream.input_tokens,
      local: audit.value.local.total_input_tokens ?? audit.value.local.input_tokens,
      delta: audit.value.delta.input_tokens,
      deviation: audit.value.deviation_pct.input_tokens,
    },
    {
      key: 'output',
      label: t('admin.usage.audit.output'),
      upstream: audit.value.upstream.output_tokens,
      local: audit.value.local.output_tokens,
      delta: audit.value.delta.output_tokens,
      deviation: audit.value.deviation_pct.output_tokens,
    },
    {
      key: 'cacheCreation',
      label: t('admin.usage.audit.cacheCreation'),
      upstream: audit.value.upstream.cache_creation_tokens,
      local: null,
      delta: null,
      deviation: null,
    },
    {
      key: 'cacheRead',
      label: t('admin.usage.audit.cacheRead'),
      upstream: audit.value.upstream.cache_read_tokens,
      local: null,
      delta: null,
      deviation: null,
    },
  ]
})

const formatNumber = (value: number | null | undefined) => value == null ? '-' : value.toLocaleString()

const formatSigned = (value: number | null | undefined) => {
  if (value == null) return '-'
  return `${value > 0 ? '+' : ''}${value.toLocaleString()}`
}

const formatPercent = (value: number | null | undefined) => {
  if (value == null || !Number.isFinite(value)) return '-'
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`
}

const evidenceLabel = computed(() => {
  const source = audit.value?.evidence_source
  return source ? t(`admin.usage.audit.evidence.${source}`) : t('admin.usage.audit.noRawUsage')
})

const confidenceLabel = computed(() => {
  const value = audit.value?.confidence
  return value ? t(`admin.usage.audit.confidence.${value}`) : '-'
})

const reasonLabel = (reason: string) => {
  const key = `admin.usage.audit.reasonsMap.${reason}`
  return te(key) ? t(key) : reason
}

const rawUsage = computed(() => {
  if (!audit.value?.upstream_raw_usage) return ''
  try {
    return JSON.stringify(audit.value.upstream_raw_usage, null, 2)
  } catch {
    return String(audit.value.upstream_raw_usage)
  }
})
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('admin.usage.audit.detailTitle')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="row" class="space-y-5">
      <div class="flex flex-wrap items-start justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <span :class="['inline-flex rounded px-2.5 py-1 text-xs font-semibold', statusClass]">{{ statusLabel }}</span>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(row.created_at) }}</span>
          </div>
          <p class="mt-2 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ row.request_id || `#${row.id}` }}</p>
        </div>
        <div class="text-right text-xs text-gray-500 dark:text-gray-400">
          <p>{{ t('usage.model') }}: <span class="font-medium text-gray-800 dark:text-gray-200">{{ row.model || '-' }}</span></p>
          <p class="mt-1">{{ t('admin.usage.account') }}: <span class="font-medium text-gray-800 dark:text-gray-200">{{ row.account?.name || `#${row.account_id}` }}</span></p>
        </div>
      </div>

      <section v-if="row.image_count > 0" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700" data-testid="image-output-evidence">
        <h4 class="mb-3 text-sm font-semibold">{{ t('admin.usage.imageEvidence.title') }}</h4>
        <template v-if="audit?.image_output_evidence">
          <dl class="grid gap-3 text-sm sm:grid-cols-2">
            <div><dt class="text-xs text-gray-500">{{ t('admin.usage.imageEvidence.model') }}</dt><dd>{{ t('admin.usage.imageEvidence.modelStatus.' + audit.image_output_evidence.model_status) }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('admin.usage.imageEvidence.size') }}</dt><dd>{{ t('admin.usage.imageEvidence.sizeStatus.' + audit.image_output_evidence.size_status) }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('admin.usage.imageEvidence.requested') }}</dt><dd>{{ audit.image_output_evidence.requested_size || 'auto' }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('admin.usage.imageEvidence.processing') }}</dt><dd>{{ t(audit.image_output_evidence.postprocessed === false ? 'admin.usage.imageEvidence.unchanged' : 'admin.usage.imageEvidence.unknown') }}</dd></div>
          </dl>
          <div v-for="(image, index) in audit.image_output_evidence.outputs" :key="index" class="mt-3 text-sm">#{{ index + 1 }} · {{ t('admin.usage.imageEvidence.declared') }} {{ image.declared_size || '—' }} · {{ t('admin.usage.imageEvidence.measured') }} {{ image.measured_size || '—' }}</div>
        </template>
        <p v-else class="text-sm text-gray-500">{{ t('admin.usage.imageEvidence.noEvidence') }}</p>
        <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.usage.imageEvidence.hint') }}</p>
      </section>

      <section>
        <h4 class="mb-3 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-gray-100">
          <Icon name="document" size="sm" class="text-primary-500" />
          {{ t('admin.usage.audit.requestInfo') }}
        </h4>
        <dl class="grid gap-3 rounded-xl border border-gray-200 p-4 text-sm dark:border-dark-700 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.requestedModel') }}</dt>
            <dd class="mt-1 break-all font-medium text-gray-800 dark:text-gray-200">{{ row.model || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.sentUpstreamModel') }}</dt>
            <dd class="mt-1 break-all font-medium text-gray-800 dark:text-gray-200">{{ row.upstream_model || row.model || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.upstreamResponseModel') }}</dt>
            <dd class="mt-1 break-all font-medium text-gray-800 dark:text-gray-200">{{ row.upstream_response_model || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.usage.group') }}</dt>
            <dd class="mt-1 font-medium text-gray-800 dark:text-gray-200">{{ row.group?.name || `#${row.group_id ?? '-'}` }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.type') }}</dt>
            <dd class="mt-1 font-medium text-gray-800 dark:text-gray-200">{{ row.request_type || '-' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.protocolLabel') }}</dt>
            <dd class="mt-1 font-mono text-xs text-gray-800 dark:text-gray-200">{{ audit?.protocol || '-' }}</dd>
          </div>
        </dl>
      </section>

      <section>
        <h4 class="mb-3 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-gray-100">
          <Icon name="chartBar" size="sm" class="text-primary-500" />
          {{ t('admin.usage.audit.comparison') }}
        </h4>
        <div v-if="audit" class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[620px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.metric') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.reported') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.estimated') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.delta') }}</th>
                <th class="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.usage.audit.deviation') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
              <tr v-for="item in comparisonRows" :key="item.key">
                <td class="px-4 py-3 font-medium text-gray-800 dark:text-gray-200">{{ item.label }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ formatNumber(item.upstream) }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ formatNumber(item.local) }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ formatSigned(item.delta) }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ formatPercent(item.deviation) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="rounded-xl border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.usage.audit.noRawUsage') }}
        </div>
      </section>

      <section class="grid gap-4 lg:grid-cols-2">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <h4 class="mb-3 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-gray-100">
            <Icon name="shield" size="sm" class="text-primary-500" />
            {{ t('admin.usage.audit.evidenceInfo') }}
          </h4>
          <dl class="space-y-2 text-sm">
            <div class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.confidenceLabel') }}</dt><dd class="font-medium text-gray-800 dark:text-gray-200">{{ confidenceLabel }}</dd></div>
            <div class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.sourceLabel') }}</dt><dd class="font-medium text-gray-800 dark:text-gray-200">{{ evidenceLabel }}</dd></div>
            <div class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.estimatorLabel') }}</dt><dd class="max-w-[65%] break-all text-right font-mono text-xs text-gray-800 dark:text-gray-200">{{ audit?.estimator || '-' }}</dd></div>
            <div v-if="audit?.cache_hit_rate != null" class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.audit.hitRate', { value: '' }).replace(/\s*$/, '') }}</dt><dd class="font-medium text-sky-600 dark:text-sky-400">{{ audit.cache_hit_rate.toFixed(2) }}%</dd></div>
          </dl>
          <p v-if="audit?.capture_truncated" class="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">{{ t('admin.usage.audit.truncated') }}</p>
        </div>

        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.usage.audit.reasons') }}</h4>
          <ul v-if="audit?.reasons?.length" class="space-y-2 text-sm text-gray-700 dark:text-gray-300">
            <li v-for="(reason, index) in audit.reasons" :key="`${reason}-${index}`" class="flex items-start gap-2">
              <Icon name="chevronRight" size="sm" class="mt-0.5 shrink-0 text-gray-400" />
              <span>{{ reasonLabel(reason) }}</span>
            </li>
          </ul>
          <p v-else class="text-sm text-gray-500 dark:text-gray-400">-</p>
        </div>
      </section>

      <details v-if="rawUsage" class="rounded-xl border border-gray-200 dark:border-dark-700">
        <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.usage.audit.rawUsage') }}</summary>
        <pre class="max-h-72 overflow-auto border-t border-gray-200 bg-gray-50 p-4 text-xs leading-5 text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300">{{ rawUsage }}</pre>
      </details>
    </div>
  </BaseDialog>
</template>
