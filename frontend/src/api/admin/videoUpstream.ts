import { apiClient } from '../client'

export interface VideoMarkup { output_per_second: number; reference_per_second: number; per_request: number }
export interface VideoSpec {
  generic?: boolean; reference_price_pending?: boolean
  supported: boolean; reason?: string; min_seconds: number; max_seconds: number; seconds?: number[]
  resolutions: string[]; ratios: string[]; min_images: number; max_images: number; max_videos: number; max_audios: number
  max_reference_seconds: number; max_combined_seconds?: number; requires_visual_or_audio?: boolean
  allow_audio_only?: boolean; reference_billed?: boolean; face: boolean; generate_audio: boolean
}
export interface VideoSupplierPrice {
  model: string; billing_mode: 'per_request' | 'video'; unit_price: number; reference_per_second: number
  reference_price_known?: boolean
  resolution_prices?: Record<string, number>; spec?: VideoSpec; unavailable_reason?: string
}
export interface VideoModelConfig {
  account_synced?: boolean
  price_source?: 'upstream' | 'manual'
  auto_price: boolean; markup?: VideoMarkup; follow_increase?: boolean; follow_decrease?: boolean
  reference_per_second: number; resolution_prices?: Record<string, number>; spec: VideoSpec
  cost?: VideoSupplierPrice; synced_at?: string
}
export interface VideoRuntimeSettings {
  auto_import_account_models?: boolean
  auto_sync: boolean; follow_increase: boolean; follow_decrease: boolean; interval_seconds: number; markup: VideoMarkup
  reference_retention_hours: number; max_concurrent_tasks: number; reference_user_mb: number; reference_total_mb: number
}
export interface VideoCatalog {
  version: string; last_attempt: string; last_success: string; last_error: string
  models: Record<string, VideoSupplierPrice>
  changes: { at: string; group_id: number; model: string; old: number; new: number; old_reference: number; new_reference: number }[]
}
export const videoUpstreamAPI = {
  get: async () => (await apiClient.get<{ settings: VideoRuntimeSettings; catalog: VideoCatalog }>('/admin/video-upstream')).data,
  save: async (settings: VideoRuntimeSettings) => (await apiClient.put('/admin/video-upstream', settings)).data,
  probe: async () => (await apiClient.post<VideoCatalog>('/admin/video-upstream/probe', {}, { timeout: 30000 })).data,
  sync: async () => (await apiClient.post<VideoCatalog>('/admin/video-upstream/sync', {}, { timeout: 30000 })).data,
  import: async (group_id: number, models: string[]) => (await apiClient.post('/admin/video-upstream/import', { group_id, models })).data,
}
