<template>
  <div class="space-y-4 rounded-lg border border-blue-200 bg-white/80 p-4 dark:border-blue-700 dark:bg-gray-800/80" data-testid="openai-password-login">
    <p class="text-sm text-blue-800 dark:text-blue-200">{{ t(reauthorize ? 'admin.accounts.oauth.openai.passwordLogin.reauthDescription' : 'admin.accounts.oauth.openai.passwordLogin.description') }}</p>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t(proxyId ? (reauthorize ? 'admin.accounts.oauth.openai.passwordLogin.reauthProxyExit' : 'admin.accounts.oauth.openai.passwordLogin.proxyExit') : 'admin.accounts.oauth.openai.passwordLogin.directExit') }}</p>
    <p v-if="!available" class="text-sm text-amber-700 dark:text-amber-300">{{ t(checking ? 'admin.accounts.oauth.openai.passwordLogin.checking' : 'admin.accounts.oauth.openai.passwordLogin.unavailable') }}</p>
    <div v-if="saved" class="space-y-2 rounded-lg bg-blue-50 p-3 text-sm dark:bg-blue-950/40">
      <label class="flex items-center gap-2"><input v-model="useSaved" type="checkbox" :disabled="busy || !!result || loading" />{{ t('admin.accounts.oauth.openai.passwordLogin.useSaved') }} · {{ saved.email }}</label>
      <button type="button" class="text-red-600 dark:text-red-400" :disabled="busy || !!result || loading" @click="forgetSaved">{{ t('admin.accounts.oauth.openai.passwordLogin.forget') }}</button>
    </div>
    <template v-if="!reuseSaved">
    <div class="grid gap-3 sm:grid-cols-2">
      <label class="text-sm text-gray-700 dark:text-gray-200">
        {{ t('admin.accounts.oauth.openai.passwordLogin.email') }}
        <input v-model="email" type="email" autocomplete="off" :disabled="busy || !!result" class="input mt-1 w-full" />
      </label>
      <label class="text-sm text-gray-700 dark:text-gray-200">
        {{ t('admin.accounts.oauth.openai.passwordLogin.password') }}
        <input v-model="password" type="password" autocomplete="new-password" :disabled="busy || !!result" class="input mt-1 w-full" />
      </label>
    </div>
    <div class="flex flex-wrap gap-4 text-sm text-gray-700 dark:text-gray-200">
      <label class="flex items-center gap-2"><input v-model="mfaMode" type="radio" value="secret" :disabled="busy || !!result" />{{ t('admin.accounts.oauth.openai.passwordLogin.secretMode') }}</label>
      <label class="flex items-center gap-2"><input v-model="mfaMode" type="radio" value="code" :disabled="busy || !!result" />{{ t('admin.accounts.oauth.openai.passwordLogin.codeMode') }}</label>
    </div>
    <label v-if="mfaMode === 'secret'" class="block text-sm text-gray-700 dark:text-gray-200">
      {{ t('admin.accounts.oauth.openai.passwordLogin.secret') }}
      <input v-model="mfaSecret" type="password" autocomplete="new-password" :disabled="busy || !!result" class="input mt-1 w-full font-mono" />
    </label>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.oauth.openai.passwordLogin.codeHint') }}</p>
    <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300"><input v-model="remember" type="checkbox" :disabled="busy || !!result || loading" />{{ t('admin.accounts.oauth.openai.passwordLogin.remember') }}</label>
    </template>
    <div v-if="job" role="status" class="rounded-lg bg-gray-100 p-3 text-sm text-gray-700 dark:bg-gray-900 dark:text-gray-200">
      {{ job.message || t(`admin.accounts.oauth.openai.passwordLogin.stages.${job.status === 'succeeded' ? 'succeeded' : job.status === 'awaiting_code' ? 'awaiting_code' : job.stage}`, job.stage) }}
    </div>
    <div v-if="job?.status === 'awaiting_code'" class="flex gap-2">
      <input v-model="code" type="text" inputmode="numeric" maxlength="6" autocomplete="one-time-code" class="input min-w-0 flex-1 font-mono" :placeholder="t('admin.accounts.oauth.openai.passwordLogin.codePlaceholder')" />
      <button type="button" class="btn btn-primary" :disabled="sending || !/^\d{6}$/.test(code)" @click="submitCode">{{ t('common.submit') }}</button>
    </div>
    <p v-if="localError || error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ localError || error }}</p>
    <div class="flex flex-wrap gap-2">
      <button v-if="!busy && !result" type="button" class="btn btn-primary" :disabled="!canStart || loading" @click="start">{{ t(reuseSaved ? 'admin.accounts.oauth.openai.passwordLogin.startSaved' : reauthorize ? 'admin.accounts.oauth.openai.passwordLogin.reauthStart' : 'admin.accounts.oauth.openai.passwordLogin.start') }}</button>
      <button v-if="busy" type="button" class="btn btn-secondary" :disabled="sending || job?.status === 'committing'" @click="cancel">{{ t('common.cancel') }}</button>
      <button v-if="result" type="button" class="btn btn-primary" :disabled="loading" @click="emit('authorized', result)">{{ t(reauthorize ? 'admin.accounts.oauth.openai.passwordLogin.reauthSave' : 'admin.accounts.oauth.openai.passwordLogin.save') }}</button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.oauth.openai.passwordLogin.savedPrivacy') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { openaiPasswordLogin, type PasswordLoginJob, type PasswordLoginResult, type SavedPasswordLogin } from '@/api/admin/openaiPasswordLogin'
