<template>
  <section class="space-y-4 rounded-xl border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-800/40" data-testid="cursor-account-fields">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.cursor.title') }}</h3>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.cursor.description') }}</p>
      </div>
      <span class="rounded-full px-2.5 py-1 text-xs" :class="status?.available ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'">
        {{ loadingStatus ? t('admin.accounts.cursor.checking') : status?.available ? t('admin.accounts.cursor.ready') : t('admin.accounts.cursor.unavailable') }}
      </span>
    </div>
    <div v-if="!loadingStatus && !status?.available" class="text-xs leading-5 text-amber-700 dark:text-amber-300" role="status">
      {{ status?.message || t('admin.accounts.cursor.statusFailed') }}
      <button type="button" class="ml-2 underline underline-offset-2" @click="loadStatus">{{ t('admin.accounts.cursor.retry') }}</button>
    </div>
    <ol class="list-decimal space-y-1 pl-5 text-xs leading-5 text-gray-600 dark:text-gray-400">
      <li>{{ t('admin.accounts.cursor.step1') }} <a href="https://cursor.com/dashboard" target="_blank" rel="noopener noreferrer" class="font-medium text-primary-600 underline underline-offset-2 dark:text-primary-400">{{ t('admin.accounts.cursor.openDashboard') }}</a></li>
      <li>{{ t('admin.accounts.cursor.step2') }}</li>
      <li>{{ t('admin.accounts.cursor.step3') }}</li>
    </ol>
    <div>
      <label :for="inputId" class="input-label">{{ t('admin.accounts.cursor.keyLabel') }} <span v-if="!editing" class="text-red-500">*</span></label>
      <div class="flex gap-2">
        <input :id="inputId" :value="modelValue" :type="showKey ? 'text' : 'password'" :required="!editing" autocomplete="new-password" spellcheck="false" class="input min-w-0 flex-1 font-mono" :placeholder="editing ? t('admin.accounts.cursor.keepExisting') : 'crsr_…'" @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)" />
        <button type="button" class="btn btn-secondary shrink-0" :aria-pressed="showKey" @click="showKey = !showKey">{{ showKey ? t('admin.accounts.cursor.hide') : t('admin.accounts.cursor.show') }}</button>
      </div>
      <p class="input-hint">{{ editing ? t('admin.accounts.cursor.editHint') : t('admin.accounts.cursor.keyHint') }}</p>
    </div>
    <div class="flex flex-wrap items-center gap-3">
      <button type="button" class="btn btn-secondary" :disabled="verifying || !status?.available || (!modelValue.trim() && !accountId)" @click="verify">
        {{ verifying ? t('admin.accounts.cursor.verifying') : t('admin.accounts.cursor.verify') }}
      </button>
      <span v-if="verified" class="text-sm text-emerald-700 dark:text-emerald-300" role="status">{{ t('admin.accounts.cursor.verified') }}</span>
    </div>
    <p v-if="error" role="alert" class="text-xs leading-5 text-red-600 dark:text-red-400">{{ error }}</p>
    <p v-if="verified" class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.cursor.verifyScope') }}</p>
    <details class="border-t border-gray-200 pt-3 text-xs dark:border-dark-600">
      <summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.cursor.helpTitle') }}</summary>
      <div class="mt-3 space-y-2 leading-5 text-gray-500 dark:text-gray-400">
        <p>{{ t('admin.accounts.cursor.authHelp') }}</p>
        <p>{{ t('admin.accounts.cursor.downstreamHelp') }}</p>
        <p>{{ t('admin.accounts.cursor.quotaHelp') }}</p>
        <p>{{ t('admin.accounts.cursor.proxyHelp') }}</p>
        <p>{{ t('admin.accounts.cursor.visionHelp') }}</p>
      </div>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiClient } from '@/api/client'

const props = defineProps<{ modelValue: string; editing?: boolean; accountId?: number }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()
const inputId = computed(() => props.editing ? 'edit-cursor-user-key' : 'create-cursor-user-key')
const showKey = ref(false)
const status = ref<{ available: boolean; message: string } | null>(null)
const loadingStatus = ref(false)
const verifying = ref(false)
const verified = ref(false)
const error = ref('')
let verification: AbortController | undefined
let statusRequest: AbortController | undefined

async function loadStatus() {
  statusRequest?.abort()
  const request = new AbortController()
  statusRequest = request
  loadingStatus.value = true
  try {
    const { data } = await apiClient.get('/admin/accounts/cursor/status', { signal: request.signal })
    if (!request.signal.aborted) status.value = data
  } catch {
    if (!request.signal.aborted) status.value = null
  } finally {
    if (statusRequest === request) loadingStatus.value = false
  }
}

function resetVerification() {
  verification?.abort()
  verification = undefined
  verifying.value = false
  verified.value = false
  error.value = ''
}

async function verify() {
  resetVerification()
  const key = props.modelValue.trim()
  if (key && !/^crsr_[A-Za-z0-9_-]{8,512}$/.test(key)) {
    error.value = t('admin.accounts.cursor.invalidKey')
    return
  }
  const request = new AbortController()
  verification = request
  verifying.value = true
  try {
    const { data } = await apiClient.post('/admin/accounts/cursor/verify', {
      ...(key ? { api_key: key } : { account_id: props.accountId })
    }, { signal: request.signal })
    if (!request.signal.aborted) verified.value = data.authenticated === true
  } catch (cause) {
    if (!request.signal.aborted) {
      error.value = (cause as { response?: { data?: { message?: string } }; message?: string }).response?.data?.message || t('admin.accounts.cursor.verifyFailed')
    }
  } finally {
    if (verification === request) verifying.value = false
  }
}

watch(() => [props.modelValue, props.accountId], resetVerification)
onMounted(loadStatus)
onBeforeUnmount(() => { verification?.abort(); statusRequest?.abort() })
</script>
