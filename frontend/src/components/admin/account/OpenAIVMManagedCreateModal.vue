<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.vmManaged.create.title')"
    width="normal"
    :close-on-escape="!busy"
    :show-close-button="!busy"
    @close="close"
  >
    <form id="managed-create-form" class="space-y-4" :aria-busy="busy" @submit.prevent="submit">
      <p class="rounded-lg bg-sky-50 p-3 text-sm text-sky-900 dark:bg-sky-900/20 dark:text-sky-200">
        {{ t('admin.accounts.vmManaged.create.intro') }}
        <button type="button" class="ml-1 font-medium text-primary-600 hover:underline disabled:opacity-50" :disabled="busy" data-testid="managed-create-batch" @click="emit('batch')">{{ t('admin.accounts.vmManaged.batch.entry') }}</button>
      </p>
      <p v-if="error" role="alert" class="whitespace-pre-wrap break-words rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" data-testid="managed-create-error">{{ error }}</p>

      <div>
        <label for="managed-create-name" class="input-label">{{ t('admin.accounts.vmManaged.create.name') }}</label>
        <input id="managed-create-name" v-model="form.name" class="input" maxlength="200" :disabled="busy" required />
      </div>

      <div>
        <label for="managed-create-proxy" class="input-label">{{ t('admin.accounts.vmManaged.create.proxy') }}</label>
        <select id="managed-create-proxy" v-model.number="form.proxyId" class="input" :disabled="busy" required>
          <option :value="0">{{ t('admin.accounts.vmManaged.proxySelect') }}</option>
          <option v-for="proxy in socksProxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }} ({{ proxy.host }}:{{ proxy.port }})</option>
        </select>
        <p v-if="!socksProxies.length" class="mt-1 text-xs text-amber-600 dark:text-amber-400" data-testid="managed-create-no-proxy">{{ t('admin.accounts.vmManaged.create.noProxy') }}</p>
        <p v-else class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.create.proxyHint') }}</p>
      </div>

      <div>
        <label for="managed-create-runtime" class="input-label">{{ t('admin.accounts.vmManaged.runtime') }}</label>
        <select id="managed-create-runtime" v-model="form.runtime" class="input" :disabled="busy">
          <option value="kvm">KVM</option>
          <option value="docker">Docker</option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.create.runtimeHint') }}</p>
      </div>

      <div>
        <span class="input-label">{{ t('admin.accounts.vmManaged.create.groups') }}</span>
        <div v-if="openaiGroups.length" class="mt-1 flex flex-wrap gap-x-4 gap-y-2">
          <label v-for="group in openaiGroups" :key="group.id" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
            <input v-model="form.groupIds" type="checkbox" :value="group.id" :disabled="busy" />
            {{ group.name }}
          </label>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.vmManaged.create.groupsHint') }}</p>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="managed-create-concurrency" class="input-label">{{ t('admin.accounts.vmManaged.create.concurrency') }}</label>
          <input id="managed-create-concurrency" v-model.number="form.concurrency" type="number" min="1" max="256" class="input" :disabled="busy" />
        </div>
        <div>
          <label for="managed-create-priority" class="input-label">{{ t('admin.accounts.vmManaged.create.priority') }}</label>
          <input id="managed-create-priority" v-model.number="form.priority" type="number" min="0" class="input" :disabled="busy" />
        </div>
      </div>

      <div v-if="mixedWarning" class="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-900/20 dark:text-amber-200" data-testid="managed-create-mixed">
        <p>{{ mixedWarning }}</p>
        <label class="flex items-center gap-2">
          <input v-model="form.confirmMixed" type="checkbox" :disabled="busy" />
          {{ t('admin.accounts.vmManaged.create.confirmMixed') }}
        </label>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="close">{{ t('common.cancel') }}</button>
        <button type="submit" form="managed-create-form" class="btn btn-primary" :disabled="busy || !canSubmit" data-testid="managed-create-submit">{{ t(busy ? 'admin.accounts.vmManaged.create.creating' : 'admin.accounts.vmManaged.create.submit') }}</button>
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
import { openaiVmManagedAPI as api, type OpenAIVMManagedCreateResult, type OpenAIVMManagedRuntime } from '@/api/admin/openaiVmManaged'
import type { AdminGroup, Proxy } from '@/types'

