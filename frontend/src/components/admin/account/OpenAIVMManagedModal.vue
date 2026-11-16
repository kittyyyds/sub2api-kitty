<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.vmManaged.title')"
    width="wide"
    :close-on-escape="!busy"
    :show-close-button="!busy"
    @close="close"
  >
    <div class="space-y-5" :aria-busy="busy">
      <div class="rounded-lg bg-sky-50 p-3 text-sm text-sky-900 dark:bg-sky-900/20 dark:text-sky-200">
        <p class="font-medium">{{ account?.name }}</p>
        <p class="mt-1">{{ t('admin.accounts.vmManaged.description') }}</p>
      </div>

      <p v-if="operation" role="status" class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t('admin.accounts.vmManaged.working') }}
      </p>
      <p v-if="error" role="alert" class="whitespace-pre-wrap break-words rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" data-testid="managed-error">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-emerald-50 p-3 text-sm text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300" data-testid="managed-notice">{{ notice }}</p>

      <!-- Status -->
      <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
        <div class="flex items-center justify-between gap-3">
          <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.statusTitle') }}</h4>
          <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-refresh" @click="refresh">{{ t('common.refresh') }}</button>
        </div>
        <p v-if="loadError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
        <template v-if="view">
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('common.status') }}</dt>
            <dd :class="statusClass" data-testid="managed-status">{{ statusLabel }}</dd>
            <template v-if="restrictionLine">
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.restriction') }}</dt>
              <dd class="text-amber-700 dark:text-amber-300" data-testid="managed-restriction">{{ restrictionLine }}</dd>
            </template>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.slotId') }}</dt>
            <dd class="break-all font-mono text-gray-900 dark:text-white">{{ view.slot_id || '—' }}</dd>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.runtime') }}</dt>
            <dd class="text-gray-900 dark:text-white">{{ view.runtime_type === 'kvm' ? 'KVM' : 'Docker' }}</dd>
            <template v-if="snapshot?.email">
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.email') }}</dt>
              <dd class="break-all text-gray-900 dark:text-white">{{ snapshot.email }}</dd>
            </template>
            <template v-if="snapshot?.plan_type">
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.plan') }}</dt>
              <dd class="text-gray-900 dark:text-white">{{ snapshot.plan_type }}<span v-if="snapshot.subscription_end" class="ml-2 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.subscriptionEnd', { time: formatTime(snapshot.subscription_end) }) }}</span></dd>
            </template>
            <template v-if="snapshot?.expires_at">
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.tokenExpires') }}</dt>
              <dd class="text-gray-900 dark:text-white">{{ formatTime(snapshot.expires_at) }}</dd>
            </template>
            <template v-if="view.synced_at">
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.syncedAt') }}</dt>
              <dd class="text-gray-900 dark:text-white">{{ formatTime(view.synced_at) }}</dd>
            </template>
          </dl>
          <p v-if="view.last_error" role="alert" class="whitespace-pre-wrap break-words rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ view.last_error }}</p>
          <p v-if="attentionLine" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200" data-testid="managed-attention">{{ attentionLine }}</p>
          <div v-if="canRetrySlot" class="flex flex-wrap gap-2">
            <button type="button" :disabled="busy" class="btn btn-primary btn-sm" data-testid="managed-retry" @click="retrySlot">{{ t(deleteRetry ? 'admin.accounts.vmManaged.retryDelete' : 'admin.accounts.vmManaged.retrySlot') }}</button>
          </div>
        </template>
      </section>

      <template v-if="view && slotUsable">
        <!-- Usage -->
        <section v-if="view.activated_at" class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between gap-3">
            <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.usageTitle') }}</h4>
            <button type="button" :disabled="busy || !ready" class="btn btn-secondary btn-sm" data-testid="managed-usage-refresh" @click="simple('usage', () => api.refreshUsage(accountId()), 'admin.accounts.vmManaged.usageRefreshed')">{{ t('admin.accounts.vmManaged.refreshUsage') }}</button>
          </div>
          <dl v-if="usageWindows.length" class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm" data-testid="managed-usage">
            <template v-for="item in usageWindows" :key="item.id">
              <dt class="text-gray-500 dark:text-gray-400">{{ item.id }}</dt>
              <dd class="text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.usedPercent', { percent: item.percent }) }}<span v-if="item.reset" class="ml-2 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.resetsAt', { time: item.reset }) }}</span></dd>
            </template>
          </dl>
          <p v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.noUsage') }}</p>
          <div class="flex flex-wrap items-center gap-3 text-sm">
            <span class="text-gray-600 dark:text-gray-300" data-testid="managed-reset-count">{{ t('admin.accounts.vmManaged.resetCredits', { count: resetCredits }) }}</span>
            <button v-if="resetCredits > 0 && !confirmReset" type="button" :disabled="busy || !ready" class="btn btn-secondary btn-sm" data-testid="managed-reset" @click="confirmReset = true">{{ t('admin.accounts.vmManaged.useResetCredit') }}</button>
          </div>
          <div v-if="confirmReset" class="space-y-3 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-900 dark:bg-amber-900/20">
            <p class="text-sm text-amber-800 dark:text-amber-200">{{ t('admin.accounts.vmManaged.resetConfirm') }}</p>
            <div class="flex gap-2">
              <button type="button" :disabled="busy" class="btn btn-danger btn-sm" data-testid="managed-reset-confirm" @click="spendResetCredit">{{ t('admin.accounts.vmManaged.useResetCredit') }}</button>
              <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" @click="confirmReset = false">{{ t('common.cancel') }}</button>
            </div>
          </div>
        </section>

        <!-- Login -->
        <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.loginTitle') }}</h4>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.loginHint') }}</p>

          <div v-if="saved" class="space-y-2 rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700" data-testid="managed-saved">
            <p class="text-gray-700 dark:text-gray-200">{{ t(saved.has_mfa_secret ? 'admin.accounts.vmManaged.savedWithMfa' : 'admin.accounts.vmManaged.savedWithoutMfa', { email: saved.email }) }}</p>
            <div class="flex flex-wrap gap-2">
              <button type="button" :disabled="busy || jobActive" class="btn btn-primary btn-sm" data-testid="managed-relogin-saved" @click="startPasswordLogin(true)">{{ t('admin.accounts.vmManaged.reloginSaved') }}</button>
              <button type="button" :disabled="busy || jobActive" class="btn btn-secondary btn-sm" data-testid="managed-forget" @click="forget">{{ t('admin.accounts.vmManaged.forgetSaved') }}</button>
            </div>
          </div>

          <div v-if="job" class="space-y-2 rounded-lg border border-sky-200 bg-sky-50 p-3 text-sm dark:border-sky-900 dark:bg-sky-900/20" data-testid="managed-job">
            <p class="text-sky-900 dark:text-sky-100">{{ jobLabel }}</p>
            <p v-if="job.message" class="text-sky-800 dark:text-sky-200">{{ job.message }}</p>
            <div v-if="job.status === 'awaiting_code'" class="flex flex-wrap items-end gap-2">
              <div>
                <label for="managed-code" class="input-label">{{ t('admin.accounts.vmManaged.code') }}</label>
                <input id="managed-code" v-model="code" class="input" inputmode="numeric" autocomplete="one-time-code" :disabled="busy" />
              </div>
              <button type="button" :disabled="busy || !code.trim()" class="btn btn-primary btn-sm" data-testid="managed-code-submit" @click="submitCode">{{ t('admin.accounts.vmManaged.submitCode') }}</button>
            </div>
            <button v-if="jobActive" type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-job-cancel" @click="cancelJob">{{ t('admin.accounts.vmManaged.cancelLogin') }}</button>
          </div>

          <div class="flex flex-wrap gap-2" role="tablist">
            <button v-for="method in loginMethods" :key="method" type="button" role="tab" :aria-selected="loginMethod === method" :disabled="busy" :class="['btn btn-sm', loginMethod === method ? 'btn-primary' : 'btn-secondary']" :data-testid="`managed-method-${method}`" @click="loginMethod = method">{{ t(`admin.accounts.vmManaged.method.${method}`) }}</button>
          </div>

          <form v-if="loginMethod === 'password'" class="space-y-3" autocomplete="off" @submit.prevent="startPasswordLogin(false)">
            <div>
              <label for="managed-email" class="input-label">{{ t('admin.accounts.vmManaged.email') }}</label>
              <input id="managed-email" v-model="form.email" type="email" class="input" autocomplete="off" :disabled="busy || jobActive" />
            </div>
            <div>
              <label for="managed-password" class="input-label">{{ t('admin.accounts.vmManaged.password') }}</label>
              <input id="managed-password" v-model="form.password" type="password" class="input" autocomplete="new-password" :disabled="busy || jobActive" />
            </div>
            <div>
              <label for="managed-mfa" class="input-label">{{ t('admin.accounts.vmManaged.mfaSecret') }}</label>
              <input id="managed-mfa" v-model="form.mfaSecret" type="password" class="input" autocomplete="off" :disabled="busy || jobActive" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.mfaHint') }}</p>
            </div>
            <button type="submit" :disabled="busy || jobActive || !form.email.trim() || !form.password" class="btn btn-primary" data-testid="managed-password-submit">{{ t('admin.accounts.vmManaged.startLogin') }}</button>
          </form>

          <div v-else-if="loginMethod === 'oauth'" class="space-y-3">
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-oauth-url" @click="getOAuthUrl">{{ t('admin.accounts.vmManaged.oauthGenerate') }}</button>
            <template v-if="oauth.url">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.oauthHint') }}</p>
              <textarea readonly class="input h-20 font-mono text-xs" :value="oauth.url" data-testid="managed-oauth-link" @focus="selectAll" />
              <div>
                <label for="managed-oauth-callback" class="input-label">{{ t('admin.accounts.vmManaged.oauthCallback') }}</label>
                <input id="managed-oauth-callback" v-model="oauth.callback" class="input" autocomplete="off" :disabled="busy" />
              </div>
              <button type="button" :disabled="busy || !oauth.callback.trim()" class="btn btn-primary" data-testid="managed-oauth-submit" @click="finishOAuth">{{ t('admin.accounts.vmManaged.oauthFinish') }}</button>
            </template>
          </div>

          <div v-else class="space-y-3">
            <label for="managed-file" class="input-label">{{ t('admin.accounts.vmManaged.fileLabel') }}</label>
            <textarea id="managed-file" v-model="fileText" class="input h-28 font-mono text-xs" autocomplete="off" spellcheck="false" :disabled="busy" />
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.fileHint') }}</p>
            <button type="button" :disabled="busy || !fileText.trim()" class="btn btn-primary" data-testid="managed-file-submit" @click="importFile">{{ t('admin.accounts.vmManaged.fileImport') }}</button>
          </div>
        </section>

        <!-- Proxy -->
        <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.proxyTitle') }}</h4>
          <p class="text-sm text-gray-600 dark:text-gray-300" data-testid="managed-proxy-current">{{ t('admin.accounts.vmManaged.proxyCurrent', { name: currentProxyName }) }}</p>
          <div class="flex flex-wrap items-end gap-3">
            <div class="min-w-[14rem] flex-1">
              <label for="managed-proxy" class="input-label">{{ t('admin.accounts.vmManaged.proxyChange') }}</label>
              <select id="managed-proxy" v-model.number="proxyChoice" class="input" :disabled="busy">
                <option :value="0">{{ t('admin.accounts.vmManaged.proxySelect') }}</option>
                <option v-for="proxy in socksProxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }} ({{ proxy.host }}:{{ proxy.port }})</option>
              </select>
            </div>
            <button type="button" :disabled="busy || !proxyChoice || proxyChoice === view.proxy_id" class="btn btn-secondary" data-testid="managed-proxy-apply" @click="changeProxy">{{ t('admin.accounts.vmManaged.proxyApply') }}</button>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.proxyHint') }}</p>
          <div class="flex flex-wrap items-center gap-3">
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-exit-probe" @click="probeExit">{{ t('admin.accounts.vmManaged.exitProbe') }}</button>
            <span v-if="exitLine" class="text-sm text-gray-700 dark:text-gray-200" data-testid="managed-exit">{{ exitLine }}</span>
          </div>
        </section>

        <!-- Maintenance -->
        <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.vmManaged.maintenanceTitle') }}</h4>
          <div class="flex flex-wrap gap-2">
            <button v-if="view.activated_at" type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-schedulable" @click="toggleSchedulable">{{ t(account?.schedulable ? 'admin.accounts.vmManaged.pauseScheduling' : 'admin.accounts.vmManaged.resumeScheduling') }}</button>
            <button type="button" :disabled="busy || !ready" class="btn btn-secondary btn-sm" data-testid="managed-token-refresh" @click="simple('token', () => api.refreshToken(accountId()), 'admin.accounts.vmManaged.tokenRefreshed')">{{ t('admin.accounts.vmManaged.refreshToken') }}</button>
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-power-on" @click="powerOn">{{ t('admin.accounts.vmManaged.powerOn') }}</button>
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-power-off" @click="simple('power', () => api.powerOff(accountId()), 'admin.accounts.vmManaged.poweredOff')">{{ t('admin.accounts.vmManaged.powerOff') }}</button>
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-diagnose" @click="runDiagnosis(false)">{{ t('admin.accounts.vmManaged.diagnose') }}</button>
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" data-testid="managed-recover" @click="runDiagnosis(true)">{{ t('admin.accounts.vmManaged.recover') }}</button>
          </div>
          <ul v-if="diagnosis?.checks?.length" class="space-y-1 text-sm" data-testid="managed-diagnosis">
            <li v-for="check in diagnosis.checks" :key="check.id" :class="check.ok ? 'text-emerald-700 dark:text-emerald-300' : 'text-red-700 dark:text-red-300'">
              {{ check.ok ? '✓' : '✗' }} {{ check.label || check.id }}<span v-if="check.detail" class="ml-2 text-gray-500 dark:text-gray-400">{{ check.detail }}</span>
            </li>
          </ul>
        </section>
      </template>

      <!-- Delete -->
      <section v-if="view" class="space-y-3 rounded-xl border border-red-200 p-4 dark:border-red-900">
        <h4 class="font-medium text-red-700 dark:text-red-300">{{ t('admin.accounts.vmManaged.deleteTitle') }}</h4>
        <button v-if="!confirmDelete" type="button" :disabled="busy" class="btn btn-danger btn-sm" data-testid="managed-delete" @click="confirmDelete = true">{{ t('admin.accounts.vmManaged.delete') }}</button>
        <div v-else class="space-y-3">
          <p class="text-sm text-red-700 dark:text-red-300">{{ t('admin.accounts.vmManaged.deleteConfirm') }}</p>
          <div class="flex gap-2">
            <button type="button" :disabled="busy" class="btn btn-danger btn-sm" data-testid="managed-delete-confirm" @click="deleteAccount">{{ t('admin.accounts.vmManaged.delete') }}</button>
            <button type="button" :disabled="busy" class="btn btn-secondary btn-sm" @click="confirmDelete = false">{{ t('common.cancel') }}</button>
          </div>
        </div>
      </section>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="close">{{ t('common.close') }}</button>
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
import { Icon } from '@/components/icons'
import { isStepUpBlocked, isStepUpCancelled, StepUpCancelledError, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import {
  OPENAI_VM_MANAGED_STATUSES,
  openaiVmManagedAPI as api,
  type OpenAIVMManagedDiagnosis,
  type OpenAIVMManagedExitProbe,
  type OpenAIVMManagedLoginJob,
  type OpenAIVMManagedPasswordState,
  type OpenAIVMManagedStatus,
  type OpenAIVMManagedView
} from '@/api/admin/openaiVmManaged'
import { extractApiErrorMetadata } from '@/utils/apiError'
import type { Account, Proxy } from '@/types'

type LoginMethod = 'password' | 'oauth' | 'file'
const loginMethods: LoginMethod[] = ['password', 'oauth', 'file']
// How often a running login is polled, and for how long at most.
const POLL_MS = 2500
const POLL_LIMIT = 300
// A starting slot (a KVM guest boots for a minute or two) is re-read this often, for at most 6 minutes.
const START_POLL_MS = 5000
const START_POLL_LIMIT = 72

const props = withDefaults(defineProps<{ show: boolean; account: Account | null; proxies?: Proxy[]; initialMethod?: LoginMethod }>(), { proxies: () => [], initialMethod: 'password' })
const emit = defineEmits<{ (e: 'close'): void; (e: 'updated'): void; (e: 'deleted'): void }>()
const { t, te } = useI18n()
const stepUp = useStepUp()

const view = ref<OpenAIVMManagedView | null>(null)
const password = ref<OpenAIVMManagedPasswordState | null>(null)
const operation = ref('')
const error = ref('')
const notice = ref('')
const loadError = ref('')
const loginMethod = ref<LoginMethod>('password')
const form = reactive({ email: '', password: '', mfaSecret: '' })
const code = ref('')
const oauth = reactive({ url: '', session: '', callback: '' })
const fileText = ref('')
const proxyChoice = ref(0)
const exit = ref<OpenAIVMManagedExitProbe | null>(null)
const diagnosis = ref<OpenAIVMManagedDiagnosis | null>(null)
const confirmDelete = ref(false)
const confirmReset = ref(false)
let session = 0
let pollTimer: ReturnType<typeof setTimeout> | null = null
let polls = 0
let startTimer: ReturnType<typeof setTimeout> | null = null
let startPolls = 0

const busy = computed(() => operation.value !== '')
const snapshot = computed(() => view.value?.snapshot ?? null)
const ready = computed(() => view.value?.binding_status === 'ready')
// Login and maintenance need an existing slot that is not being removed.
const slotUsable = computed(() => ['ready', 'awaiting_login'].includes(view.value?.binding_status ?? ''))
const deleteRetry = computed(() => ['deleting', 'delete_failed'].includes(view.value?.binding_status ?? ''))
const canRetrySlot = computed(() => ['provisioning', 'error'].includes(view.value?.binding_status ?? '') || deleteRetry.value)
const job = computed<OpenAIVMManagedLoginJob | null>(() => password.value?.job ?? null)
const jobActive = computed(() => ['running', 'awaiting_code', 'committing'].includes(job.value?.status ?? ''))
const saved = computed(() => password.value?.saved ?? snapshot.value?.saved_login ?? null)
const socksProxies = computed(() => props.proxies.filter(proxy => (proxy.protocol === 'socks5' || proxy.protocol === 'socks5h') && proxy.status === 'active'))
const resetCredits = computed(() => Number(snapshot.value?.reset_credits?.available_count ?? 0) || 0)

const statusLabel = computed(() => {
  const status = view.value?.status as OpenAIVMManagedStatus | ''
  return status && OPENAI_VM_MANAGED_STATUSES.includes(status) ? t(`admin.accounts.vmManaged.status.${status}`) : t('admin.accounts.vmManaged.status.unknown')
})
const statusClass = computed(() => {
  switch (view.value?.status) {
    case 'available': return 'text-emerald-600 dark:text-emerald-400'
    case 'needs_attention': case 'needs_relogin': case 'error': case 'slot_error': return 'text-red-600 dark:text-red-400'
    default: return 'text-amber-600 dark:text-amber-400'
  }
})
const restrictionLine = computed(() => {
  if (view.value?.status !== 'restricted') return ''
  const text = snapshot.value?.restriction_text || snapshot.value?.restriction_reason || ''
  const until = snapshot.value?.restriction_until
  return until ? t('admin.accounts.vmManaged.restrictionUntil', { reason: text, time: formatTime(until) }) : text
})
const attentionLine = computed(() => {
  const auto = snapshot.value?.auto_relogin
  if (view.value?.status === 'needs_attention') {
    const reason = auto?.attention_reason || ''
    const key = `admin.accounts.vmManaged.attention.${reason}`
    return t('admin.accounts.vmManaged.attentionLine', { reason: reason && te(key) ? t(key) : reason || t('admin.accounts.vmManaged.attention.unknown') })
  }
  if (view.value?.status === 'needs_relogin') {
    return t(saved.value?.has_mfa_secret ? 'admin.accounts.vmManaged.reloginAutoHint' : 'admin.accounts.vmManaged.reloginManualHint')
  }
  if (view.value?.status === 'relogin_running') return t('admin.accounts.vmManaged.reloginRunningHint')
  if (view.value?.status === 'starting') return t('admin.accounts.vmManaged.startingHint')
  return ''
})
const usageWindows = computed(() => (snapshot.value?.usage?.windows ?? [])
  .filter(item => item.used_percent != null)
  .map(item => ({ id: item.id, percent: Math.round(Number(item.used_percent) * 10) / 10, reset: item.reset_at ? formatTime(item.reset_at) : '' })))
const jobLabel = computed(() => {
  const status = job.value?.status ?? ''
  const key = `admin.accounts.vmManaged.job.${status}`
  return te(key) ? t(key) : status
})
const currentProxyName = computed(() => {
  const proxy = props.proxies.find(item => item.id === view.value?.proxy_id)
  return proxy ? `${proxy.name} (${proxy.host}:${proxy.port})` : snapshot.value?.proxy_label || '—'
})
const exitLine = computed(() => {
  if (!exit.value) return ''
  const place = [exit.value.country || exit.value.country_code, exit.value.region, exit.value.city].filter(Boolean).join(' / ')
  return [exit.value.ip, place].filter(Boolean).join(' · ')
})

function accountId(): number {
  return props.account?.id ?? 0
}

function formatTime(value: string | number): string {
  // An epoch can arrive as a number or as digits in a string, in milliseconds or in seconds.
  let input: string | number = value
  if (typeof input === 'string' && /^\d+$/.test(input.trim())) input = Number(input.trim())
  if (typeof input === 'number' && input > 0 && input < 1e11) input *= 1000
  const date = new Date(input)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

function selectAll(event: FocusEvent) {
  (event.target as HTMLTextAreaElement | null)?.select()
}

function messageOf(cause: unknown): string {
  if (isStepUpBlocked(cause)) {
    return t(stepUpBlockReason(cause) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? 'stepUp.adminApiKeyForbidden' : 'stepUp.notEnabled')
  }
  // The slot's short refusal code, when there is one, has its own wording.
  const slotCode = extractApiErrorMetadata(cause)?.code
  if (typeof slotCode === 'string' && slotCode) {
    const key = `admin.accounts.vmManaged.slotError.${slotCode}`
    return te(key) ? t(key) : t('admin.accounts.vmManaged.slotError.other', { code: slotCode })
  }
  const message = (cause as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : t('admin.accounts.vmManaged.failed')
}

function stopPolling() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function stopStartWatch() {
  if (startTimer) clearTimeout(startTimer)
  startTimer = null
}

/** A starting slot is re-read until it is up; nobody has to click anything. */
function watchStarting(current: number) {
  if (startTimer || current !== session || startPolls >= START_POLL_LIMIT) return
  startTimer = setTimeout(async () => {
    startTimer = null
    if (current !== session || !props.show) return
    startPolls += 1
    try {
      // Re-arms itself while the slot is still starting.
      await readStatus(current)
      if (current === session && view.value?.status !== 'starting') emit('updated')
    } catch {
      watchStarting(current)
    }
  }, START_POLL_MS)
}

/** Passwords and secrets never outlive the request that carries them. */
function clearSecrets() {
  form.password = ''
  form.mfaSecret = ''
  code.value = ''
  fileText.value = ''
  oauth.callback = ''
}

async function readStatus(current: number) {
  const value = await api.getStatus(accountId())
  if (current !== session) return
  view.value = value
  loadError.value = ''
  if (!proxyChoice.value && value.proxy_id) proxyChoice.value = value.proxy_id
  if (value.status === 'starting') watchStarting(current)
  else startPolls = 0
}

async function readPassword(current: number) {
  if (!slotUsable.value) return
  const value = await api.passwordStatus(accountId())
  if (current !== session) return
  password.value = value
  if (!form.email && (value.saved?.email || value.email)) form.email = value.saved?.email || value.email || ''
}

async function load() {
  if (busy.value || !props.show || !props.account) return
  const current = session
  operation.value = 'load'
  try {
    await readStatus(current)
    await readPassword(current)
    if (current === session && jobActive.value) schedulePoll(current)
  } catch (cause) {
    if (current === session) loadError.value = messageOf(cause)
  } finally {
    if (current === session) operation.value = ''
  }
}

/**
 * Runs one action behind the busy flag. Anything that changes the slot goes
 * through the second factor; `sensitive: false` is for plain reads.
 */
async function run(kind: string, action: (current: number) => Promise<void>, sensitive = true) {
  if (busy.value || !props.account || !props.show) return
  const current = session
  operation.value = kind
  notice.value = ''
  try {
    const guarded = async () => {
      if (current !== session) throw new StepUpCancelledError()
      await action(current)
    }
    await (sensitive ? stepUp.run(guarded) : guarded())
    if (current === session) error.value = ''
  } catch (cause) {
    if (current !== session || isStepUpCancelled(cause)) return
    error.value = messageOf(cause)
    try { await readStatus(current) } catch { /* the error above is what matters */ }
  } finally {
    if (current === session) operation.value = ''
  }
}

async function afterChange(current: number, noticeKey?: string) {
  await readStatus(current)
  if (current !== session) return
  if (noticeKey) notice.value = t(noticeKey)
  emit('updated')
}

function refresh() {
  return run('refresh', async current => {
    await readStatus(current)
    await readPassword(current)
  }, false)
}

function simple(kind: string, call: () => Promise<unknown>, noticeKey: string) {
  return run(kind, async current => {
    await call()
    await afterChange(current, noticeKey)
  })
}

function powerOn() {
  return run('power', async current => {
    const result = await api.powerOn(accountId())
    await afterChange(current, result?.starting ? 'admin.accounts.vmManaged.startingNotice' : 'admin.accounts.vmManaged.poweredOn')
  })
}

function retrySlot() {
  if (!canRetrySlot.value) return
  if (deleteRetry.value) return deleteAccountNow()
  return run('retry', async current => {
    await api.retrySlot(accountId())
    await afterChange(current)
  })
}

function schedulePoll(current: number) {
  stopPolling()
  polls = 0
  const tick = async () => {
    if (current !== session || !props.show) return
    polls += 1
    try {
      await readPassword(current)
      if (current !== session) return
      if (!jobActive.value) {
        await readStatus(current)
        if (current !== session) return
        if (job.value?.status === 'succeeded') notice.value = t('admin.accounts.vmManaged.loginSucceeded')
        emit('updated')
        return
      }
    } catch {
      // A failed poll is retried; the job keeps running on the runtime either way.
    }
    if (polls < POLL_LIMIT) pollTimer = setTimeout(tick, POLL_MS)
  }
  pollTimer = setTimeout(tick, POLL_MS)
}

function startPasswordLogin(useSaved: boolean) {
  if (jobActive.value) return
  if (!useSaved && (!form.email.trim() || !form.password)) return
  const input = useSaved
    ? { use_saved: true }
    : { email: form.email.trim(), password: form.password, mfa_mode: form.mfaSecret.trim() ? 'secret' as const : 'code' as const, ...(form.mfaSecret.trim() ? { mfa_secret: form.mfaSecret.trim() } : {}) }
  return run('login', async current => {
    const result = await api.passwordLogin(accountId(), input)
    if (current !== session) return
    clearSecrets()
    password.value = { ...(password.value ?? {}), job: result.job }
    schedulePoll(current)
  })
}

function submitCode() {
  const jobId = job.value?.id
  const value = code.value.trim()
  if (!jobId || !value) return
  return run('code', async current => {
    const result = await api.passwordCode(accountId(), jobId, value)
    if (current !== session) return
    code.value = ''
    password.value = { ...(password.value ?? {}), job: result.job }
    schedulePoll(current)
  })
}

function cancelJob() {
  const jobId = job.value?.id
  if (!jobId) return
  return run('cancel', async current => {
    const result = await api.passwordCancel(accountId(), jobId)
    if (current !== session) return
    stopPolling()
    password.value = { ...(password.value ?? {}), job: result.job }
  })
}

function forget() {
  return run('forget', async current => {
    await api.forgetSavedLogin(accountId())
    if (current !== session) return
    password.value = { ...(password.value ?? {}), saved: null }
    await afterChange(current, 'admin.accounts.vmManaged.forgotten')
  })
}

function getOAuthUrl() {
  return run('oauth-url', async current => {
    const result = await api.oauthUrl(accountId())
    if (current !== session) return
    oauth.url = result.auth_url
    oauth.session = result.session_id
    oauth.callback = ''
  })
}

function finishOAuth() {
  const callback = oauth.callback.trim()
  if (!oauth.session || !callback) return
  // Either the full redirected address or just the code may be pasted.
  const input = /^https?:\/\//i.test(callback) ? { session_id: oauth.session, callback_url: callback } : { session_id: oauth.session, code: callback }
  return run('oauth-code', async current => {
    await api.oauthCode(accountId(), input)
    if (current !== session) return
    oauth.url = ''
    oauth.session = ''
    clearSecrets()
    await afterChange(current, 'admin.accounts.vmManaged.loginSucceeded')
  })
}

function importFile() {
  const raw = fileText.value.trim()
  if (!raw) return
  return run('import', async current => {
    await api.importAccountFile(accountId(), raw)
    if (current !== session) return
    clearSecrets()
    await afterChange(current, 'admin.accounts.vmManaged.loginSucceeded')
  })
}

function changeProxy() {
  const proxyId = proxyChoice.value
  if (!proxyId || proxyId === view.value?.proxy_id) return
  return run('proxy', async current => {
    const value = await api.changeProxy(accountId(), proxyId)
    if (current !== session) return
    view.value = value
    exit.value = null
    notice.value = t('admin.accounts.vmManaged.proxyChanged')
    emit('updated')
  })
}

function probeExit() {
  return run('exit', async current => {
    const value = await api.exitProbe(accountId())
    if (current === session) exit.value = value
  })
}

function runDiagnosis(recover: boolean) {
  return run(recover ? 'recover' : 'diagnose', async current => {
    const value = recover ? await api.recover(accountId()) : await api.diagnose(accountId())
    if (current !== session) return
    diagnosis.value = value
    if (recover) await afterChange(current, value.starting ? 'admin.accounts.vmManaged.startingNotice' : 'admin.accounts.vmManaged.recovered')
  }, recover)
}

function toggleSchedulable() {
  const next = !props.account?.schedulable
  return run('schedulable', async current => {
    const value = await api.setSchedulable(accountId(), next)
    if (current !== session) return
    view.value = value
    notice.value = t(next ? 'admin.accounts.vmManaged.schedulingResumed' : 'admin.accounts.vmManaged.schedulingPaused')
    emit('updated')
  })
}

function spendResetCredit() {
  if (!confirmReset.value) return
  return run('reset', async current => {
    await api.useResetCredit(accountId())
    if (current !== session) return
    confirmReset.value = false
    await afterChange(current, 'admin.accounts.vmManaged.resetDone')
  })
}

function deleteAccountNow() {
  return run('delete', async current => {
    await api.deleteAccount(accountId())
    if (current !== session) return
    confirmDelete.value = false
    emit('deleted')
    emit('updated')
    emit('close')
  })
}

function deleteAccount() {
  if (!confirmDelete.value) return
  return deleteAccountNow()
}

function close() {
  if (!busy.value) emit('close')
}

function reset() {
  session += 1
  stopPolling()
  stopStartWatch()
  startPolls = 0
  stepUp.onCancel()
  operation.value = ''
  view.value = null
  password.value = null
  error.value = ''
  notice.value = ''
  loadError.value = ''
  loginMethod.value = props.initialMethod
  form.email = ''
  clearSecrets()
  oauth.url = ''
  oauth.session = ''
  proxyChoice.value = 0
  exit.value = null
  diagnosis.value = null
  confirmDelete.value = false
  confirmReset.value = false
}

watch([() => props.show, () => props.account?.id], () => {
  reset()
  if (props.show && props.account) void load()
}, { immediate: true })

onUnmounted(reset)
</script>
