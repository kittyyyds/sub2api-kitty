import { apiClient } from '../client'
import type { OpenAITokenInfo } from '@/composables/useOpenAIOAuth'

export interface PasswordLoginJob {
  id: string
  status: 'running' | 'awaiting_code' | 'committing' | 'succeeded' | 'failed' | 'canceled'
  stage: string
  reason?: string
  message?: string
  created_at: number
}
export interface PasswordLoginResult {
  token_info: OpenAITokenInfo
  proxy_id: number | null
  remember_job_id?: string
  remember_failed?: boolean
}
export interface SavedPasswordLogin {
  email: string
  has_password: boolean
  has_mfa_secret: boolean
  mfa_mode: 'secret' | 'code'
}
export interface PasswordLoginInput {
  email: string
  password: string
  mfa_mode: 'secret' | 'code'
  mfa_secret: string
  proxy_id?: number | null
  remember?: boolean
}
const base = '/admin/openai/password-login'
export const openaiPasswordLogin = {
  saved: async (id: number): Promise<{ saved: SavedPasswordLogin | null }> => (await apiClient.get(`/admin/accounts/${id}/password-login`)).data,
  remember: async (id: number, jobID: string): Promise<void> => { await apiClient.post(`/admin/accounts/${id}/password-login/remember`, { job_id: jobID }) },
  forget: async (id: number): Promise<void> => { await apiClient.delete(`/admin/accounts/${id}/password-login`) },
  startSaved: async (id: number): Promise<PasswordLoginJob> => (await apiClient.post(`/admin/accounts/${id}/password-login/tasks`, {})).data,
  capabilities: async (): Promise<{ available: boolean }> => (await apiClient.get(`${base}/capabilities`)).data,
  start: async (input: PasswordLoginInput): Promise<PasswordLoginJob> => (await apiClient.post(`${base}/tasks`, input)).data,
  status: async (id: string): Promise<PasswordLoginJob> => (await apiClient.get(`${base}/tasks/${id}`)).data,
  code: async (id: string, code: string): Promise<PasswordLoginJob> => (await apiClient.post(`${base}/tasks/${id}/code`, { code })).data,
  cancel: async (id: string): Promise<PasswordLoginJob> => (await apiClient.delete(`${base}/tasks/${id}`)).data,
  exchange: async (id: string): Promise<PasswordLoginResult> => (await apiClient.post(`${base}/tasks/${id}/exchange`, {})).data,
}
