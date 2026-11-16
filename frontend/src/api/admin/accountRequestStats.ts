import { apiClient } from '../client'

export interface OpsAccountRequestStats {
  success_rate: number | null
  success_requests: number
  error_requests: number
  request_samples: number
  window_start: string
  window_end: string
  cache_rate: number | null
  avg_first_token_ms: number | null
  avg_duration_ms: number | null
  cache_samples: number
  first_token_samples: number
  duration_samples: number
}
export async function getAccountRequestStats(accountIds: number[], signal?: AbortSignal): Promise<Record<string, OpsAccountRequestStats>> {
  return (await apiClient.post<{ stats: Record<string, OpsAccountRequestStats> }>('/admin/ops/account-request-stats/batch', { account_ids: accountIds }, { signal })).data.stats
}
