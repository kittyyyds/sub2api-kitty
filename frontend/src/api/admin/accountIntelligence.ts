import { apiClient } from '../client'
export interface AccountIntelligenceTest {
 id: string; request_id: string; account_id: number; source?: 'manual' | 'scheduled'; kind: 'candy' | 'custom'; model: string; reasoning_effort: string;
 prompt: string; expected_answer?: string; scoring_version: string;
 status: 'running' | 'correct' | 'incorrect' | 'completed' | 'error'; response: string; error?: string;
 upstream_model?: string; latency_ms: number; created_at: string; finished_at?: string;
}
export interface AccountIntelligenceInput { request_id: string; kind: 'candy' | 'custom'; model: string; reasoning_effort: string; prompt: string }
export interface AccountIntelligenceSchedule {
 account_id: number; revision: number; enabled: boolean; kind: 'candy' | 'custom'; model: string; reasoning_effort: string; prompt: string;
 cron_expression: string; max_results: number; timezone: string; next_run_at: string | null; last_attempt_at: string | null; last_error: string;
}
export const accountIntelligence = {
 schedule: async (id: number): Promise<AccountIntelligenceSchedule> => (await apiClient.get(`/admin/accounts/${id}/intelligence-schedule`)).data,
 saveSchedule: async (id: number, input: AccountIntelligenceSchedule): Promise<AccountIntelligenceSchedule> => (await apiClient.put(`/admin/accounts/${id}/intelligence-schedule`, input)).data,
 history: async (id: number, offset = 0): Promise<AccountIntelligenceTest[]> => (await apiClient.get(`/admin/accounts/${id}/intelligence-tests`, { params: { limit: 50, offset } })).data,
 recent: async (ids: number[]): Promise<AccountIntelligenceTest[]> => (await apiClient.get('/admin/accounts/intelligence-tests/recent', { params: { ids: ids.join(',') } })).data,
 start: async (id: number, input: AccountIntelligenceInput): Promise<AccountIntelligenceTest> => (await apiClient.post(`/admin/accounts/${id}/intelligence-tests`, input)).data,
}
export const intelligenceColor = (status: string) => ({correct: 'bg-emerald-500', incorrect: 'bg-amber-400', error: 'bg-red-500', running: 'animate-pulse bg-blue-400', completed: 'bg-slate-400'}[status] || 'bg-slate-300')
