import { apiClient } from '../client'

export interface ImageRuntimeSettings {
  auto_sync: boolean
  auto_import_account_models: boolean
  follow_increase: boolean
  follow_decrease: boolean
  interval_seconds: number
  markup: number
}
export interface ImageSupplierPrice {
  model: string
  unit_price: number
  description?: string
  billing_mode: string
}
export interface ImageModelConfig {
  provider: string
  upstream_model: string
  auto_price: boolean
  cost: number
  description?: string
  applied_markup: number
  applied_prices: Record<string, number>
}
export interface ImageCatalog {
  version: string
  last_attempt: string
  last_success: string
  last_error: string
  models: Record<string, ImageSupplierPrice>
}
export interface ImagePricingGroup {
  id: number
  name: string
  pricing: { models: string[]; image?: ImageModelConfig }[]
  prices: Record<string, Record<string, number>>
}
export interface ImageAdminState {
  settings: ImageRuntimeSettings
  catalog: ImageCatalog
  groups: ImagePricingGroup[]
}
export const imageUpstreamAPI = {
  get: async () => (await apiClient.get<ImageAdminState>('/admin/image-upstream')).data,
  save: async (settings: ImageRuntimeSettings) => (await apiClient.put('/admin/image-upstream', settings)).data,
  sync: async (apply = false) => (await apiClient.post<ImageCatalog>('/admin/image-upstream/sync', {}, { params: { apply }, timeout: 30000 })).data,
  price: async (group_id: number, model: string, price: number | null) => (await apiClient.put('/admin/image-upstream/model-price', { group_id, model, price })).data,
}
