<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-6 pb-8">
      <header class="page-header rounded-3xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6">
        <h1 class="page-title flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
          <span class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300">
            <Icon name="chart" size="sm" />
          </span>
          {{ t('modelCapability.title') }}
        </h1>
        <p class="page-description mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('modelCapability.description') }}</p>
      </header>

      <section class="rounded-2xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6">
        <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('modelCapability.account') }}</span>
            <select v-model="selectedAccountId" class="input w-full" :disabled="loadingAccounts || running">
              <option :value="null">{{ loadingAccounts ? t('common.loading') : t('modelCapability.accountPlaceholder') }}</option>
              <option v-for="account in compatibleAccounts" :key="account.id" :value="account.id">
                {{ account.name }} · {{ account.platform }} · #{{ account.id }}
              </option>
            </select>
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('modelCapability.model') }}</span>
            <input v-model="modelId" class="input w-full" :placeholder="t('modelCapability.modelPlaceholder')" :disabled="running" />
          </label>
        </div>

        <fieldset class="mt-5">
          <legend class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('modelCapability.protocols') }}</legend>
          <div class="grid gap-2 sm:grid-cols-3">
            <label v-for="probe in probeOptions" :key="probe.value" class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm text-gray-700 transition hover:border-blue-400 dark:border-dark-700 dark:text-gray-200">
              <input v-model="selectedProtocols" type="checkbox" :value="probe.value" :disabled="running" class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500" />
              <span>{{ t(probe.label) }}</span>
            </label>
          </div>
        </fieldset>

        <div class="mt-5 flex flex-wrap items-center gap-3">
          <button type="button" class="btn btn-primary" :disabled="running || loadingAccounts" @click="runProbe">
            <Icon name="chart" size="sm" />
            {{ running ? t('modelCapability.running') : t('modelCapability.run') }}
          </button>
          <p v-if="errorMessage" class="text-sm text-red-600 dark:text-red-400">{{ errorMessage }}</p>
        </div>
      </section>

      <section v-if="result" class="space-y-4">
        <div class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
          {{ result.disclaimer }}
        </div>
        <div class="flex flex-wrap gap-x-6 gap-y-1 text-sm text-gray-600 dark:text-gray-300">
          <span>{{ t('modelCapability.requestedModel') }}: <strong class="text-gray-900 dark:text-white">{{ result.requested_model }}</strong></span>
          <span>{{ t('modelCapability.upstreamModel') }}: <strong class="text-gray-900 dark:text-white">{{ result.upstream_model || '-' }}</strong></span>
        </div>
        <div class="overflow-x-auto rounded-2xl bg-white shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-900/50 dark:text-gray-400">
              <tr>
                <th class="px-4 py-3">{{ t('modelCapability.protocols') }}</th>
                <th class="px-4 py-3">{{ t('modelCapability.status') }}</th>
                <th class="px-4 py-3">{{ t('modelCapability.latency') }}</th>
                <th class="px-4 py-3">{{ t('modelCapability.httpStatus') }}</th>
                <th class="px-4 py-3">{{ t('modelCapability.tool') }}</th>
                <th class="px-4 py-3">{{ t('modelCapability.evidence') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in result.results" :key="item.protocol">
                <td class="whitespace-nowrap px-4 py-3 font-medium text-gray-900 dark:text-white">{{ protocolLabel(item.protocol) }}</td>
                <td class="px-4 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span>
                  <p v-if="item.error" class="mt-1 max-w-sm text-xs text-gray-500 dark:text-gray-400">{{ item.error }}</p>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-gray-600 dark:text-gray-300">{{ item.latency_ms }} ms</td>
                <td class="whitespace-nowrap px-4 py-3 text-gray-600 dark:text-gray-300">{{ item.http_status || '-' }}</td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                  <template v-if="item.protocol === 'responses_tools'">{{ item.tool_call_found ? t('modelCapability.yes') : t('modelCapability.no') }} / {{ item.tool_arguments_valid ? t('modelCapability.valid') : t('modelCapability.invalid') }}</template>
                  <span v-else>-</span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400">{{ item.evidence }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section v-else class="rounded-2xl border border-dashed border-gray-300 px-6 py-12 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
        {{ t('modelCapability.empty') }}
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { Account } from '@/types'
import type { ModelCapabilityProbeProtocol, ModelCapabilityProbeResponse } from '@/api/admin/accounts'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const accounts = ref<Account[]>([])
const selectedAccountId = ref<number | null>(null)
const modelId = ref('')
const selectedProtocols = ref<ModelCapabilityProbeProtocol[]>(['chat_completions', 'responses', 'responses_tools'])
const loadingAccounts = ref(false)
const running = ref(false)
const errorMessage = ref('')
const result = ref<ModelCapabilityProbeResponse | null>(null)

const compatibleAccounts = computed(() => accounts.value.filter((account) => {
  const openAI = account.platform === 'openai'
  const cn = ['kimi', 'deepseek', 'zhipu'].includes(String(account.platform))
  return account.type === 'apikey' && (openAI || cn)
}))

const probeOptions = [
  { value: 'chat_completions' as const, label: 'modelCapability.chatText' },
  { value: 'responses' as const, label: 'modelCapability.responsesText' },
  { value: 'responses_tools' as const, label: 'modelCapability.responsesTools' },
]

function protocolLabel(protocol: string): string {
  const labels: Record<string, string> = {
    chat_completions: t('modelCapability.chatText'),
    responses: t('modelCapability.responsesText'),
    responses_tools: t('modelCapability.responsesTools'),
  }
  return labels[protocol] || protocol
}

function statusLabel(status: string): string {
  const labels: Record<string, string> = {
    verified: t('modelCapability.verified'),
    model_mismatch: t('modelCapability.modelMismatch'),
    unsupported: t('modelCapability.unsupported'),
    timeout: t('modelCapability.timeout'),
    error: t('modelCapability.error'),
    needs_retest: t('modelCapability.needsRetest'),
  }
  return labels[status] || status
}

function statusClass(status: string): string {
  if (status === 'verified') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  if (status === 'model_mismatch') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
  if (status === 'unsupported') return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
}

async function loadAccounts() {
  loadingAccounts.value = true
  try {
    const page = await adminAPI.accounts.list(1, 100, { status: 'active' })
    accounts.value = page.items
    if (!selectedAccountId.value && compatibleAccounts.value.length > 0) selectedAccountId.value = compatibleAccounts.value[0].id
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error)
  } finally {
    loadingAccounts.value = false
  }
}

async function runProbe() {
  errorMessage.value = ''
  result.value = null
  if (!selectedAccountId.value) {
    errorMessage.value = t('modelCapability.selectAccount')
    return
  }
  if (selectedProtocols.value.length === 0) {
    errorMessage.value = t('modelCapability.selectProtocol')
    return
  }
  running.value = true
  try {
    result.value = await adminAPI.accounts.probeModelCapabilities(selectedAccountId.value, {
      model_id: modelId.value.trim() || undefined,
      protocols: selectedProtocols.value,
    })
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error)
  } finally {
    running.value = false
  }
}

onMounted(loadAccounts)
</script>
