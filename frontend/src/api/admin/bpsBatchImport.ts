import { apiClient } from '../client'

export interface BPSImportEntry { email: string; password: string; totp_secret: string }
export interface BPSImportInput {
  request_id: string
  entries: BPSImportEntry[]
  group_ids: number[]
  proxy_id: number | null
  workspace: string
  concurrency: number
  priority: number
  models: string[]
}
export interface BPSImportRow {
  email: string
  status: 'queued' | 'running' | 'succeeded' | 'skipped' | 'failed' | 'canceled'
  stage: string
  message?: string
  account_id?: number
}
export interface BPSImportStatus extends Omit<BPSImportInput, 'entries'> {
  id: string
  status: 'running' | 'canceling' | 'completed' | 'canceled'
  rows: BPSImportRow[]
  created_at: string
  expires_at: string
}
const base = '/admin/accounts/bps-imports'
export const bpsBatchImportAPI = {
  async start(input: BPSImportInput) { return (await apiClient.post<BPSImportStatus>(base, input)).data },
  async list() { return (await apiClient.get<BPSImportStatus[]>(base)).data },
  async get(id: string) { return (await apiClient.get<BPSImportStatus>(`${base}/${encodeURIComponent(id)}`)).data },
  async cancel(id: string) { return (await apiClient.post<BPSImportStatus>(`${base}/${encodeURIComponent(id)}/cancel`)).data }
}

// Errors carry line numbers only, never passwords, TOTP secrets or raw input.
export function parseBPSImportEntries(raw: string): BPSImportEntry[] {
  const lines = raw.split(/\r?\n/).map((text, i) => ({ text, line: i + 1 })).filter(x => x.text.trim())
  if (lines.length < 1 || lines.length > 20) throw new Error('count')
  const seen = new Set<string>()
  return lines.map(({ text, line }) => {
    const fields = text.includes('\t') ? text.split('\t') : text.split(/-{3,}/)
    const email = (fields[0] || '').trim().toLowerCase()
    const password = fields[1] || ''
    const totp_secret = (fields[2] || '').replace(/\s/g, '').toUpperCase()
    if (fields.length < 2 || fields.length > 3 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) ||
        email.length > 254 || !password || password.length > 4096 || /[\x00\r\n]/.test(password) || seen.has(email) ||
        (totp_secret && !/^[A-Z2-7]{16,128}={0,6}$/.test(totp_secret))) throw new Error(`line:${line}`)
    seen.add(email)
    return { email, password, totp_secret }
  })
}
