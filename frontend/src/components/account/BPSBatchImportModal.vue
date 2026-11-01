<template>
  <BaseDialog :show="show" :title="t('openaiAuth.batchImport.title')" width="wide" @close="emit('close')">
    <div class="space-y-5">
      <p class="rounded-xl bg-primary-50 p-3 text-sm leading-6 text-primary-800 dark:bg-primary-950/30 dark:text-primary-200">{{ t('openaiAuth.batchImport.hint') }}</p>
      <div v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>
      <div v-if="loading" class="text-sm text-gray-500">{{ t('openaiAuth.batchImport.loading') }}</div>
      <template v-if="!job">
        <fieldset :disabled="busy || !!pending || loading || !loaded" class="space-y-5 disabled:opacity-60">
          <div>
            <label class="label">{{ t('openaiAuth.batchImport.groups') }}</label>
            <div class="grid max-h-40 grid-cols-1 gap-2 overflow-auto rounded-xl border border-gray-200 p-3 sm:grid-cols-2 dark:border-dark-600">
              <label v-for="group in availableGroups" :key="group.id" class="flex items-center gap-2 text-sm">
                <input v-model="groupIDs" type="checkbox" :value="group.id" class="checkbox" />{{ group.name }}
              </label>
              <span v-if="!availableGroups.length" class="text-sm text-gray-500">{{ t('openaiAuth.batchImport.noGroups') }}</span>
            </div>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <div><label for="bps-import-proxy" class="label">{{ t('openaiAuth.batchImport.proxy') }}</label>
              <select id="bps-import-proxy" v-model="proxyID" class="input"><option :value="null">{{ t('openaiAuth.batchImport.direct') }}</option><option v-for="proxy in availableProxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option></select>
            </div>
            <div><label for="bps-import-workspace" class="label">{{ t('openaiAuth.batchImport.workspace') }}</label><input id="bps-import-workspace" v-model="workspace" class="input" maxlength="200" :placeholder="t('openaiAuth.batchImport.optional')" /></div>
            <div><label for="bps-import-concurrency" class="label">{{ t('openaiAuth.batchImport.concurrency') }}</label><input id="bps-import-concurrency" v-model.number="concurrency" class="input" type="number" min="1" max="100" /></div>
            <div><label for="bps-import-priority" class="label">{{ t('openaiAuth.batchImport.priority') }}</label><input id="bps-import-priority" v-model.number="priority" class="input" type="number" min="0" max="10000" /></div>
          </div>
          <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('openaiAuth.batchImport.proxyHint') }}</p>
          <div><label class="label">{{ t('openaiAuth.batchImport.models') }}</label>
            <ModelWhitelistSelector v-model="models" platform="openai" :account-id="syncAccountID" :show-sync-button="!!syncAccountID" :show-catalog-button="false" :catalog-models="DEFAULT_EXCEL_BPS_MODELS" />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('openaiAuth.batchImport.modelsHint') }}</p>
          </div>
          <div><label for="bps-import-entries" class="label">{{ t('openaiAuth.batchImport.entries') }}</label>
            <textarea id="bps-import-entries" v-model="raw" class="input min-h-40 font-mono text-sm" rows="6" autocomplete="off" autocapitalize="off" spellcheck="false" :placeholder="t('openaiAuth.batchImport.placeholder')" />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('openaiAuth.batchImport.format') }}</p>
          </div>
        </fieldset>
        <p v-if="pending" class="text-sm text-amber-700 dark:text-amber-300">{{ t('openaiAuth.batchImport.uncertain') }}</p>
      </template>
      <template v-else>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="text-sm font-medium">{{ t('openaiAuth.batchImport.progress', { done: finished, total: job.rows.length }) }} · {{ t(`openaiAuth.batchImport.state.${missing ? 'unavailable' : job.status}`) }}</p>
          <span class="text-xs text-gray-500">{{ new Date(job.created_at).toLocaleString() }}</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700"><div class="h-full rounded-full bg-primary-500 transition-all" :style="{width: `${100 * finished / job.rows.length}%`}" /></div>
        <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="w-full text-left text-sm"><thead class="bg-gray-50 dark:bg-dark-800"><tr><th class="p-3">{{ t('openaiAuth.batchImport.account') }}</th><th class="p-3">{{ t('openaiAuth.batchImport.result') }}</th></tr></thead>
            <tbody><tr v-for="row in job.rows" :key="row.email" class="border-t border-gray-100 dark:border-dark-700"><td class="p-3 align-top"><span class="break-all">{{ row.email }}</span><span v-if="row.account_id" class="mt-1 block text-xs text-gray-500">ID {{ row.account_id }}</span></td><td class="p-3"><span :class="row.status === 'succeeded' ? 'text-emerald-600' : row.status === 'failed' ? 'text-red-600' : 'text-gray-600 dark:text-gray-300'">{{ t(`openaiAuth.batchImport.state.${row.status}`) }}</span><p v-if="row.message" class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ row.message }}</p></td></tr></tbody>
          </table>
        </div>
      </template>
      <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('openaiAuth.batchImport.background') }}</p>
    </div>
    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-2">
        <button class="btn btn-secondary" :disabled="busy || loading" @click="refresh">{{ t('openaiAuth.batchImport.refresh') }}</button>
        <button class="btn btn-secondary" @click="emit('close')">{{ t('openaiAuth.batchImport.close') }}</button>
        <button v-if="running" class="btn btn-danger" :disabled="busy || job?.status === 'canceling'" @click="cancel">{{ t('openaiAuth.batchImport.cancel') }}</button>
        <button v-else-if="job" class="btn btn-primary" @click="newBatch">{{ t('openaiAuth.batchImport.newBatch') }}</button>
        <button v-else class="btn btn-primary" :disabled="busy || loading || !loaded" @click="submit">{{ t(busy ? 'openaiAuth.batchImport.submitting' : pending ? 'openaiAuth.batchImport.retry' : 'openaiAuth.batchImport.start') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelWhitelistSelector from './ModelWhitelistSelector.vue'
import { DEFAULT_EXCEL_BPS_MODELS } from '@/constants/account'
import { bpsBatchImportAPI as api, parseBPSImportEntries, type BPSImportInput, type BPSImportStatus } from '@/api/admin/bpsBatchImport'
import type { AdminGroup, Proxy } from '@/types'
const props = defineProps<{ show: boolean; groups: AdminGroup[]; proxies: Proxy[] }>()
const emit = defineEmits<{ close: []; created: [] }>()
const { t } = useI18n()
const groupIDs = ref<number[]>([]), proxyID = ref<number | null>(null), workspace = ref('')
const concurrency = ref(3), priority = ref(50), models = ref<string[]>([...DEFAULT_EXCEL_BPS_MODELS]), raw = ref('')
const job = ref<BPSImportStatus | null>(null), pending = shallowRef<BPSImportInput | null>(null)
const busy = ref(false), loading = ref(false), loaded = ref(false), error = ref(''), syncAccountID = ref<number>()
const availableGroups = computed(() => props.groups.filter(g => g.platform === 'openai' && g.status === 'active'))
const availableProxies = computed(() => props.proxies.filter(p => p.status === 'active' && (!p.expires_at || Date.parse(p.expires_at) > Date.now())))
const missing = ref(false)
const running = computed(() => !missing.value && (job.value?.status === 'running' || job.value?.status === 'canceling'))
const finished = computed(() => job.value?.rows.filter(r => !['queued', 'running'].includes(r.status)).length || 0)
let timer: ReturnType<typeof setTimeout> | undefined, disposed = false
let completedNotified = ''
const stopPolling = () => { if (timer) clearTimeout(timer); timer = undefined }
function accept(value: BPSImportStatus) {
  missing.value = false
  job.value = value
  if (pending.value?.request_id === value.request_id) { pending.value = null; raw.value = '' }
  syncAccountID.value = value.rows.find(r => r.status === 'succeeded')?.account_id || syncAccountID.value
  if (!['running', 'canceling'].includes(value.status) && completedNotified !== value.id) { completedNotified = value.id; emit('created') }
}
function schedule() {
  stopPolling()
  if (props.show && running.value && !disposed) timer = setTimeout(refresh, 2000)
}
async function refresh() {
  if (loading.value || busy.value || disposed) return
  loading.value = true
  try {
    const list = await api.list()
    if (disposed) return
    error.value = ''
    const found = list.find(x => x.request_id === pending.value?.request_id) || list.find(x => ['running', 'canceling'].includes(x.status)) || list.find(x => x.id === job.value?.id) || (!loaded.value ? list[0] : undefined)
    if (found) accept(found)
    else if (job.value && !list.some(x => x.id === job.value?.id)) { missing.value = true; error.value = t('openaiAuth.batchImport.expired') }
    loaded.value = true
  } catch { error.value = t('openaiAuth.batchImport.refreshFailed') }
  finally { loading.value = false; schedule() }
}
async function submit() {
  if (busy.value || job.value || !loaded.value) return
  error.value = ''
  if (!pending.value) {
    try {
      const entries = parseBPSImportEntries(raw.value)
      if (!groupIDs.value.length || !models.value.length || !Number.isInteger(concurrency.value) || concurrency.value < 1 || concurrency.value > 100 || !Number.isInteger(priority.value) || priority.value < 0 || priority.value > 10000) { error.value = t('openaiAuth.batchImport.invalidSettings'); return }
      pending.value = { request_id: crypto.randomUUID(), entries, group_ids: [...groupIDs.value], proxy_id: proxyID.value, workspace: workspace.value.trim(), concurrency: concurrency.value, priority: priority.value, models: [...models.value] }
    } catch (err) {
      const message = err instanceof Error ? err.message : ''
      error.value = message.startsWith('line:') ? t('openaiAuth.batchImport.invalidLine', { line: message.slice(5) }) : t('openaiAuth.batchImport.invalidCount')
      return
    }
  }
  busy.value = true
  try { accept(await api.start(pending.value!)); raw.value = ''; pending.value = null }
  catch (err) {
    const response = (err as { response?: { status?: number; data?: { message?: string } } }).response
    if (response?.status && response.status >= 400 && response.status < 500 && response.status !== 408 && response.status !== 429) pending.value = null
    error.value = response?.data?.message || t('openaiAuth.batchImport.submitFailed')
  } finally { busy.value = false; schedule() }
}
async function cancel() {
  if (!job.value || busy.value) return
  busy.value = true; error.value = ''
  try { accept(await api.cancel(job.value.id)) } catch { error.value = t('openaiAuth.batchImport.cancelFailed') }
  finally { busy.value = false; schedule() }
}
function newBatch() { if (running.value || pending.value) return; job.value = null; missing.value = false; raw.value = ''; error.value = '' }
watch(() => props.show, show => { if (show) void refresh(); else stopPolling() }, { immediate: true })
onUnmounted(() => { disposed = true; stopPolling(); raw.value = ''; pending.value = null })
</script>