const props = defineProps<{ proxyId?: number | null; loading?: boolean; error?: string; reauthorize?: boolean; accountId?: number | null }>()
const emit = defineEmits<{ authorized: [result: PasswordLoginResult] }>()
const { t } = useI18n()
const available = ref(false), checking = ref(true), sending = ref(false)
const email = ref(''), password = ref(''), mfaSecret = ref(''), code = ref(''), localError = ref('')
const mfaMode = ref<'secret' | 'code'>('secret')
const job = ref<PasswordLoginJob | null>(null)
const result = ref<PasswordLoginResult | null>(null)
const saved = ref<SavedPasswordLogin | null>(null), useSaved = ref(true), remember = ref(true)
const reuseSaved = computed(() => !!props.accountId && !!saved.value && useSaved.value)
const busy = computed(() => sending.value || !!job.value && ['running', 'awaiting_code', 'committing'].includes(job.value.status))
const canStart = computed(() => available.value && (reuseSaved.value || (email.value.trim() && password.value && (mfaMode.value === 'code' || mfaSecret.value.trim()))))
let alive = true
let timer: ReturnType<typeof setTimeout> | undefined
let pendingID: string | undefined
let pollErrors = 0
let rememberRequested = false
const message = (error: any) => error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.oauth.openai.passwordLogin.failed')
const clearSecrets = () => { password.value = ''; mfaSecret.value = ''; code.value = '' }
function schedule() { if (alive) timer = setTimeout(poll, 1500) }
async function poll() {
  const id = pendingID
  if (!id || !alive) return
  try {
    const state = await openaiPasswordLogin.status(id)
    if (!alive || pendingID !== id) return
    job.value = state
    pollErrors = 0
    if (state.status === 'succeeded') {
      result.value = await openaiPasswordLogin.exchange(id)
      if (!alive) { result.value = null; return }
      if (rememberRequested && !result.value.remember_job_id) result.value.remember_failed = true
      pendingID = undefined
      emit('authorized', result.value)
      return
    }
    if (['failed', 'canceled'].includes(state.status)) { pendingID = undefined; return }
  } catch (error) {
    if (!alive) return
    if ((error as any).response?.status === 404) {
      pendingID = undefined
      localError.value = message(error)
      job.value = { id, status: 'failed', stage: 'starting', created_at: Date.now() }
      return
    }
    if (++pollErrors >= 3) localError.value = message(error)
    // Keep the owned task id so a transient network error can still be canceled.
  }
  schedule()
}
async function start() {
  if (!canStart.value || busy.value) return
  sending.value = true; localError.value = ''; result.value = null
  try {
    const input = { email: email.value.trim(), password: password.value, mfa_mode: mfaMode.value, mfa_secret: mfaMode.value === 'secret' ? mfaSecret.value : '', proxy_id: props.proxyId ?? null, remember: remember.value }
    rememberRequested = !reuseSaved.value && remember.value
    const state = reuseSaved.value ? await openaiPasswordLogin.startSaved(props.accountId!) : await openaiPasswordLogin.start(input)
    input.password = ''; input.mfa_secret = ''
    if (!alive) { void openaiPasswordLogin.cancel(state.id).catch(() => {}); return }
    job.value = state; pendingID = state.id; schedule()
  } catch (error) { if (alive) localError.value = message(error) }
  finally { clearSecrets(); sending.value = false }
}
async function submitCode() {
  if (!pendingID || sending.value || !/^\d{6}$/.test(code.value)) return
  sending.value = true; localError.value = ''
  try { job.value = await openaiPasswordLogin.code(pendingID, code.value) }
  catch (error) { localError.value = message(error) }
  finally { code.value = ''; sending.value = false }
}
async function cancel() {
  if (!pendingID) return
  const id = pendingID; sending.value = true
  try {
    job.value = await openaiPasswordLogin.cancel(id)
    pendingID = undefined; clearTimeout(timer)
  } catch (error) { localError.value = message(error) }
  finally { clearSecrets(); sending.value = false }
}
onMounted(async () => {
  if (props.accountId) {
    void openaiPasswordLogin.saved(props.accountId).then((value) => {
      if (!alive) return
      saved.value = value.saved
      if (value.saved) { email.value = value.saved.email; mfaMode.value = value.saved.mfa_mode }
    }).catch((error) => { if (alive) localError.value = message(error) })
  }
  try { available.value = (await openaiPasswordLogin.capabilities()).available }
  catch { available.value = false }
  finally { checking.value = false }
})
async function forgetSaved() {
  if (!props.accountId || busy.value || props.loading) return
  sending.value = true
  try { await openaiPasswordLogin.forget(props.accountId); if (alive) saved.value = null }
  catch (error) { if (alive) localError.value = message(error) }
  finally { sending.value = false }
}
onBeforeUnmount(() => {
  alive = false; clearTimeout(timer); clearSecrets(); email.value = ''; result.value = null
  if (pendingID) void openaiPasswordLogin.cancel(pendingID).catch(() => {})
})
</script>
