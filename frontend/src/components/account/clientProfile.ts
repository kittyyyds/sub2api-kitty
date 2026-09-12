import type { Account } from '@/types'

export type ClientPreset = 'default' | 'macos' | 'linux' | 'windows'
export interface ClientProfile {
  enabled: boolean
  preset: ClientPreset
  language: string
  client_version: string
  http_version: 'inherit' | 'http1' | 'http2'
  tls_profile: 'inherit' | 'chrome' | 'firefox' | 'safari'
  connection_reuse: 'inherit' | 'reuse' | 'fresh'
  ws_mode: 'inherit' | 'off' | 'ctx_pool' | 'passthrough'
  max_conns_per_host: number
  max_idle_conns_per_host: number
  idle_conn_timeout_seconds: number
  retry_backoff_initial_ms: number
  retry_backoff_max_ms: number
  retry_total_budget_ms: number
}

export const presetOptions: { value: ClientPreset; label: string; description: string }[] = [
  { value: 'default', label: '默认', description: '保留账号原有 User-Agent；未设置时沿用全局' },
  { value: 'macos', label: 'Mac', description: 'Mac OS X 14.5 · arm64 · iTerm2' },
  { value: 'linux', label: 'Linux', description: 'Ubuntu 22.04 · x86_64 · xterm-256color' },
  { value: 'windows', label: 'Windows', description: 'Windows 11 · x86_64 · WindowsTerminal' }
]

export function defaultClientProfile(): ClientProfile {
  return { enabled: false, preset: 'default', language: '', client_version: '', http_version: 'inherit', tls_profile: 'inherit', connection_reuse: 'inherit', ws_mode: 'inherit', max_conns_per_host: 0, max_idle_conns_per_host: 0, idle_conn_timeout_seconds: 0, retry_backoff_initial_ms: 0, retry_backoff_max_ms: 0, retry_total_budget_ms: 0 }
}

export function isClientProfileAccount(account: Pick<Account, 'platform' | 'type' | 'parent_account_id'>): boolean {
  return account.platform === 'openai' && account.type === 'oauth' && !account.parent_account_id
}

export function readClientProfile(extra?: Record<string, unknown>): ClientProfile {
  const raw = extra?.openai_client_profile
  const profile = defaultClientProfile()
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    for (const key of Object.keys(profile) as (keyof ClientProfile)[]) {
      const value = (raw as Record<string, unknown>)[key]
      if (typeof value === typeof profile[key]) Object.assign(profile, { [key]: value })
    }
  }
  return profile
}

export function clientProfileLabel(extra?: Record<string, unknown>): string {
  const profile = readClientProfile(extra)
  return profile.enabled ? presetOptions.find(item => item.value === profile.preset)?.label || '默认' : '默认'
}

export function validateClientProfile(profile: ClientProfile): string {
  if (profile.tls_profile !== 'inherit' && profile.http_version === 'http2') return '自定义 TLS 模板暂仅支持 HTTP/1.1，请将 HTTP 版本设为沿用现有或 HTTP/1.1。'
  if (!/^[\x20-\x7e]{0,64}$/.test(profile.language) || /[\r\n]/.test(profile.client_version)) return '语言需为最多 64 个标准 ASCII 字符；语言和版本不能包含换行。'
  if (profile.client_version && !/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(profile.client_version)) return '客户端版本请填写完整版本号，例如 0.146.0。'
  const limits: Partial<Record<keyof ClientProfile, number>> = { max_conns_per_host: 256, max_idle_conns_per_host: 256, idle_conn_timeout_seconds: 600, retry_backoff_initial_ms: 30000, retry_backoff_max_ms: 30000, retry_total_budget_ms: 120000 }
  for (const [key, maximum] of Object.entries(limits)) {
    const value = profile[key as keyof ClientProfile] as number
    if (!Number.isInteger(value) || value < 0 || value > maximum) return '高级数值超出允许范围；0 表示沿用现有。'
  }
  if (profile.max_conns_per_host > 0 && profile.max_idle_conns_per_host > profile.max_conns_per_host) return '空闲连接上限不能大于总连接上限。'
  if (profile.retry_backoff_initial_ms > 0 && profile.retry_backoff_max_ms > 0 && profile.retry_backoff_initial_ms > profile.retry_backoff_max_ms) return '首次重试等待不能大于最大重试等待。'
  return ''
}
