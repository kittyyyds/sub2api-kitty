import { apiClient } from './client'

export interface BillingCatalogModel {
  name: string
  platform: string
}

export interface BillingCatalogGroup {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  peak_multiplier: number
  effective_rate_multiplier: number
  rate_source: 'group' | 'user'
  models: BillingCatalogModel[]
}

export interface BillingCatalogResponse {
  groups: BillingCatalogGroup[]
}

export interface BillingPreviewRequest {
  group_id: number
  model: string
  model_platform: string
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  cache_creation_5m_tokens: number
  cache_creation_1h_tokens: number
  cache_creation_30m_tokens?: number
  cache_creation_12h_tokens?: number
  service_tier: '' | 'priority' | 'flex'
}

export interface BillingPreviewBreakdown {
  input_cost: number
  output_cost: number
  cache_creation_cost: number
  cache_read_cost: number
}

export interface BillingPreviewResult {
  group_id: number
  group_name: string
  model: string
  model_platform: string
  pricing_source: 'group' | 'channel' | 'litellm' | 'fallback'
  billing_mode: 'token'
  rate_multiplier: number
  peak_multiplier: number
  effective_rate_multiplier: number
  standard_cost: number
  actual_cost: number
  breakdown: BillingPreviewBreakdown
  charged_breakdown: BillingPreviewBreakdown
  long_context_applied: boolean
}

export async function getBillingCatalog(options?: { signal?: AbortSignal }): Promise<BillingCatalogResponse> {
  const { data } = await apiClient.get<BillingCatalogResponse>('/billing/catalog', {
    signal: options?.signal,
  })
  return data
}

export async function previewBilling(
  request: BillingPreviewRequest,
  options?: { signal?: AbortSignal },
): Promise<BillingPreviewResult> {
  const { data } = await apiClient.post<BillingPreviewResult>('/billing/preview', request, {
    signal: options?.signal,
  })
  return data
}

export const billingPreviewAPI = {
  getCatalog: getBillingCatalog,
  preview: previewBilling,
}

export default billingPreviewAPI
