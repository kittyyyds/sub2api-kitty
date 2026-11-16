import { apiClient } from '../client'

// Slot creation and login steps can take a while on the runtime side.
const requestOptions = { timeout: 120_000 }

export type OpenAIVMManagedRuntime = 'kvm' | 'docker'

/** What the administrator sees. Derived on the server from the slot's own report. */
export type OpenAIVMManagedStatus =
  | 'available'
  | 'awaiting_login'
  | 'needs_relogin'
  | 'needs_attention'
  | 'relogin_running'
  | 'restricted'
  | 'paused'
  | 'stopped'
  | 'slot_error'
  | 'provisioning'
  | 'starting'
  | 'error'
  | 'deleting'
  | 'unknown'

export const OPENAI_VM_MANAGED_STATUSES: readonly OpenAIVMManagedStatus[] = [
  'available', 'awaiting_login', 'needs_relogin', 'needs_attention', 'relogin_running', 'restricted',
  'paused', 'stopped', 'slot_error', 'provisioning', 'starting', 'error', 'deleting', 'unknown'
]

export interface OpenAIVMManagedUsageWindow {
  id: string
  used_percent: number | null
  reset_at: string | null
  reset_after_seconds: number | null
  window_minutes: number | null
}

/** The slot report. It never carries a token, a password or a 2FA secret. */
export interface OpenAIVMManagedSnapshot {
  status?: string
  schedulable?: boolean
  schedule_state?: string
  restriction_reason?: string
  restriction_text?: string
  restriction_until?: number
  unschedulable_reason?: string
  unschedulable_text?: string
  has_token?: boolean
  has_refresh?: boolean
  refresh_error?: string
  expires_at?: string
  email?: string
  chatgpt_account_id?: string
  plan_type?: string
  subscription_end?: string
  runtime_type?: string
  execution?: string
  max_concurrency?: number
  proxy_label?: string
  proxy_status?: string
  usage?: { windows?: OpenAIVMManagedUsageWindow[] } | null
  credits?: Record<string, unknown> | null
  reset_credits?: { available_count?: number; credits?: Array<{ expires_at?: string }> } | null
  /** Says that a login is saved on the runtime; the login itself stays there. */
  saved_login?: { email: string; has_password: boolean; has_mfa_secret: boolean; mfa_mode?: string } | null
  auto_relogin?: {
    attempts_today?: number
    last_attempt_at?: string | null
    last_result?: string | null
    last_reason?: string | null
    needs_attention?: boolean
    attention_reason?: string | null
    attention_at?: string | null
  } | null
}

export interface OpenAIVMManagedView {
  account_id: number
  status: OpenAIVMManagedStatus | ''
  binding_status: string
  runtime_type: OpenAIVMManagedRuntime
  slot_id: string
  proxy_id: number | null
  last_error?: string
  activated_at?: string | null
  synced_at?: string | null
  snapshot?: OpenAIVMManagedSnapshot | null
}

export interface OpenAIVMManagedSettings {
  notes?: string | null
  group_ids: number[]
  concurrency: number
  priority: number
  rate_multiplier?: number
  group_rate_multiplier?: number
  load_factor?: number
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  group_allowed_models?: Record<number, string[]>
}
export interface OpenAIVMBatchEntry { email: string; password: string; mfa_secret: string }

export interface OpenAIVMManagedCreateInput {
  settings?: OpenAIVMManagedSettings
  name: string
  notes?: string | null
  group_ids?: number[]
  proxy_id: number
  runtime_type?: OpenAIVMManagedRuntime
  concurrency?: number
  priority?: number
  confirm_mixed_channel_risk?: boolean
}

export interface OpenAIVMManagedCreateResult {
  account_id: number
  name: string
  managed: OpenAIVMManagedView | null
  /** Present when the account exists but its slot could not be created yet. */
  slot_error?: string
}

export interface OpenAIVMManagedLoginJob {
  id: string
  status: 'running' | 'awaiting_code' | 'committing' | 'succeeded' | 'failed' | 'canceled' | string
  stage?: string
  reason?: string | null
  message?: string | null
  created_at?: number | string
}

