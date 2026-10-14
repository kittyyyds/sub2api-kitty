import apiClient from '../client'
export interface ContextRule {
 id: number; revision: number; model: string; platform: string; threshold_tokens: number
 input_multiplier: number; output_multiplier: number; cache_read_multiplier: number; cache_write_multiplier: number
 enabled: boolean; updated_at?: string
}
export const contextPricing = {
 list: async () => (await apiClient.get<ContextRule[]>('/admin/channels/context-rules')).data,
 save: async (rule: ContextRule) => (await apiClient.put<ContextRule>('/admin/channels/context-rules', rule)).data,
 remove: async (rule: ContextRule) => apiClient.delete(`/admin/channels/context-rules/${rule.id}`, { params: { revision: rule.revision } }),
 models: async (platform: string) => (await apiClient.get<{models: string[]}>('/admin/channels/context-rule-models', {params:{platform}})).data.models
}
