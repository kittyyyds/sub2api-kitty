import { apiClient } from './client'

export type CandyStatus = 'queued' | 'running' | 'passed' | 'failed' | 'inconclusive' | 'error' | 'cancelled'
export interface CandyConfig {
  id: number
  group_id: number
  api_key_id: number
  label: string
  model: string
  reasoning_effort: string
  question: string
  expected_answers: string[]
  interval_minutes: number
  max_output_tokens: number
  enabled: boolean
  published: boolean
}
export interface CandyRun {
  id: number
  test_id: number
  status: CandyStatus
  question: string
  model: string
  response_model?: string
  response_id?: string
  reasoning_effort: string
  output: string
  answer: string
  standard_answer?: string
  message?: string
  input_tokens: number | null
  output_tokens: number | null
  cache_read_tokens: number | null
  latency_ms: number
  created_at: string
  finished_at: string | null
}
export interface CandyCard {
	history_count: number
  id: number
  label: string
  model: string
  reasoning_effort: string
  enabled: boolean
  interval_minutes: number
  next_run_at: string | null
  history: CandyRun[]
  config?: CandyConfig
}
export interface CandyKey { id: number; name: string; group_id: number; group_name: string }
export const candyTestsAPI = {
  async list(admin = false, signal?: AbortSignal) {
    const { data } = await apiClient.get<{ cards: CandyCard[]; updated_at: string }>(admin ? '/admin/candy-tests' : '/candy-tests', { signal })
    return data
  },
  async keys() {
    return (await apiClient.get<CandyKey[]>('/admin/candy-tests/keys')).data
  },
  async history(id: number, admin = false) {
    return (await apiClient.get<CandyRun[]>(`${admin ? '/admin' : ''}/candy-tests/${id}/history`)).data
  },
  async save(config: CandyConfig) {
    return (await apiClient.post<CandyConfig>('/admin/candy-tests', config)).data
  },
  async run(id: number) {
    return (await apiClient.post<{ run_id: number }>(`/admin/candy-tests/${id}/run`)).data
  }
}

export const candyStatusLabels: Record<CandyStatus, string> = {
  queued: '等待测试', running: '正在回答', passed: '答案通过', failed: '答案未通过',
  inconclusive: '待核对', error: '请求失败', cancelled: '已取消'
}
export const candyEffortLabels: Record<string, string> = { low: '轻度', medium: '中等', high: '高', xhigh: '极高' }
