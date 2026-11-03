import type { PelicanTestConfig, QualityJudgeConfig, QualityPolicy, ScheduledTestPlan, UpdateScheduledTestPlanRequest } from '@/types'

export function qualityJudgeConfigured(judge?: QualityJudgeConfig): judge is QualityJudgeConfig {
  if (!judge?.prompt.trim()) return false
  if (judge.kind === 'jev') return !judge.group_id && !judge.model_id
  return (!judge.kind || judge.kind === 'llm') && judge.group_id > 0 && !!judge.model_id.trim()
}


// Copy reusable inputs only. Result snapshots and future server-only fields must
// never enter a create request through a spread of the source configuration.
export function copyQualityRuleConfig(source: PelicanTestConfig): PelicanTestConfig & { quality: QualityPolicy } {
  const policy: QualityPolicy = source.quality || { action: 'observe_only', expected_answer: '', remove_group_ids: [], auto_restore: false }
  if (policy.action === 'openai_two_fa_admission') throw new Error('qualityOps.copyUnsupported')
  const action = policy.action || 'observe_only'
  return {
    rule_name: source.rule_name,
    question_kind: source.question_kind === 'random_reasoning' ? 'random_reasoning' : 'candy',
    prompt: source.question_kind === 'random_reasoning' ? '' : source.prompt,
    reasoning_effort: source.reasoning_effort,
    parallel_count: source.parallel_count,
    model_ids: source.model_ids ? [...source.model_ids] : undefined,
    quality: {
      trigger_on_upstream_5xx: policy.trigger_on_upstream_5xx ?? false,
      expected_answer: source.question_kind === 'random_reasoning' ? '' : policy.expected_answer,
      action,
      remove_group_ids: qualityUsesGroups(action) ? [...(policy.remove_group_ids || [])] : [],
      remove_models: action === 'remove_models' ? [...(policy.remove_models || [])] : undefined,
      recovery_concurrency: action === 'remove_models' ? policy.recovery_concurrency : undefined,
      auto_restore: action === 'observe_only' ? false : policy.auto_restore ?? false,
      judge: policy.judge ? { ...(policy.judge.kind ? { kind: policy.judge.kind } : {}), group_id: policy.judge.group_id, model_id: policy.judge.model_id, prompt: policy.judge.prompt } : undefined,
    },
  }
}

export function qualityUsesGroups(action?: QualityPolicy['action']) {
  return action === 'remove_groups' || action === 'isolate_models'
}

export function qualityUsesModelResults(action?: QualityPolicy['action']) {
  return action === 'remove_models' || action === 'isolate_models'
}

// Used by single-account creation, templates and bulk editing. Action cleanup
// and validation belong here so all entry points preserve the same semantics.
export function qualityActionPayload(policy: QualityPolicy): QualityPolicy {
  const next = { ...(policy as QualityPolicy & { bps?: unknown }), remove_group_ids: qualityUsesGroups(policy.action) ? [...policy.remove_group_ids] : [] }
  delete next.bps
  if (qualityUsesGroups(next.action) && !next.remove_group_ids.length) throw new Error('qualityOps.selectGroups')
  if (next.action === 'remove_models') {
    next.remove_models = [...(policy.remove_models || [])]
    if (!next.remove_models.length) throw new Error('qualityOps.selectModels')
    const cap = policy.recovery_concurrency ?? 5
    if (!Number.isInteger(cap) || cap < 1 || cap > 10000) throw new Error('qualityOps.invalidRecoveryConcurrency')
    next.recovery_concurrency = cap
  } else { delete next.remove_models; delete next.recovery_concurrency }
  if (next.action === 'observe_only') next.auto_restore = false
  return next
}

export const qualityRuleFields = ['model', 'schedule', 'test', 'action', 'restore', 'retention', 'trigger', 'enabled'] as const
export type QualityRuleField = typeof qualityRuleFields[number]
export type QualityRuleDraft = {
  model_id: string
  cron_expression: string
  max_results?: number
  enabled: boolean
  pelican_config: PelicanTestConfig & { quality: QualityPolicy }
}

// Compare only the business fields owned by each bulk checkbox. Dependencies
// between fields must be confirmed too when importing a configuration.
export function qualityRuleFieldValue(rule: Pick<ScheduledTestPlan, 'model_id' | 'cron_expression' | 'enabled' | 'pelican_config'> & { max_results?: number }, field: QualityRuleField): unknown {
  const cfg = rule.pelican_config, q = cfg?.quality
  switch (field) {
    case 'model': return [...new Set([rule.model_id, ...(cfg?.model_ids || [])])]
    case 'schedule': return rule.cron_expression
    case 'enabled': return rule.enabled
    case 'retention': return rule.max_results
    case 'trigger': return q?.trigger_on_upstream_5xx ?? false
    case 'test': return [cfg?.question_kind, cfg?.prompt, cfg?.reasoning_effort, cfg?.parallel_count,
      q?.expected_answer, q?.judge?.kind || 'llm', q?.judge?.group_id, q?.judge?.model_id, q?.judge?.prompt]
    case 'restore': return q?.auto_restore
    case 'action': return [q?.action, q?.remove_group_ids || [], q?.remove_models || [], q?.recovery_concurrency]
  }
}

