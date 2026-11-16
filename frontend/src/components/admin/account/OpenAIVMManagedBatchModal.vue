<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.vmManaged.batch.title')"
    width="wide"
    :close-on-escape="!busy && !running"
    :show-close-button="!busy"
    @close="close"
  >
    <div class="space-y-4" :aria-busy="busy">
      <p class="rounded-lg bg-sky-50 p-3 text-sm text-sky-900 dark:bg-sky-900/20 dark:text-sky-200">{{ t('admin.accounts.vmManaged.batch.intro') }}</p>
      <p v-if="error" role="alert" class="whitespace-pre-wrap break-words rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" data-testid="batch-error">{{ error }}</p>

      <!-- Step 1: paste and choose -->
      <template v-if="!job">
        <div>
          <label for="batch-text" class="input-label">{{ t('admin.accounts.vmManaged.batch.text') }}</label>
          <textarea id="batch-text" v-model="form.text" class="input h-40 font-mono text-xs" autocomplete="off" spellcheck="false" :disabled="busy" :placeholder="t('admin.accounts.vmManaged.batch.placeholder')" @input="preview = null" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.batch.textHint', { max: 20 }) }}</p>
        </div>

        <div>
          <span class="input-label">{{ t('admin.accounts.vmManaged.batch.proxies') }}</span>
          <div v-if="socksProxies.length" class="mt-1 flex max-h-32 flex-wrap gap-x-4 gap-y-2 overflow-y-auto">
            <label v-for="proxy in socksProxies" :key="proxy.id" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
              <input v-model="form.proxyIds" type="checkbox" :value="proxy.id" :disabled="busy" @change="preview = null" />
              {{ proxy.name }} ({{ proxy.host }}:{{ proxy.port }})
            </label>
          </div>
          <p v-else class="mt-1 text-xs text-amber-600 dark:text-amber-400">{{ t('admin.accounts.vmManaged.create.noProxy') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.batch.proxiesHint') }}</p>
        </div>

        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div>
            <label for="batch-per-proxy" class="input-label">{{ t('admin.accounts.vmManaged.batch.perProxy') }}</label>
            <input id="batch-per-proxy" v-model.number="form.perProxy" type="number" min="1" max="32" class="input" :disabled="busy" @input="preview = null" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.batch.perProxyHint') }}</p>
          </div>
          <div>
            <label for="batch-runtime" class="input-label">{{ t('admin.accounts.vmManaged.runtime') }}</label>
            <select id="batch-runtime" v-model="form.runtime" class="input" :disabled="busy" @change="preview = null">
              <option value="kvm">KVM</option>
              <option value="docker">Docker</option>
            </select>
          </div>
          <div>
            <label for="batch-concurrency" class="input-label">{{ t('admin.accounts.vmManaged.create.concurrency') }}</label>
            <input id="batch-concurrency" v-model.number="form.concurrency" type="number" min="1" max="256" class="input" :disabled="busy" />
          </div>
          <div>
            <label for="batch-priority" class="input-label">{{ t('admin.accounts.vmManaged.create.priority') }}</label>
            <input id="batch-priority" v-model.number="form.priority" type="number" min="0" class="input" :disabled="busy" />
          </div>
        </div>

        <div v-if="openaiGroups.length">
          <span class="input-label">{{ t('admin.accounts.vmManaged.create.groups') }}</span>
          <div class="mt-1 flex flex-wrap gap-x-4 gap-y-2">
            <label v-for="group in openaiGroups" :key="group.id" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
              <input v-model="form.groupIds" type="checkbox" :value="group.id" :disabled="busy" />
              {{ group.name }}
            </label>
          </div>
        </div>

        <!-- Step 2: what will happen, before anything is created -->
        <section v-if="preview" class="space-y-2 rounded-xl border border-gray-200 p-4 text-sm dark:border-dark-600" data-testid="batch-preview">
          <p class="font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.batch.previewSummary', { ok: preview.importable, bad: preview.problems.length }) }}</p>
          <p v-if="preview.estimated_memory_mb" class="text-gray-600 dark:text-gray-300" data-testid="batch-footprint">{{ t('admin.accounts.vmManaged.batch.footprint', { memory: Math.round(preview.estimated_memory_mb / 102.4) / 10, disk: preview.estimated_disk_gb }) }}</p>
          <ul v-if="preview.lines.length" class="max-h-32 space-y-0.5 overflow-y-auto text-gray-700 dark:text-gray-200">
            <li v-for="line in preview.lines" :key="line.line_no">{{ t('admin.accounts.vmManaged.batch.lineOk', { line: line.line_no, email: line.email, proxy: proxyName(line.proxy_id, line.new_proxy) }) }}</li>
          </ul>
          <ul v-if="preview.problems.length" class="max-h-32 space-y-0.5 overflow-y-auto text-amber-700 dark:text-amber-300" data-testid="batch-problems">
            <li v-for="problem in preview.problems" :key="problem.line_no">{{ t('admin.accounts.vmManaged.batch.lineBad', { line: problem.line_no, email: problem.email || '—', reason: reasonText(problem.code) }) }}</li>
          </ul>
        </section>
      </template>

      <!-- Step 3: progress -->
      <section v-else class="space-y-2 text-sm" data-testid="batch-progress">
        <p class="font-medium text-gray-900 dark:text-white">{{ t(`admin.accounts.vmManaged.batch.jobStatus.${jobStatusKey}`) }} · {{ t('admin.accounts.vmManaged.batch.progress', progress) }}</p>
        <ul class="max-h-72 space-y-0.5 overflow-y-auto">
          <li v-for="item in job.items" :key="item.id" :class="itemClass(item.status)">
            {{ t('admin.accounts.vmManaged.batch.item', { line: item.line_no, email: item.email || '—', status: t(`admin.accounts.vmManaged.batch.itemStatus.${itemStatusKey(item.status)}`) }) }}<span v-if="item.error_code" class="ml-1">({{ reasonText(item.error_code) }})</span>
          </li>
        </ul>
        <p v-if="!running" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.batch.afterHint') }}</p>
      </section>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <template v-if="!job">
          <button type="button" class="btn btn-secondary" :disabled="busy" @click="close">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="busy || !canPreview" data-testid="batch-preview-button" @click="runPreview">{{ t('admin.accounts.vmManaged.batch.check') }}</button>
          <button type="button" class="btn btn-primary" :disabled="busy || !preview || preview.importable === 0" data-testid="batch-start" @click="start">{{ t('admin.accounts.vmManaged.batch.start', { count: preview?.importable ?? 0 }) }}</button>
        </template>
        <template v-else>
          <button v-if="running" type="button" class="btn btn-danger" :disabled="busy" data-testid="batch-cancel" @click="cancel">{{ t('admin.accounts.vmManaged.batch.cancel') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="busy" data-testid="batch-close" @click="close">{{ t('common.close') }}</button>
        </template>
      </div>
    </template>
  </BaseDialog>
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpBlocked, isStepUpCancelled, StepUpCancelledError, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import {
  openaiVmManagedAPI as api,
  type OpenAIVMBatchInput,
  type OpenAIVMBatchJob,
  type OpenAIVMBatchPreview,
  type OpenAIVMManagedRuntime
} from '@/api/admin/openaiVmManaged'
import type { AdminGroup, Proxy } from '@/types'

const POLL_MS = 3000
const ITEM_STATUSES = ['queued', 'creating', 'logging_in', 'succeeded', 'failed', 'skipped', 'canceled', 'interrupted']
const JOB_STATUSES = ['running', 'finished', 'canceled', 'interrupted']

const props = withDefaults(defineProps<{ show: boolean; proxies?: Proxy[]; groups?: AdminGroup[] }>(), { proxies: () => [], groups: () => [] })
const emit = defineEmits<{ (e: 'close'): void; (e: 'updated'): void }>()
const { t, te } = useI18n()
const stepUp = useStepUp()

const blank = () => ({ text: '', proxyIds: [] as number[], perProxy: 1, runtime: 'kvm' as OpenAIVMManagedRuntime, groupIds: [] as number[], concurrency: 3, priority: 1 })
const form = reactive(blank())
const preview = ref<OpenAIVMBatchPreview | null>(null)
const job = ref<OpenAIVMBatchJob | null>(null)
const busy = ref(false)
const error = ref('')
let session = 0
let timer: ReturnType<typeof setTimeout> | null = null

const socksProxies = computed(() => props.proxies.filter(proxy => (proxy.protocol === 'socks5' || proxy.protocol === 'socks5h') && proxy.status === 'active'))
const openaiGroups = computed(() => props.groups.filter(group => group.platform === 'openai'))
const canPreview = computed(() => form.text.trim() !== '' && Number.isInteger(form.perProxy) && form.perProxy >= 1 && form.perProxy <= 32 &&
  Number.isInteger(form.concurrency) && form.concurrency >= 1 && form.concurrency <= 256)
const running = computed(() => job.value?.status === 'running')
const jobStatusKey = computed(() => (JOB_STATUSES.includes(job.value?.status ?? '') ? job.value!.status : 'running'))
const progress = computed(() => {
  const items = job.value?.items ?? []
  return {
    done: items.filter(item => !['queued', 'creating', 'logging_in'].includes(item.status)).length,
    total: items.length,
    ok: items.filter(item => item.status === 'succeeded').length
  }
})

function input(): OpenAIVMBatchInput {
  return {
    text: form.text,
    proxy_ids: [...form.proxyIds],
    per_proxy: form.perProxy,
    runtime_type: form.runtime,
    group_ids: [...form.groupIds],
    concurrency: form.concurrency,
    priority: Number.isFinite(form.priority) ? form.priority : 1
  }
}

function itemStatusKey(status: string): string {
  return ITEM_STATUSES.includes(status) ? status : 'failed'
}

function itemClass(status: string): string {
  if (status === 'succeeded') return 'text-emerald-700 dark:text-emerald-300'
  if (['failed', 'interrupted'].includes(status)) return 'text-red-700 dark:text-red-300'
  if (['skipped', 'canceled'].includes(status)) return 'text-amber-700 dark:text-amber-300'
  return 'text-gray-700 dark:text-gray-200'
}

function proxyName(id?: number, fresh?: boolean): string {
  if (fresh) return t('admin.accounts.vmManaged.batch.newProxy')
  const proxy = props.proxies.find(item => item.id === id)
  return proxy ? proxy.name : '—'
}

/** A code from the server becomes its own wording when there is one, else it is shown as the code. */
function reasonText(code: string): string {
  const key = `admin.accounts.vmManaged.batch.reason.${code}`
  if (te(key)) return t(key)
  for (const prefix of ['login_', 'slot_']) {
    if (code.startsWith(prefix)) {
      const inner = code.slice(prefix.length)
      for (const namespace of ['attention', 'slotError']) {
        const innerKey = `admin.accounts.vmManaged.${namespace}.${inner}`
        if (te(innerKey)) return t(innerKey)
      }
    }
  }
  return code
}

function messageOf(cause: unknown): string {
  if (isStepUpBlocked(cause)) {
    return t(stepUpBlockReason(cause) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? 'stepUp.adminApiKeyForbidden' : 'stepUp.notEnabled')
  }
  const message = (cause as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : t('admin.accounts.vmManaged.failed')
}

function stopPolling() {
  if (timer) clearTimeout(timer)
  timer = null
}

async function guarded<T>(action: () => Promise<T>): Promise<T | undefined> {
  if (busy.value) return undefined
  const current = session
  busy.value = true
  error.value = ''
  try {
    return await stepUp.run(async () => {
      if (current !== session) throw new StepUpCancelledError()
      return action()
    })
  } catch (cause) {
    if (current === session && !isStepUpCancelled(cause)) error.value = messageOf(cause)
    return undefined
  } finally {
    if (current === session) busy.value = false
  }
}

async function runPreview() {
  if (!canPreview.value) return
  const current = session
  const result = await guarded(() => api.batchPreview(input()))
  if (result && current === session) preview.value = result
}

async function start() {
  if (!preview.value || preview.value.importable === 0) return
  const current = session
  const result = await guarded(() => api.batchStart(input()))
  if (!result || current !== session) return
  // The pasted logins are not needed any more: the server has them in memory only.
  form.text = ''
  preview.value = null
  job.value = result
  schedulePoll(current)
}

function schedulePoll(current: number) {
  stopPolling()
  const tick = async () => {
    if (current !== session || !job.value) return
    try {
      const next = await api.batchStatus(job.value.id)
      if (current !== session) return
      job.value = next
      if (next.status !== 'running') {
        emit('updated')
        return
      }
    } catch {
      // A failed poll is retried; the batch keeps running on the server.
    }
    timer = setTimeout(tick, POLL_MS)
  }
  timer = setTimeout(tick, POLL_MS)
}

async function cancel() {
  if (!job.value || !running.value) return
  const current = session
  const id = job.value.id
  const result = await guarded(() => api.batchCancel(id))
  if (result && current === session) {
    job.value = result
    if (result.status !== 'running') {
      stopPolling()
      emit('updated')
    }
  }
}

function close() {
  if (busy.value) return
  // Closing does not stop a running batch; it keeps going on the server.
  if (job.value) emit('updated')
  emit('close')
}

function reset() {
  session += 1
  stopPolling()
  stepUp.onCancel()
  Object.assign(form, blank())
  preview.value = null
  job.value = null
  busy.value = false
  error.value = ''
}

watch(() => props.show, reset, { immediate: true })
onUnmounted(reset)
</script>