export interface OpenAIVMManagedPasswordState {
  available?: boolean
  job?: OpenAIVMManagedLoginJob | null
  saved?: { email: string; has_password: boolean; has_mfa_secret: boolean; mfa_mode?: string } | null
  email?: string
}

export interface OpenAIVMManagedPasswordInput {
  email?: string
  password?: string
  mfa_mode?: 'secret' | 'code'
  mfa_secret?: string
  /** Log in again with what the runtime saved; nothing is typed or sent from here. */
  use_saved?: boolean
}

const base = (accountId: number) => `/admin/accounts/${accountId}/vm-managed`

export async function createAccount(input: OpenAIVMManagedCreateInput): Promise<OpenAIVMManagedCreateResult> {
  const { data } = await apiClient.post<OpenAIVMManagedCreateResult>('/admin/openai-vm/managed-accounts', input, requestOptions)
  return data
}

export async function getStatus(accountId: number): Promise<OpenAIVMManagedView> {
  const { data } = await apiClient.get<OpenAIVMManagedView>(base(accountId), requestOptions)
  return data
}

export async function retrySlot(accountId: number): Promise<OpenAIVMManagedView> {
  const { data } = await apiClient.post<OpenAIVMManagedView>(`${base(accountId)}/retry`, undefined, requestOptions)
  return data
}

export async function deleteAccount(accountId: number): Promise<void> {
  await apiClient.delete(base(accountId), requestOptions)
}

export async function changeProxy(accountId: number, proxyId: number): Promise<OpenAIVMManagedView> {
  const { data } = await apiClient.put<OpenAIVMManagedView>(`${base(accountId)}/proxy`, { proxy_id: proxyId }, requestOptions)
  return data
}

export async function setSchedulable(accountId: number, schedulable: boolean): Promise<OpenAIVMManagedView> {
  const { data } = await apiClient.post<OpenAIVMManagedView>(`${base(accountId)}/schedulable`, { schedulable }, requestOptions)
  return data
}

export async function refreshUsage(accountId: number): Promise<OpenAIVMManagedView> {
  const { data } = await apiClient.post<OpenAIVMManagedView>(`${base(accountId)}/usage/refresh`, undefined, requestOptions)
  return data
}

async function operation<T>(accountId: number, name: string, body?: unknown): Promise<T> {
  const { data } = await apiClient.post<T>(`${base(accountId)}/op/${name}`, body ?? {}, requestOptions)
  return data
}

async function readOperation<T>(accountId: number, name: string): Promise<T> {
  const { data } = await apiClient.get<T>(`${base(accountId)}/op/${name}`, requestOptions)
  return data
}

// --- login -----------------------------------------------------------------

export const oauthUrl = (accountId: number) =>
  operation<{ auth_url: string; session_id: string; expires_at?: number }>(accountId, 'login/oauth-url')

export const oauthCode = (accountId: number, input: { session_id: string; code?: string; callback_url?: string }) =>
  operation<Record<string, unknown>>(accountId, 'login/oauth-code', input)

export const passwordStatus = (accountId: number) =>
  readOperation<OpenAIVMManagedPasswordState>(accountId, 'login/password')

export const passwordLogin = (accountId: number, input: OpenAIVMManagedPasswordInput) =>
  operation<{ job: OpenAIVMManagedLoginJob }>(accountId, 'login/password', input)

export const passwordCode = (accountId: number, jobId: string, code: string) =>
  operation<{ job: OpenAIVMManagedLoginJob }>(accountId, 'login/password/code', { job_id: jobId, code })

export const passwordCancel = (accountId: number, jobId: string) =>
  operation<{ job: OpenAIVMManagedLoginJob }>(accountId, 'login/password/cancel', { job_id: jobId })

export const forgetSavedLogin = (accountId: number) =>
  operation<{ saved: null }>(accountId, 'login/password/forget')

/** Import a Codex account file. `raw` is the pasted file content. */
export const importAccountFile = (accountId: number, raw: string) =>
  operation<Record<string, unknown>>(accountId, 'login/import', { raw })

// --- maintenance -----------------------------------------------------------