// The API replaces pelican_config as a whole. Merge selected fields into each
// rule's own configuration, never into a shared copy of the first rule.
export function buildQualityRulePatch(
  plan: ScheduledTestPlan,
  draft: QualityRuleDraft,
  fields: readonly QualityRuleField[],
): UpdateScheduledTestPlanRequest {
  const patch: UpdateScheduledTestPlanRequest = {}
  if (!fields.length) throw new Error('qualityOps.selectBulkFields')
  if (fields.includes('model')) {
    if (!draft.model_id.trim()) throw new Error('qualityOps.modelRequired')
    patch.model_id = draft.model_id.trim()
  }
  if (fields.includes('schedule')) {
    if (!draft.cron_expression.trim()) throw new Error('qualityOps.scheduleRequired')
    patch.cron_expression = draft.cron_expression.trim()
  }
  if (fields.includes('retention')) {
    if (!Number.isInteger(draft.max_results) || draft.max_results! < 1 || draft.max_results! > 200) throw new Error('qualityOps.invalidMaxResults')
    patch.max_results = draft.max_results
  }
  if (fields.includes('enabled')) patch.enabled = draft.enabled
  if (fields.includes('retention') && !fields.includes('test') && !fields.includes('model') && (plan.pelican_config?.model_ids?.length || 1) * (plan.pelican_config?.parallel_count || 1) > patch.max_results!) throw new Error('qualityOps.retentionTooSmall')
  if (!fields.some(field => ['test', 'action', 'restore', 'trigger'].includes(field)) &&
    !(fields.includes('model') && (draft.pelican_config.model_ids?.length || plan.pelican_config?.model_ids?.length))) return patch

  if (!plan.pelican_config?.quality) throw new Error('qualityOps.ruleConfigMissing')
  const config: PelicanTestConfig & { quality: QualityPolicy } = JSON.parse(JSON.stringify(plan.pelican_config))
  const source = draft.pelican_config
  if (fields.includes('model')) {
    config.model_ids = [...new Set([draft.model_id.trim(), ...(source.model_ids || []).map(model => model.trim())])]
    if (config.model_ids.some(model => !model || model.length > 100)) throw new Error('qualityOps.modelRequired')
  }
  if (fields.includes('test')) {
    config.question_kind = source.question_kind === 'random_reasoning' ? 'random_reasoning' : 'candy'
    delete config.test_channel
    config.reasoning_effort = source.reasoning_effort
    const random = source.question_kind === 'random_reasoning'
    delete config.random_question
    config.prompt = random ? '' : source.prompt
    config.parallel_count = source.parallel_count
    config.quality.expected_answer = random ? '' : source.quality.expected_answer
    const judge = copyQualityRuleConfig(source).quality.judge
    if (!qualityJudgeConfigured(judge)) throw new Error('qualityOps.configureJudge')
    if (!random && (!config.prompt.trim() || !config.quality.expected_answer.trim())) throw new Error('qualityOps.questionRequired')
    config.quality.judge = { ...judge }
  }
  if (fields.includes('trigger')) config.quality.trigger_on_upstream_5xx = source.quality.trigger_on_upstream_5xx ?? false
  if (fields.includes('action')) {
    config.quality = qualityActionPayload({ ...config.quality,
      action: source.quality.action, remove_group_ids: source.quality.remove_group_ids,
      remove_models: source.quality.remove_models, recovery_concurrency: source.quality.recovery_concurrency,
    })
  }

  if (config.quality.action === 'observe_only') {
    config.quality.remove_group_ids = []
    config.quality.auto_restore = false
  }
  if (fields.includes('restore') && config.quality.action !== 'observe_only') config.quality.auto_restore = source.quality.auto_restore
  if (config.quality.action === 'isolate_models' && config.question_kind !== 'candy' && config.question_kind !== 'random_reasoning') throw new Error('qualityOps.isolationRequiresCandy')
  const samples = (config.model_ids?.length || 1) * config.parallel_count
  if (samples > 100 || (config.model_ids?.length || 0) > 50) throw new Error('qualityOps.tooManyModelSamples')
  if (fields.includes('retention') && samples > patch.max_results!) throw new Error('qualityOps.retentionTooSmall')
  if (!fields.includes('retention') && samples > plan.max_results) patch.max_results = samples
  patch.pelican_config = config
  return patch
}
