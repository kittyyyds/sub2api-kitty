import type { OpenAIVMBatchEntry } from '@/api/admin/openaiVmManaged'
import type { Proxy } from '@/types'

export interface ParsedVMLogin extends OpenAIVMBatchEntry { selected: boolean; problem: string }

export function validateVMLogin(row: OpenAIVMBatchEntry): string {
  if (!/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(row.email.trim())) return 'invalid_email'
  if (!row.password.trim()) return 'missing_password'
  if (!/^[A-Z2-7]{16,128}=*$/.test(row.mfa_secret.replace(/ /g, '').toUpperCase())) return 'invalid_mfa_secret'
  return ''
}

export function parseVMLogins(text: string): ParsedVMLogin[] {
  return text.split(/\r?\n/).filter(line => line.trim()).map(line => {
    const raw = line.trim()
    const first = raw.indexOf('---')
    const separator = first >= 0 && raw.startsWith('----', first) ? '----' : '---'
    const last = raw.lastIndexOf(separator)
    if (first <= 0 || last <= first) return { email: '', password: '', mfa_secret: '', selected: false, problem: 'invalid_format' }
    const row = { email: raw.slice(0, first).trim(), password: raw.slice(first + separator.length, last), mfa_secret: raw.slice(last + separator.length).trim().replace(/ /g, '').toUpperCase(), selected: true, problem: '' }
    row.problem = validateVMLogin(row)
    return row
  })
}

/** Same naming as the server: one account keeps the prefix as is, a batch gets -01, -02, … */
export function vmAccountName(prefix: string, lineNo: number, total: number): string {
  const name = prefix.trim()
  if (!name) return ''
  return total > 1 ? `${name}-${String(lineNo).padStart(2, '0')}` : name
}

export type VMProxyUnusableReason = 'protocol' | 'inactive' | 'expired'

/** Why a proxy cannot carry a VM2-managed slot; null when it can. Mirrors the server's checks. */
export function proxyUnusableReason(proxy: Pick<Proxy, 'protocol' | 'status' | 'expires_at'>, now = Date.now()): VMProxyUnusableReason | null {
  if (proxy.protocol !== 'socks5' && proxy.protocol !== 'socks5h') return 'protocol'
  if (proxy.status !== 'active') return 'inactive'
  if (proxy.expires_at && Date.parse(proxy.expires_at) <= now) return 'expired'
  return null
}

export function shuffleVMItems<T>(items: T[]): T[] {
  const result = [...items]
  for (let i = result.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[result[i], result[j]] = [result[j], result[i]]
  }
  return result
}