const props = withDefaults(defineProps<{ show: boolean; proxies?: Proxy[]; groups?: AdminGroup[] }>(), { proxies: () => [], groups: () => [] })
const emit = defineEmits<{ (e: 'close'): void; (e: 'created', result: OpenAIVMManagedCreateResult): void; (e: 'batch'): void }>()
const { t } = useI18n()
const stepUp = useStepUp()

const blank = () => ({ name: '', proxyId: 0, runtime: 'kvm' as OpenAIVMManagedRuntime, groupIds: [] as number[], concurrency: 3, priority: 1, confirmMixed: false })
const form = reactive(blank())
const busy = ref(false)
const error = ref('')
const mixedWarning = ref('')
let session = 0

const socksProxies = computed(() => props.proxies.filter(proxy => (proxy.protocol === 'socks5' || proxy.protocol === 'socks5h') && proxy.status === 'active'))
const openaiGroups = computed(() => props.groups.filter(group => group.platform === 'openai'))
const concurrencyValid = computed(() => Number.isInteger(form.concurrency) && form.concurrency >= 1 && form.concurrency <= 256)
const canSubmit = computed(() => form.name.trim() !== '' && form.proxyId > 0 && concurrencyValid.value && (!mixedWarning.value || form.confirmMixed))

function messageOf(cause: unknown): string {
  if (isStepUpBlocked(cause)) {
    return t(stepUpBlockReason(cause) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? 'stepUp.adminApiKeyForbidden' : 'stepUp.notEnabled')
  }
  const message = (cause as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : t('admin.accounts.vmManaged.failed')
}

function isMixedChannelWarning(cause: unknown): string {
  const body = cause as { error?: unknown; message?: unknown; response?: { data?: { error?: unknown; message?: unknown } } } | null
  const marker = body?.error ?? body?.response?.data?.error
  if (marker !== 'mixed_channel_warning') return ''
  const message = body?.message ?? body?.response?.data?.message
  return typeof message === 'string' && message ? message : t('admin.accounts.vmManaged.create.mixedDefault')
}

async function submit() {
  if (busy.value || !canSubmit.value) return
  const current = session
  busy.value = true
  error.value = ''
  const input = {
    name: form.name.trim(),
    proxy_id: form.proxyId,
    runtime_type: form.runtime,
    group_ids: [...form.groupIds],
    concurrency: form.concurrency,
    priority: Number.isFinite(form.priority) ? form.priority : 1,
    ...(form.confirmMixed ? { confirm_mixed_channel_risk: true } : {})
  }
  try {
    const result = await stepUp.run(async () => {
      if (current !== session) throw new StepUpCancelledError()
      return api.createAccount(input)
    })
    if (current !== session) return
    // The account exists even when its slot does not yet; the panel offers Retry.
    emit('created', result)
    emit('close')
  } catch (cause) {
    if (current !== session || isStepUpCancelled(cause)) return
    const warning = isMixedChannelWarning(cause)
    if (warning) {
      mixedWarning.value = warning
      form.confirmMixed = false
      return
    }
    error.value = messageOf(cause)
  } finally {
    if (current === session) busy.value = false
  }
}

function close() {
  if (!busy.value) emit('close')
}

function reset() {
  session += 1
  stepUp.onCancel()
  Object.assign(form, blank())
  busy.value = false
  error.value = ''
  mixedWarning.value = ''
}

watch(() => props.show, reset, { immediate: true })
// Changing the groups invalidates an earlier risk confirmation.
watch(() => [...form.groupIds], () => { mixedWarning.value = ''; form.confirmMixed = false })
onUnmounted(reset)
</script>
