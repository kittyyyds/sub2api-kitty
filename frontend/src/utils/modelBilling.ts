export interface ModelBillingRule {
  group_id: number
  model: string
  enabled: boolean
  mode: 'always' | 'context'
  threshold_tokens: number
  multiplier: number
}
export interface ModelBillingConfig {
  schema_version: number
  enabled: boolean
  rules: ModelBillingRule[]
}
export function defaultModelBillingConfig(): ModelBillingConfig {
  return { schema_version: 2, enabled: false, rules: [] }
}
export function newModelBillingRule(): ModelBillingRule {
  return { group_id: 0, model: '', enabled: true, mode: 'context', threshold_tokens: 50000, multiplier: 2 }
}
export function normalizeModelBillingConfig(c?: ModelBillingConfig): ModelBillingConfig {
  return c?.schema_version === 2 ? c : defaultModelBillingConfig()
}
export function modelBillingConfigError(config: ModelBillingConfig): string | null {
  if (config.schema_version !== 2 || config.rules.length > 100 || (config.enabled && !config.rules.length)) return 'autoConfig.modelBilling.invalid'
  const seen = new Set<string>()
  for (const rule of config.rules) {
    const model = rule.model.trim().toLowerCase(), key = rule.group_id + ':' + model
    if (!Number.isSafeInteger(rule.group_id) || rule.group_id <= 0 || !/^[a-z0-9_./:-]+\*?$/.test(model) || model.length > 200 || seen.has(key) ||
      !Number.isFinite(rule.multiplier) || rule.multiplier < 1 || rule.multiplier > 1000 ||
      !['always', 'context'].includes(rule.mode) || (rule.mode === 'context' && (!Number.isSafeInteger(rule.threshold_tokens) || rule.threshold_tokens < 1 || rule.threshold_tokens > 100000000))) return 'autoConfig.modelBilling.invalid'
    seen.add(key)
  }
  return null
}
export function modelBillingPayload(config: ModelBillingConfig): ModelBillingConfig {
  return { schema_version: 2, enabled: config.enabled, rules: config.rules.map(rule => ({ ...rule, model: rule.model.trim(), threshold_tokens: rule.mode === 'always' ? 0 : rule.threshold_tokens })) }
}
