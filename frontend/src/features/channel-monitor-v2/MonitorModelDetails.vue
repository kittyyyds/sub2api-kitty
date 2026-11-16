<template>
  <BaseDialog :show="!!group" :title="`${group?.group_name || ''} · ${t('channelMonitorV2.cards.models')}`" width="wide" placement="right" @close="$emit('close')">
    <p class="mb-4 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.cards.modelsHint') }}</p>
    <p v-if="loading" class="py-8 text-center text-sm text-gray-500" role="status">{{ t('channelMonitorV2.updating') }}</p>
    <div v-else-if="error" class="space-y-3 py-8 text-center text-sm text-red-600 dark:text-red-300" role="alert">
      <p>{{ t('channelMonitorV2.detailLoadFailed') }}</p>
      <button class="btn btn-secondary btn-sm" type="button" @click="load">{{ t('channelMonitorV2.cards.retry') }}</button>
    </div>
    <p v-else-if="!items.length" class="py-8 text-center text-sm text-gray-500">{{ t('channelMonitorV2.cards.noSamples') }}</p>
    <div v-else class="space-y-3">
      <article v-for="row in items" :key="`${row.platform}-${row.group_id}-${row.model}`" class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700" data-testid="monitor-model-item">
        <div class="flex items-start justify-between gap-3">
          <h4 class="min-w-0 break-all font-mono text-sm font-semibold text-gray-900 dark:text-gray-100">{{ row.model === '__other__' ? t('channelMonitorV2.otherModels') : row.model }}</h4>
          <span class="shrink-0 rounded-full px-2 py-1 text-[10px]" :class="healthClass(row.health.overall)">{{ t(`channelMonitorV2.cards.health.${row.health.overall}`) }}</span>
        </div>
        <dl class="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div><dt class="text-xs text-gray-500">{{ t('channelMonitorV2.cards.availability') }}</dt><dd class="mt-1 font-mono text-sm">{{ hasMonitorSamples(row.metrics) ? formatMonitorPercent(1 - row.metrics.error_rate) : '—' }}</dd></div>
          <div><dt class="text-xs text-gray-500">{{ t('channelMonitorV2.cards.ttft') }}</dt><dd class="mt-1 font-mono text-sm">{{ formatMonitorMs(row.metrics.ttft.p50_ms) }}</dd></div>
          <div><dt class="text-xs text-gray-500">{{ t('channelMonitorV2.cards.cache') }}</dt><dd class="mt-1 font-mono text-sm">{{ hasMonitorSamples(row.metrics) ? formatMonitorPercent(row.metrics.cache_rate) : '—' }}</dd></div>
          <div v-if="showThroughput"><dt class="text-xs text-gray-500" :title="t('channelMonitorV2.metrics.tpsDetail')">{{ t('channelMonitorV2.metrics.tps') }}</dt><dd class="mt-1 font-mono text-sm">{{ formatMonitorOutputTPS(row.metrics.output_tps) }}</dd></div>
        </dl>
      </article>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getMatrix, type HealthState, type MonitorFilter, type MonitorMatrixRow } from '@/api/channelMonitorV2'
import { hasMonitorSamples } from './monitorCards'
import { formatMonitorMs, formatMonitorOutputTPS, formatMonitorPercent } from './monitorFormat'

const props = defineProps<{ group: MonitorMatrixRow | null; filter: MonitorFilter; admin?: boolean; showThroughput?: boolean; refreshKey?: unknown }>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
const items = ref<MonitorMatrixRow[]>([])
const loading = ref(false)
const error = ref(false)
let controller: AbortController | undefined
let sequence = 0
async function load() {
  controller?.abort()
  const id = ++sequence
  const row = props.group
  if (!row?.group_id) { items.value = []; loading.value = false; error.value = false; return }
  const request = new AbortController()
  controller = request
  loading.value = true
  error.value = false
  try {
    const result = await getMatrix({ ...props.filter, platforms: [row.platform], groupIds: [row.group_id] }, 'platform_group_model', props.admin, request.signal)
    if (id !== sequence) return
    items.value = result.items.filter(item => item.platform === row.platform && item.group_id === row.group_id)
  } catch {
    if (id === sequence && !request.signal.aborted) { items.value = []; error.value = true }
  } finally {
    if (id === sequence) loading.value = false
  }
}
// No background query while closed; open panels reuse the page's existing refresh.
watch([() => props.group, () => props.filter, () => props.refreshKey], () => { void load() }, { immediate: true, deep: false })
onBeforeUnmount(() => { ++sequence; controller?.abort() })
function healthClass(state: HealthState) {
  return { healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300', warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300', critical: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300', unknown: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400' }[state]
}
</script>