export const refreshToken = (accountId: number) => operation<Record<string, unknown>>(accountId, 'token/refresh')
/** Spends one reset credit. It cannot be undone or refunded. */
export const useResetCredit = (accountId: number) => operation<Record<string, unknown>>(accountId, 'usage/reset-credit')
export const powerOn = (accountId: number) => operation<Record<string, unknown>>(accountId, 'power/on')
export const powerOff = (accountId: number) => operation<Record<string, unknown>>(accountId, 'power/off')
export const recover = (accountId: number) => operation<OpenAIVMManagedDiagnosis>(accountId, 'recover')
export const exitProbe = (accountId: number) => operation<OpenAIVMManagedExitProbe>(accountId, 'exit-probe')
export const diagnose = (accountId: number) => readOperation<OpenAIVMManagedDiagnosis>(accountId, 'diagnose')

export interface OpenAIVMManagedDiagnosis {
  /** Set instead of a report when a restart is still booting the slot. */
  starting?: boolean
  checked_at?: string
  ok?: boolean
  first_failure?: string | null
  checks?: Array<{ id: string; ok: boolean; label?: string; detail?: string }>
}

export interface OpenAIVMManagedExitProbe {
  ip?: string
  country?: string
  country_code?: string
  region?: string
  city?: string
  timezone?: string
  [key: string]: unknown
}

// --- batch import -------------------------------------------------------------

export interface OpenAIVMBatchInput {
  count?: number
  login_mode?: 'manual' | 'batch_2fa'
  name_prefix?: string
  entries?: OpenAIVMBatchEntry[]
  settings?: OpenAIVMManagedSettings
  plan_token?: string
  /** One account per line: email---password---2fa-secret[---socks5://...]. Never stored. */
  text: string
  proxy_ids: number[]
  per_proxy: number
  runtime_type?: OpenAIVMManagedRuntime
  group_ids?: number[]
  concurrency?: number
  priority?: number
  confirm_mixed_channel_risk?: boolean
}

export interface OpenAIVMBatchPreview {
  plan_token?: string
  importable: number
  problems: Array<{ line_no: number; email?: string; code: string }>
  lines: Array<{ line_no: number; name?: string; email: string; proxy_id?: number; new_proxy?: boolean }>
  estimated_memory_mb: number
  estimated_disk_gb: number
}

export interface OpenAIVMBatchItem {
  id: number
  line_no: number
  email: string
  proxy_id: number | null
  account_id: number | null
  status: string
  error_code?: string
  updated_at: string
}

export interface OpenAIVMBatchJob {
  id: number
  status: string
  runtime_type: OpenAIVMManagedRuntime
  total: number
  created_at: string
  finished_at?: string | null
  items: OpenAIVMBatchItem[]
}

export async function batchPreview(input: OpenAIVMBatchInput): Promise<OpenAIVMBatchPreview> {
  const { data } = await apiClient.post<OpenAIVMBatchPreview>('/admin/openai-vm/managed-batches/preview', input, requestOptions)
  return data
}

export async function batchStart(input: OpenAIVMBatchInput): Promise<OpenAIVMBatchJob> {
  const { data } = await apiClient.post<OpenAIVMBatchJob>('/admin/openai-vm/managed-batches', input, requestOptions)
  return data
}

export async function batchStatus(id: number): Promise<OpenAIVMBatchJob> {
  const { data } = await apiClient.get<OpenAIVMBatchJob>(`/admin/openai-vm/managed-batches/${id}`, requestOptions)
  return data
}

export async function batchCancel(id: number): Promise<OpenAIVMBatchJob> {
  const { data } = await apiClient.post<OpenAIVMBatchJob>(`/admin/openai-vm/managed-batches/${id}/cancel`, undefined, requestOptions)
  return data
}

export const openaiVmManagedAPI = {
  createAccount, getStatus, retrySlot, deleteAccount, changeProxy, setSchedulable, refreshUsage,
  batchPreview, batchStart, batchStatus, batchCancel,
  oauthUrl, oauthCode, passwordStatus, passwordLogin, passwordCode, passwordCancel, forgetSavedLogin, importAccountFile,
  refreshToken, useResetCredit, powerOn, powerOff, recover, exitProbe, diagnose
}
