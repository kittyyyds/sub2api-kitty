<template>
  <BaseDialog :show="show && !loginAccount" :title="w('title')" width="wide" @close="close">
    <VMManagedSteps :current="job ? 5 : step" />
    <p v-if="error" role="alert" class="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>

    <fieldset v-if="!job" :disabled="busy" class="space-y-6">
      <!-- What step 1 decided, always in view -->
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-gray-200 bg-gray-50 px-4 py-2.5 text-sm dark:border-dark-600 dark:bg-dark-700/60" data-testid="vm-settings-summary">
        <span class="text-gray-500 dark:text-gray-400">{{ w('summaryName') }}</span>
        <span class="font-medium text-gray-900 dark:text-white">{{ namesPreview }}</span>
        <span class="text-gray-300 dark:text-dark-500">|</span>
        <span class="text-gray-600 dark:text-gray-300">{{ w('summaryGroups', {n: settings.group_ids?.length || 0}) }}</span>
        <span class="text-gray-600 dark:text-gray-300">{{ w('summaryConcurrency', {n: settings.concurrency}) }}</span>
        <button v-if="step === 2" type="button" class="ml-auto text-sm font-medium text-primary-600 hover:underline dark:text-primary-400" @click="emit('back')">{{ w('editSettings') }}</button>
      </div>

      <div v-if="savedJobId && step === 2" class="flex items-center justify-between gap-3 rounded-lg border border-sky-200 bg-sky-50 px-4 py-2.5 text-sm text-sky-800 dark:border-sky-800 dark:bg-sky-900/20 dark:text-sky-300">
        <span>{{ w('resumeHint') }}</span>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="resume">{{ w('resume') }}</button>
      </div>

      <!-- Step 2: how many slots, where they run, which exits -->
      <template v-if="step === 2">
        <section class="space-y-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ w('sectionSlots') }}</h4>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-[minmax(0,13rem)_1fr]">
            <div>
              <span class="input-label">{{ w('count') }}</span>
              <div class="flex items-stretch overflow-hidden rounded-lg border border-gray-300 dark:border-dark-500">
                <button type="button" class="px-3 text-lg text-gray-600 hover:bg-gray-100 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-600" :disabled="count <= 1" :aria-label="w('decrease')" data-testid="vm-count-minus" @click="count = Math.max(1, count - 1)">−</button>
                <input v-model.number="count" data-testid="vm-count" type="number" min="1" max="20" class="w-full border-x border-gray-300 bg-white px-2 py-2 text-center text-base font-semibold focus:outline-none dark:border-dark-500 dark:bg-dark-800" />
                <button type="button" class="px-3 text-lg text-gray-600 hover:bg-gray-100 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-600" :disabled="count >= 20" :aria-label="w('increase')" data-testid="vm-count-plus" @click="count = Math.min(20, count + 1)">+</button>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ w('countRange') }}</p>
            </div>
            <div>
              <span class="input-label">{{ t('admin.accounts.vmManaged.runtime') }}</span>
              <div class="grid grid-cols-2 gap-2" role="radiogroup">
                <button v-for="kind in ['kvm', 'docker'] as const" :key="kind" type="button" role="radio" :aria-checked="runtime === kind" :data-testid="`vm-runtime-${kind}`" :class="choiceClass(runtime === kind)" @click="runtime = kind">
                  <span class="font-medium">{{ kind === 'kvm' ? 'KVM' : 'Docker' }}</span>
                  <span class="text-xs text-gray-500 dark:text-gray-400">{{ w(kind === 'kvm' ? 'kvmHint' : 'dockerHint') }}</span>
                </button>
              </div>
            </div>
          </div>
          <p v-if="runtime === 'kvm'" class="flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300"><Icon name="infoCircle" size="sm" class="mt-px shrink-0" />{{ w('footprint', {memory: count, disk: count * 20}) }}</p>
        </section>

        <section class="space-y-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ w('sectionProxy') }}</h4>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2" role="radiogroup">
            <button v-for="mode in ['single', 'multiple'] as const" :key="mode" type="button" role="radio" :aria-checked="proxyMode === mode" :data-testid="`vm-proxy-mode-${mode}`" :class="choiceClass(proxyMode === mode)" @click="proxyMode = mode">
              <span class="font-medium">{{ w(mode === 'single' ? 'sameProxy' : 'multipleProxies') }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ w(mode === 'single' ? 'sameProxyHint' : 'multipleProxiesHint') }}</span>
            </button>
          </div>

          <div v-if="usableProxies.length" class="max-h-64 space-y-1.5 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-dark-600" data-testid="vm-proxy-list">
            <label v-for="proxy in usableProxies" :key="proxy.id" :class="['flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm', isPicked(proxy.id) ? 'bg-primary-50 dark:bg-primary-900/20' : 'hover:bg-gray-50 dark:hover:bg-dark-700']">
              <input v-if="proxyMode === 'single'" v-model.number="singleProxy" type="radio" name="vm-single-proxy" :value="proxy.id" :data-testid="`vm-proxy-${proxy.id}`" />
              <input v-else v-model="proxyIds" type="checkbox" :value="proxy.id" :data-testid="`vm-proxy-${proxy.id}`" />
              <span class="min-w-0 flex-1 truncate font-medium text-gray-900 dark:text-white">{{ proxy.name }}</span>
              <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] uppercase text-gray-600 dark:bg-dark-600 dark:text-gray-300">{{ proxy.protocol }}</span>
              <span v-if="proxy.country || proxy.city" class="hidden shrink-0 text-xs text-gray-500 sm:inline dark:text-gray-400">{{ [proxy.country, proxy.city].filter(Boolean).join(' · ') }}</span>
              <span v-if="proxy.account_count != null" class="shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ w('proxyAccounts', {n: proxy.account_count}) }}</span>
            </label>
          </div>
          <p v-else class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400" data-testid="vm-no-proxy">{{ w('noUsableProxy') }}</p>

          <details v-if="unusableProxies.length" class="rounded-lg border border-gray-200 text-sm dark:border-dark-600" data-testid="vm-unusable-proxies">
            <summary class="cursor-pointer select-none px-3 py-2 text-gray-600 dark:text-gray-300">{{ w('unusableSummary', {n: unusableProxies.length}) }}</summary>
            <ul class="space-y-1 border-t border-gray-200 px-3 py-2 dark:border-dark-600">
              <li v-for="item in unusableProxies" :key="item.proxy.id" class="flex flex-wrap items-center gap-x-3 gap-y-0.5 py-0.5">
                <span class="min-w-0 truncate text-gray-500 line-through decoration-gray-300 dark:text-gray-400">{{ item.proxy.name }}</span>
                <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] uppercase text-gray-500 dark:bg-dark-600 dark:text-gray-400">{{ item.proxy.protocol }}</span>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ w(`unusable.${item.reason}`) }}</span>
              </li>
            </ul>
          </details>

          <template v-if="proxyMode === 'multiple'">
            <div class="flex flex-wrap items-center gap-2 text-sm">
              <span class="text-gray-600 dark:text-gray-300">{{ w('proxyOrder') }}</span>
              <button v-for="order in ['ordered', 'random'] as const" :key="order" type="button" :class="pillClass(proxyOrder === order)" @click="proxyOrder = order">{{ w(order === 'ordered' ? 'balanced' : 'balancedRandom') }}</button>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ w('proxyHint') }}</p>
          </template>
        </section>
      </template>

      <!-- Step 3: how the slots log in -->
      <template v-else-if="step === 3">
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2" role="tablist">
          <button v-for="mode in modes" :key="mode" role="tab" type="button" :aria-selected="method === mode" :data-testid="`vm-method-${mode}`" :class="choiceClass(method === mode)" @click="method = mode">
            <span class="flex items-center gap-2 font-medium"><Icon :name="methodIcon[mode]" size="sm" />{{ mode === 'batch' ? w('batch2fa') : t(`admin.accounts.vmManaged.method.${mode}`) }}</span>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ w(`methodHint.${mode}`) }}</span>
          </button>
        </div>

        <template v-if="method === 'batch'">
          <div>
            <div class="mb-1 flex items-center justify-between gap-2">
              <span class="input-label mb-0">{{ w('paste') }}</span>
              <span :class="['rounded-full px-2.5 py-0.5 text-xs font-semibold', selectedRows.length === count ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300']" data-testid="vm-count-summary">{{ w('selectedCount', {selected: selectedRows.length, total: count}) }}</span>
            </div>
            <textarea v-model="raw" data-testid="vm-batch-text" rows="4" class="input font-mono text-xs" autocomplete="off" spellcheck="false" :placeholder="w('pasteHint')" @paste="onPaste" />
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary btn-sm" data-testid="vm-parse" :disabled="!raw.trim() || raw.length > 65536" @click="parse">{{ w('parse') }}</button>
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ w('pasteAutoHint') }}</span>
            </div>
          </div>

          <div v-if="rows.length" class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <div class="hidden grid-cols-[2rem_2rem_minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)] gap-2 bg-gray-50 px-3 py-2 text-xs font-medium text-gray-500 md:grid dark:bg-dark-700 dark:text-gray-400">
              <span /><span>#</span><span>{{ t('admin.accounts.vmManaged.email') }}</span><span>{{ t('admin.accounts.vmManaged.password') }}</span><span>{{ w('mfaColumn') }}</span>
            </div>
            <div v-for="(row, index) in rows" :key="index" :class="['grid grid-cols-[2rem_1fr] items-center gap-2 border-t border-gray-200 px-3 py-2 first:border-t-0 md:grid-cols-[2rem_2rem_minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)] md:first:border-t dark:border-dark-600', !row.selected && 'opacity-50']">
              <input v-model="row.selected" type="checkbox" :aria-label="w('selectRow', {n: index + 1})" />
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ index + 1 }}<span v-if="problemOf(row)" class="ml-2 text-red-600 md:hidden dark:text-red-300">{{ reason(problemOf(row)) }}</span></span>
              <input v-model="row.email" class="input col-span-2 md:col-span-1" type="email" :aria-label="t('admin.accounts.vmManaged.email')" :placeholder="t('admin.accounts.vmManaged.email')" autocomplete="off" />
              <input v-model="row.password" class="input col-span-2 md:col-span-1" type="password" :aria-label="t('admin.accounts.vmManaged.password')" :placeholder="t('admin.accounts.vmManaged.password')" autocomplete="new-password" />
              <input v-model="row.mfa_secret" class="input col-span-2 md:col-span-1" type="password" :aria-label="t('admin.accounts.vmManaged.mfaSecret')" :placeholder="w('mfaColumn')" autocomplete="off" />
              <p v-if="problemOf(row)" class="col-span-2 hidden text-xs text-red-600 md:col-span-5 md:block md:pl-[4.5rem] dark:text-red-300">{{ reason(problemOf(row)) }}</p>
            </div>
          </div>

          <p class="text-xs text-gray-500 dark:text-gray-400">{{ w('countHint') }}</p>
          <div class="flex flex-wrap items-center gap-2 text-sm">
            <span class="text-gray-600 dark:text-gray-300">{{ w('accountOrder') }}</span>
            <button v-for="order in ['ordered', 'random'] as const" :key="order" type="button" :class="pillClass(accountOrder === order)" @click="accountOrder = order">{{ w(order) }}</button>
          </div>
        </template>
        <p v-else class="flex items-start gap-2 rounded-lg bg-gray-50 p-4 text-sm text-gray-700 dark:bg-dark-700 dark:text-gray-300"><Icon name="infoCircle" size="sm" class="mt-px shrink-0 text-gray-400" />{{ w('manualHint', {count}) }}</p>
      </template>

      <!-- Step 4: what will be created -->
      <template v-else-if="step === 4 && preview">
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4" data-testid="vm-review-stats">
          <div v-for="stat in reviewStats" :key="stat.label" class="rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ stat.label }}</p>
            <p class="mt-0.5 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ stat.value }}</p>
          </div>
        </div>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ w('reviewHint') }}</p>
        <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="w-full text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400"><tr><th class="px-3 py-2">#</th><th class="px-3 py-2">{{ w('account') }}</th><th class="px-3 py-2">{{ w('email') }}</th><th class="px-3 py-2">{{ w('proxy') }}</th></tr></thead>
            <tbody>
              <tr v-for="line in preview.lines" :key="line.line_no" class="border-t border-gray-200 dark:border-dark-600">
                <td class="px-3 py-2 text-gray-500 dark:text-gray-400">{{ line.line_no }}</td>
                <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">{{ line.name || line.email }}</td>
                <td class="px-3 py-2">{{ line.email || w('pendingLogin') }}</td>
                <td class="px-3 py-2">{{ proxyName(line.proxy_id) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-for="problem in preview.problems" :key="problem.line_no" class="text-sm text-red-600 dark:text-red-300">{{ w('rowError', {n: problem.line_no, reason: reason(problem.code)}) }}</p>
        <label v-if="mixedRisk" class="flex items-start gap-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300"><input v-model="confirmMixed" type="checkbox" class="mt-0.5" />{{ w('mixedConfirm') }}</label>
      </template>
    </fieldset>

    <!-- After confirming: progress of the background job -->
    <section v-else class="space-y-4" data-testid="vm-job">
      <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
        <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span class="font-medium text-gray-900 dark:text-white">{{ w('jobSummary', {id: job.id, done: doneCount, total: job.total}) }}</span>
          <span :class="['rounded-full px-2.5 py-0.5 text-xs font-semibold', job.status === 'running' ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300' : 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-300']">{{ t(`admin.accounts.vmManaged.batch.jobStatus.${job.status}`) }}</span>
        </div>
        <div class="mt-3 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600"><div class="h-full rounded-full bg-primary-500 transition-all" :style="{width: `${job.total ? Math.round(doneCount / job.total * 100) : 0}%`}" /></div>
        <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
          <span>{{ w('tallySucceeded', {n: tally.succeeded}) }}</span>
          <span>{{ w('tallyWaiting', {n: tally.waiting}) }}</span>
          <span :class="tally.failed ? 'text-red-600 dark:text-red-300' : ''">{{ w('tallyFailed', {n: tally.failed}) }}</span>
        </div>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ w('progressHint') }}</p>
      <ul class="divide-y divide-gray-200 overflow-hidden rounded-lg border border-gray-200 dark:divide-dark-600 dark:border-dark-600">
        <li v-for="item in job.items" :key="item.id" class="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5 text-sm">
          <div class="flex min-w-0 items-center gap-3">
            <span :class="['h-2 w-2 shrink-0 rounded-full', dotClass(item.status)]" />
            <div class="min-w-0">
              <p class="truncate font-medium text-gray-900 dark:text-white">{{ itemName(item) }}</p>
              <p class="break-words text-xs text-gray-500 dark:text-gray-400">{{ item.email ? `${item.email} · ` : '' }}{{ proxyName(item.proxy_id) }} · {{ statusText(item.status) }}<span v-if="item.error_code" class="text-red-600 dark:text-red-300"> · {{ reason(item.error_code) }}</span></p>
            </div>
          </div>
          <button v-if="item.account_id" type="button" class="btn btn-secondary btn-sm" :disabled="['creating','logging_in'].includes(item.status)" @click="openLogin(item)">{{ w('manageLogin') }}</button>
        </li>
      </ul>
    </section>

    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <button v-if="!job" type="button" class="btn btn-secondary" :disabled="busy" @click="back">{{ t('common.back') }}</button>
        <button v-else-if="job.status !== 'running'" type="button" class="btn btn-secondary" @click="newBatch">{{ w('newBatch') }}</button>
        <div class="ml-auto flex gap-2">
          <button type="button" class="btn btn-secondary" :disabled="busy" @click="close">{{ t('common.close') }}</button>
          <button v-if="job?.status === 'running'" type="button" class="btn btn-danger" :disabled="busy" @click="cancel">{{ t('admin.accounts.vmManaged.batch.cancel') }}</button>
          <button v-if="!job && step === 2" type="button" class="btn btn-primary" data-testid="vm-next" :disabled="!resourcesValid" @click="step = 3">{{ t('common.next') }}</button>
          <button v-if="!job && step === 3" type="button" class="btn btn-primary" data-testid="vm-preview" :disabled="busy || !loginValid" @click="makePreview">{{ w('preview') }}</button>
          <button v-if="!job && step === 4" type="button" class="btn btn-primary" data-testid="vm-start" :disabled="busy || !preview || preview.importable !== count || preview.problems.length > 0 || (mixedRisk && !confirmMixed)" @click="start">{{ w('start', {count}) }}</button>
        </div>
      </div>
    </template>
  </BaseDialog>
  <OpenAIVMManagedModal :show="!!loginAccount" :account="loginAccount" :proxies="proxies" :initial-method="method === 'batch' ? 'password' : method" @close="loginAccount = null; pollOnce()" @updated="emit('updated'); pollOnce()" />
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import OpenAIVMManagedModal from './OpenAIVMManagedModal.vue'
import VMManagedSteps from './VMManagedSteps.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import { openaiVmManagedAPI as api, type OpenAIVMBatchInput, type OpenAIVMBatchJob, type OpenAIVMBatchItem, type OpenAIVMBatchPreview, type OpenAIVMManagedSettings, type OpenAIVMManagedRuntime } from '@/api/admin/openaiVmManaged'
import type { Account, Proxy } from '@/types'
import { useAuthStore } from '@/stores/auth'
import { parseVMLogins, proxyUnusableReason, shuffleVMItems, validateVMLogin, vmAccountName, type ParsedVMLogin } from './vmManagedWizard'

