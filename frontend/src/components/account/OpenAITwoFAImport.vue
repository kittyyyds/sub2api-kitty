<template>
  <div class="space-y-4">
    <CredentialEncryptionSetup @ready="encryptionReady = $event" />
    <p class="input-hint">一行一个账号：邮箱----密码----2FA密钥，也支持 --、Tab 或逗号分隔，最多200行。仅在内存中处理，关闭窗口会停止未完成任务。</p>
    <p class="input-hint">2FA 使用原版后台登录服务，无需打开登录窗口。邮箱、密码及 2FA 密钥会提交给凭证守护中配置的重登服务。</p>
    <p class="input-hint">{{ autoProxy ? '按 IP 规则为导入后的账号自动分配代理；该代理不用于本次登录。' : proxyId ? '所选代理仅绑定到导入后的账号，不用于本次登录。' : '登录出口由后台重登服务决定，不代表本服务器直连官方。' }}</p>
    <a href="/admin/token-guard" target="_blank" rel="noopener noreferrer" class="text-primary-600">
      {{ t('tokenGuard.twoFA.settings') }}
    </a>
    <template v-if="!rows.length">
      <label for="two-fa-credentials" class="input-label">{{ t('tokenGuard.twoFA.credentials') }}</label>
      <textarea id="two-fa-credentials" v-model="raw" class="input font-mono" rows="6"
        autocomplete="off" autocapitalize="off" :spellcheck="false" :disabled="busy"
        :placeholder="t('tokenGuard.twoFA.placeholder')" />
    </template>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <ul v-if="rows.length" class="space-y-2" aria-live="polite">
      <li v-for="(row, index) in rows" :key="row.entry.email" class="flex justify-between gap-4 text-sm">
        <span>{{ index + 1 }}. {{ row.entry.email }}</span>
        <div class="text-right space-y-1">
         <span>{{ t(`tokenGuard.twoFA.states.${row.status}`) }}</span>
         <p v-if="row.detail" class="text-xs text-gray-500">{{ row.detail }}</p>
         <p v-if="row.job?.message" class="text-xs text-amber-600">{{ row.job.message }}</p>
       </div>
      </li>
    </ul>
    <div class="flex gap-3">
      <button v-if="!busy && (!rows.length || hasPending)" type="button" class="btn btn-primary" :disabled="!encryptionReady" data-testid="two-fa-start" @click="run">
        {{ t(rows.length ? 'tokenGuard.twoFA.retry' : 'tokenGuard.twoFA.start') }}
      </button>
      <button v-if="busy" type="button" class="btn btn-secondary" :disabled="stopRequested" @click="stopRequested = true">
        {{ t('tokenGuard.twoFA.stop') }}
      </button>
      <button v-if="!busy && rows.length" type="button" class="btn btn-secondary" @click="reset">
        {{ t('tokenGuard.twoFA.reset') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import CredentialEncryptionSetup from './CredentialEncryptionSetup.vue'
import { useI18n } from 'vue-i18n'
import { deleteTwoFALogin, getTwoFALogin, parseTwoFALoginText, startTwoFALogin } from '@/api/admin/accountTokenGuard'
import { twoFAStartFailureMessage } from '@/utils/twoFAImportError'
import type { TokenGuardReloginAccount, TwoFALoginJob, TwoFALoginEnvironment } from '@/api/admin/accountTokenGuard'

const props = defineProps<{
  proxyId?: number | null
  autoProxy?: boolean
  loginEnvironment?: TwoFALoginEnvironment
  prepare?: () => Promise<void> | void
  importCredential: (credential: Record<string, unknown>, email: string, login: TokenGuardReloginAccount) => Promise<'created' | 'skipped'>
}>()
const emit = defineEmits<{ busy: [value: boolean]; reset: [] }>()
const { t } = useI18n()
type Row = {
  entry: TokenGuardReloginAccount
  status: 'pending' | 'login' | 'importing' | 'created' | 'skipped' | 'failed' | 'importFailed'
  credential?: Record<string, unknown>
  jobId?: string
  job?: TwoFALoginJob
  detail?: string
}
const raw = ref('')
const encryptionReady = ref(false)
const error = ref('')
const rows = ref<Row[]>([])
const busy = ref(false)
const stopRequested = ref(false)
const hasPending = computed(() => rows.value.some(row => row.status !== 'created' && row.status !== 'skipped'))
let disposed = false
let batchProxy: number | null | undefined
const stages: Record<string,string> = {queued:'排队中',relogin_service:'等待后台登录服务返回',protocol_login:'后台协议登录',timezone_probe:'检测代理时区',browser_starting:'启动登录窗口',navigation:'打开官方页面',security_verification:'等待安全验证',email:'账号验证',password:'密码验证',totp:'两步验证',authorization:'授权确认',token_exchange:'交换授权凭证',succeeded:'登录完成',failed:'登录未完成'}
function showProgress(row: Row, job: TwoFALoginJob) {
 row.job = {...job, credential: undefined}
 row.detail = [job.login_backend === 'configured_service' ? '原版后台登录服务' : '', job.stage ? (stages[job.stage] || '处理中') : '', job.timezone ? `登录时区：${job.timezone}` : '', job.http_status ? `HTTP ${job.http_status}` : '', job.proxy_id ? `导入后绑定代理：#${job.proxy_id}` : '', job.error_code || ''].filter(Boolean).join(' · ')
}

async function discardJob(row: Row) {
  if (!row.jobId) return
  const id = row.jobId
  row.jobId = undefined
  // Server expiry also cleans up jobs if the browser loses its connection.
  await deleteTwoFALogin(id).catch(() => {})
}

async function run() {
  if (busy.value || !encryptionReady.value) return
  error.value = ''
  busy.value = true
  stopRequested.value = false
  emit('busy', true)
  try {
    if (!rows.value.length) {
      try {
        const entries = parseTwoFALoginText(raw.value)
        const selectedProxy = props.proxyId
        await props.prepare?.()
        if (disposed || stopRequested.value) return
        batchProxy = selectedProxy
        rows.value = entries.map(entry => ({ entry, status: 'pending' }))
        raw.value = ''
      } catch (cause) {
        error.value = cause instanceof Error && cause.message === 'select_proxy'
          ? '登录时区设置需要先选择一个可用代理。'
          : t('tokenGuard.twoFA.invalid')
        return
      }
    }
    for (const row of rows.value) {
      if (stopRequested.value || disposed) break
      if (row.status === 'created' || row.status === 'skipped') continue
      try {
        if (!row.credential) {
          row.status = 'login'
          // Keep the job ID on polling failures; retry resumes the same login.
          if (!row.jobId) {
            row.job = undefined
            row.detail = undefined
            row.jobId = (await startTwoFALogin(row.entry, batchProxy)).id
          }
          let job = await getTwoFALogin(row.jobId)
          showProgress(row, job)
          while ((job.status === 'running' || job.status === 'waiting_interaction') && !stopRequested.value && !disposed) {
            await new Promise(resolve => setTimeout(resolve, 1500))
            if (stopRequested.value || disposed) break
            job = await getTwoFALogin(row.jobId)
            showProgress(row, job)
          }
          if (stopRequested.value || disposed) {
            await discardJob(row)
            row.status = 'pending'
            break
          }
          if (job.status !== 'succeeded' || !job.credential) {
            // Terminal diagnostic records remain until their server-side expiry.
            row.jobId = undefined
            throw new Error('login_failed')
          }
          row.credential = { ...job.credential, _import_ticket:job.import_ticket, _import_proxy_id:job.proxy_id }
        }
        if (stopRequested.value || disposed) { row.status = 'pending'; break }
        row.status = 'importing'
        // Keep the input in memory until encrypted credential-operations
        // enrollment succeeds. An import/enrollment retry reuses this login.
        row.status = await props.importCredential(row.credential, row.entry.email, { ...row.entry })
        await discardJob(row)
        row.credential = undefined
        row.entry.password = ''
        row.entry.mfa_secret = ''
      } catch (cause: unknown) {
        const status = (cause as { status?: number; response?: { status?: number } })?.status ??
          (cause as { response?: { status?: number } })?.response?.status
        if (status === 404) row.jobId = undefined
        if (row.credential) {
          row.detail = '账号保存未完成，登录结果已保留，可重试保存。'
        } else if (row.job?.status !== 'failed') {
          row.detail = twoFAStartFailureMessage(cause) ?? (row.jobId
            ? '登录任务状态读取失败；点击继续将查询原任务，不会重复登录。'
            : '登录任务未能启动，请检查代理及登录服务配置后重试。')
        }
        row.status = row.credential ? 'importFailed' : 'failed'
      }
    }
  } finally {
    busy.value = false
    emit('busy', false)
  }
}

function reset() {
  for (const row of rows.value) void discardJob(row)
  rows.value = []
  batchProxy = undefined
  emit('reset')
  raw.value = ''
  error.value = ''
}

onBeforeUnmount(() => {
  disposed = true
  stopRequested.value = true
  reset()
})
</script>
