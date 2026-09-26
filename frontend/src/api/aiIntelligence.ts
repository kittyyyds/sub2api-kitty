import { apiClient } from './client'

export type AIServiceStatusValue =
  | 'operational'
  | 'under_maintenance'
  | 'degraded_performance'
  | 'partial_outage'
  | 'major_outage'
  | 'unknown'

export interface AIIntelligencePoint {
  model: string
  effort: string
  iq: number
  passed: number
  valid_tasks: number
  average_price_usd: number
  average_minutes: number
  latest_graded_at: string
  recommendation_score: number
}

export interface AIServiceComponent {
  key: 'api' | 'web' | 'desktop'
  status: AIServiceStatusValue
}

export interface AIServiceDay {
  date: string
  status: AIServiceStatusValue
}

export interface AIServiceIncident {
  title: string
  status: AIServiceStatusValue
  occurred_at: string
}

export interface AIServiceStatus {
  overall_status: AIServiceStatusValue
  components: readonly AIServiceComponent[]
  days: readonly AIServiceDay[]
  event_free_days: number
  window_days: number
  recent_incident?: AIServiceIncident
}

export interface AIIntelligencePayload {
  points: readonly AIIntelligencePoint[]
  service_status: AIServiceStatus
  fetched_at: string
  source_updated_at: string
  stale: boolean
}

export async function getAIIntelligence(options?: { signal?: AbortSignal; forceRefresh?: boolean }): Promise<AIIntelligencePayload> {
  const { data } = await apiClient.get<AIIntelligencePayload>('/public/ai-intelligence', {
    signal: options?.signal,
    params: options?.forceRefresh ? { refresh: Date.now() } : undefined,
    headers: options?.forceRefresh ? { 'Cache-Control': 'no-cache' } : undefined,
  })
  return data
}

export const aiIntelligenceAPI = {
  get: getAIIntelligence,
}

export default aiIntelligenceAPI