const props = defineProps<{ show: boolean; settings: OpenAIVMManagedSettings; namePrefix: string; proxies: Proxy[] }>()
const emit = defineEmits<{ close: []; back: []; updated: [] }>()
const { t, te } = useI18n()
const w = (key: string, params: Record<string, unknown> = {}) => t(`admin.accounts.vmManaged.wizard.${key}`, params)
const stepUp = useStepUp()
const auth = useAuthStore()
const storageKey = computed(() => `vm-managed-last-batch:${auth.user?.id ?? 'admin'}`)
const savedJobId = ref(Number(localStorage.getItem(storageKey.value)) || 0)
const step = ref<2 | 3 | 4>(2), count = ref(1), runtime = ref<OpenAIVMManagedRuntime>('kvm')
const proxyMode = ref<'single' | 'multiple'>('single'), singleProxy = ref(0), proxyIds = ref<number[]>([]), proxyOrder = ref<'ordered' | 'random'>('ordered'), accountOrder = ref<'ordered' | 'random'>('ordered')
const modes = ['batch', 'password', 'oauth', 'file'] as const
const methodIcon = { batch: 'clipboard', password: 'key', oauth: 'link', file: 'document' } as const
const method = ref<typeof modes[number]>('batch')
const raw = ref(''), rows = ref<ParsedVMLogin[]>([]), busy = ref(false), error = ref('')
const preview = ref<OpenAIVMBatchPreview | null>(null), request = ref<OpenAIVMBatchInput | null>(null), job = ref<OpenAIVMBatchJob | null>(null)
const loginAccount = ref<Account | null>(null), mixedRisk = ref(false), confirmMixed = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let generation = 0
const usableProxies = computed(() => props.proxies.filter(p => !proxyUnusableReason(p)))
const unusableProxies = computed(() => props.proxies.map(proxy => ({ proxy, reason: proxyUnusableReason(proxy) })).filter((item): item is { proxy: Proxy; reason: NonNullable<ReturnType<typeof proxyUnusableReason>> } => !!item.reason))
const selectedProxies = computed(() => proxyMode.value === 'single' ? [singleProxy.value].filter(Boolean) : proxyIds.value)
const resourcesValid = computed(() => Number.isInteger(count.value) && count.value >= 1 && count.value <= 20 && selectedProxies.value.length > 0 && selectedProxies.value.length <= 32 && selectedProxies.value.every(id => usableProxies.value.some(p => p.id === id)))
const selectedRows = computed(() => rows.value.filter(r => r.selected))
function problemOf(row: ParsedVMLogin) { const p = validateVMLogin(row); if(p) return p; return selectedRows.value.filter(r => r.email.trim().toLowerCase() === row.email.trim().toLowerCase()).length > 1 ? 'duplicate_in_batch' : '' }
const loginValid = computed(() => resourcesValid.value && (method.value !== 'batch' || (selectedRows.value.length === count.value && selectedRows.value.every(row => !problemOf(row)))))
const doneCount = computed(() => job.value?.items.filter(i => !['queued', 'creating', 'logging_in'].includes(i.status)).length || 0)
const tally = computed(() => {
  const items = job.value?.items || []
  return { succeeded: items.filter(i => i.status === 'succeeded').length, waiting: items.filter(i => i.status === 'awaiting_login').length, failed: items.filter(i => ['failed', 'interrupted'].includes(i.status)).length }
})
const namesPreview = computed(() => count.value > 1 ? `${vmAccountName(props.namePrefix, 1, count.value)} … ${vmAccountName(props.namePrefix, count.value, count.value)}` : vmAccountName(props.namePrefix, 1, 1))
const reviewStats = computed(() => [
  { label: w('statCount'), value: String(preview.value?.lines.length ?? count.value) },
  { label: w('statRuntime'), value: runtime.value === 'kvm' ? 'KVM' : 'Docker' },
  { label: w('statProxies'), value: String(new Set(preview.value?.lines.map(l => l.proxy_id).filter(Boolean)).size) },
  { label: w('statLogin'), value: method.value === 'batch' ? w('batch2fa') : t(`admin.accounts.vmManaged.method.${method.value}`) }
])
function choiceClass(active: boolean) { return ['flex flex-col items-start gap-0.5 rounded-lg border-2 px-3 py-2.5 text-left text-sm transition-colors', active ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'] }
function pillClass(active: boolean) { return ['rounded-full border px-3 py-1 text-xs transition-colors', active ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-300'] }
function dotClass(status: string) { return status === 'succeeded' ? 'bg-emerald-500' : ['failed', 'interrupted'].includes(status) ? 'bg-red-500' : status === 'awaiting_login' ? 'bg-amber-400' : ['creating', 'logging_in'].includes(status) ? 'animate-pulse bg-sky-500' : 'bg-gray-300 dark:bg-dark-500' }
function isPicked(id: number) { return proxyMode.value === 'single' ? singleProxy.value === id : proxyIds.value.includes(id) }
function itemName(item: OpenAIVMBatchItem) { return vmAccountName(props.namePrefix, item.line_no, job.value?.total || 1) || item.email }
function parse() { rows.value = parseVMLogins(raw.value); raw.value = ''; error.value = '' }
function onPaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text/plain')
  if (!text) return
  event.preventDefault()
  raw.value = text
  if (text.length <= 65536) parse()
}
function proxyName(id?: number | null) { return props.proxies.find(p => p.id === id)?.name || '—' }
function reason(code: string) { const key = `admin.accounts.vmManaged.batch.reason.${code}`; return te(key) ? t(key) : code }
function statusText(status: string) { return status === 'awaiting_login' ? w('pendingLogin') : t(`admin.accounts.vmManaged.batch.itemStatus.${status}`) }
function clearSecrets() { raw.value = ''; for(const row of rows.value) { row.password = ''; row.mfa_secret = '' } rows.value = []; if(request.value) request.value.entries = []; request.value = null }
async function guarded<T>(fn: () => Promise<T>): Promise<T | undefined> {
  if (busy.value) return
  const g = generation; busy.value = true; error.value = ''
  try { return await stepUp.run(fn) } catch(e: any) {
    if(g === generation && !isStepUpCancelled(e)) {
      if((e?.status ?? e?.response?.status) === 409 && [e?.code, e?.error, e?.response?.data?.error].includes('mixed_channel_warning')) mixedRisk.value = true
      error.value = e?.message || w('failed')
    }
  } finally { if(g === generation) busy.value = false }
}
async function makePreview() {
  if(!loginValid.value) return
  const entries = selectedRows.value.map(({email,password,mfa_secret}) => ({email:email.trim(),password,mfa_secret:mfa_secret.trim()}))
  request.value = {text:'', count:count.value, login_mode:method.value === 'batch' ? 'batch_2fa' : 'manual', name_prefix:props.namePrefix, proxy_ids:proxyMode.value === 'multiple' && proxyOrder.value === 'random' ? shuffleVMItems(selectedProxies.value) : [...selectedProxies.value], per_proxy:32, runtime_type:runtime.value, settings:JSON.parse(JSON.stringify(props.settings)), entries:method.value === 'batch' ? (accountOrder.value === 'random' ? shuffleVMItems(entries) : entries) : []}
  const result = await guarded(() => api.batchPreview(request.value!))
  if(result) { preview.value = result; step.value = 4 }
}
async function start() {
  if(!request.value || !preview.value) return
  const result = await guarded(() => api.batchStart({...request.value!, plan_token:preview.value!.plan_token, confirm_mixed_channel_risk:confirmMixed.value}))
  if(result) { job.value = result; savedJobId.value = result.id; localStorage.setItem(storageKey.value,String(result.id)); clearSecrets(); emit('updated'); schedule() }
}
async function pollOnce() {
  if(!job.value || !props.show) return
  const g = generation
  try { const next = await api.batchStatus(job.value.id); if(g === generation) { const changed = next.status !== job.value?.status; job.value = next; if(changed) emit('updated') } } catch { /* The next poll retries; no creation is repeated. */ }
}
function schedule() { clearTimeout(timer); if(!props.show || !job.value || (job.value.status !== 'running' && !job.value.items.some(item => item.status === 'awaiting_login'))) return; timer=setTimeout(async()=>{await pollOnce();schedule()},3000) }
async function resume() { const result=await guarded(()=>api.batchStatus(savedJobId.value)); if(result){job.value=result;schedule()} }
async function cancel() { if(!job.value)return;const result=await guarded(()=>api.batchCancel(job.value!.id));if(result){job.value=result;emit('updated')} }
function openLogin(item:OpenAIVMBatchItem) { if(!item.account_id)return;loginAccount.value={id:item.account_id,name:itemName(item),platform:'openai',type:'oauth',extra:{openai_vm_managed:true}} as unknown as Account }
function back() { error.value=''; if(step.value === 2) emit('back'); else {step.value = step.value === 4 ? 3 : 2;preview.value=null;request.value=null} }
function close() { if(busy.value)return;clearSecrets();emit('close') }
function newBatch() { clearSecrets();job.value=null;preview.value=null;request.value=null;step.value=2;mixedRisk.value=false;confirmMixed.value=false;savedJobId.value=0;localStorage.removeItem(storageKey.value);clearTimeout(timer) }
watch(proxyMode, mode => { if(mode === 'multiple' && singleProxy.value && !proxyIds.value.length) proxyIds.value = [singleProxy.value]; if(mode === 'single' && !singleProxy.value && proxyIds.value.length) singleProxy.value = proxyIds.value[0] })
watch(()=>props.show,show=>{generation++;busy.value=false;if(show){savedJobId.value=Number(localStorage.getItem(storageKey.value))||0;if(job.value)schedule()}else{clearTimeout(timer);clearSecrets();loginAccount.value=null;if(!job.value){step.value=2;preview.value=null;mixedRisk.value=false;confirmMixed.value=false;error.value=''}}})
onUnmounted(()=>{generation++;clearTimeout(timer);clearSecrets()})
</script>
