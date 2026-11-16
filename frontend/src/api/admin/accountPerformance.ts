import { apiClient } from '../client'
import type { AccountPerformance, PerformanceConfig, PerformanceEvent } from '@/types/accountPerformance'

export const accountPerformanceAPI = {
  async getPerformance(accountIds: number[], signal?: AbortSignal): Promise<Record<number, AccountPerformance>> {
    return (await apiClient.post<Record<number, AccountPerformance>>('/admin/accounts/performance', { account_ids: accountIds }, { signal })).data
  },
  async savePerformance(id: number, config: PerformanceConfig, revision: number) {
    await apiClient.put(`/admin/accounts/${id}/performance`, { config, revision })
  },
  async releasePerformance(id: number, revision: number, pauseGeneration: number) {
    await apiClient.post(`/admin/accounts/${id}/performance/release`, { revision, pause_generation: pauseGeneration })
  },
  async getPerformanceEvents(id: number): Promise<PerformanceEvent[]> {
    return (await apiClient.get<PerformanceEvent[]>(`/admin/accounts/${id}/performance/events`)).data
  }
}
