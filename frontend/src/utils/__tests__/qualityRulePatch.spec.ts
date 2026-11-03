import { describe, expect, it } from 'vitest'
import { buildQualityRulePatch, copyQualityRuleConfig, qualityActionPayload, qualityJudgeConfigured, qualityRuleFieldValue, type QualityRuleDraft } from '../qualityRulePatch'
import type { ScheduledTestPlan } from '@/types'

const draft = (): QualityRuleDraft => ({
  model_id: 'new-model', cron_expression: '0 * * * *', enabled: false,
  pelican_config: { question_kind: 'candy', prompt: 'New question', reasoning_effort: 'high', parallel_count: 3,
    quality: { expected_answer: '42', action: 'remove_groups', remove_group_ids: [21], auto_restore: true,
      judge: { group_id: 21, model_id: 'new-judge', prompt: 'Grade the answer' } } },
})
const plan = (): ScheduledTestPlan => ({
  id: 1, account_id: 10, model_id: 'old-model', cron_expression: '*/30 * * * *', enabled: true,
  max_results: 150, auto_recover: false, last_run_at: null, next_run_at: null, created_at: '', updated_at: '',
  pelican_config: { question_kind: 'candy', prompt: 'Own question', reasoning_effort: 'low', parallel_count: 2,
    quality: { expected_answer: '7', action: 'remove_groups', remove_group_ids: [99], auto_restore: false,
      judge: { group_id: 99, model_id: 'own-judge', prompt: 'Own grading instructions' } } },
})

describe('quality rule copy inputs', () => {
  it('retains Jev choice in copy and bulk patches without importing arbitrary fields', () => {
    const source = draft()
    source.pelican_config.quality.judge = { kind: 'jev', group_id: 0, model_id: '', prompt: 'Compare values' }
    Object.assign(source.pelican_config.quality.judge, { api_key: 'synthetic-secret', base_url: 'https://example.com?key=secret' })
    const copy = copyQualityRuleConfig(source.pelican_config)
    expect(copy.quality.judge).toEqual({ kind: 'jev', group_id: 0, model_id: '', prompt: 'Compare values' })
    expect(qualityJudgeConfigured(copy.quality.judge)).toBe(true)
    const patch = buildQualityRulePatch(plan(), source, ['test'])
    expect(patch.pelican_config!.quality!.judge).toEqual(copy.quality.judge)
    expect(JSON.stringify(patch)).not.toContain('synthetic-secret')
    expect(qualityRuleFieldValue({ ...plan(), pelican_config: copy }, 'test')).not.toEqual(qualityRuleFieldValue(plan(), 'test'))
    expect(qualityJudgeConfigured({ kind: 'jev', group_id: 21, model_id: 'chat', prompt: 'grade' })).toBe(false)
    expect(qualityJudgeConfigured({ group_id: 21, model_id: 'chat', prompt: 'grade' })).toBe(true)
    expect(qualityJudgeConfigured({ kind: 'llm', group_id: 0, model_id: '', prompt: 'grade' })).toBe(false)
  })
  it('copies allowlisted fields and keeps nested values independent', () => {
    const source = plan().pelican_config!
    source.model_ids = ['first', 'second']
    source.quality!.action = 'remove_models'
    source.quality!.remove_models = ['first']
    source.quality!.recovery_concurrency = 9
    Object.assign(source.quality!, { bps: { models: ['legacy-model'] }, owner_plan_id: 1 })
    const original = JSON.stringify(source)
    const copy = copyQualityRuleConfig(source)
    expect(copy).toMatchObject({ prompt: source.prompt, reasoning_effort: 'low', parallel_count: 2, model_ids: ['first', 'second'],
      quality: { action: 'remove_models', remove_models: ['first'], recovery_concurrency: 9, judge: source.quality!.judge } })
    expect(copy.quality).not.toHaveProperty('bps')
    expect(copy.quality).not.toHaveProperty('owner_plan_id')
    copy.model_ids!.push('third')
    copy.quality.remove_group_ids.push(100)
    copy.quality.remove_models!.push('second')
    copy.quality.judge!.prompt = 'Changed'
    expect(JSON.stringify(source)).toBe(original)
  })

  it('drops generated questions, result snapshots and unknown nested fields', () => {
    const source = plan().pelican_config!
    source.question_kind = 'random_reasoning'
    source.random_question = { id: 'previous-question', family: 'paths', version: 'v1' }
    Object.assign(source, { trigger_source: 'upstream_5xx', model_id: 'result-model', quality_round_id: 'round',
      quality_model_actions: { first: 'cooled' }, quality_model_outcomes: { first: 'failed' },
      quality_restore_diagnostic: { ownership_retained: true }, credentials: 'synthetic-secret' })
    Object.assign(source.quality!, { admission: { enabled: true }, owner_plan_id: 1, failure_count: 3 })
    Object.assign(source.quality!.judge!, { credentials: 'synthetic-secret' })
    const copy = copyQualityRuleConfig(source)
    expect(copy.prompt).toBe('')
    expect(copy.quality.expected_answer).toBe('')
    expect(copy.quality.judge).toEqual({ group_id: 99, model_id: 'own-judge', prompt: 'Own grading instructions' })
    expect(copy).not.toHaveProperty('random_question')
    expect(copy).not.toHaveProperty('credentials')
  })
})

describe('quality rule partial updates', () => {
  it('bulk switches to generated questions without requiring fixed answers', () => {
    const input = draft()
    input.pelican_config.question_kind = 'random_reasoning'
    input.pelican_config.prompt = ''
    input.pelican_config.quality.expected_answer = ''
    input.pelican_config.quality.action = 'isolate_models'
    const updated = buildQualityRulePatch(plan(), input, ['test', 'action'])
    expect(updated.pelican_config).toMatchObject({ question_kind: 'random_reasoning', prompt: '', parallel_count: 3,
      quality: { expected_answer: '', action: 'isolate_models', judge: input.pelican_config.quality.judge } })
    expect(updated.pelican_config).not.toHaveProperty('random_question')
  })

  it('sends only selected top-level fields, including false values', () => {
    expect(buildQualityRulePatch(plan(), draft(), ['model', 'schedule', 'enabled'])).toEqual({
      model_id: 'new-model', cron_expression: '0 * * * *', enabled: false,
    })
  })

  it('replaces the full model selection without overwriting the rule grading policy', () => {
    const source = draft()
    Object.assign(source.pelican_config, { model_ids: ['second-model', 'second-model'] })
    const updated = buildQualityRulePatch(plan(), source, ['model'])
    expect(updated.model_id).toBe('new-model')
    expect(updated.pelican_config).toMatchObject({ model_ids: ['new-model', 'second-model'],
      quality: { expected_answer: '7', judge: { model_id: 'own-judge' } } })
  })

  it('preserves each rule’s own question, judge and targets when changing restoration', () => {
    const first = plan(), second = plan()
    second.pelican_config!.quality!.remove_group_ids = [100]
    second.pelican_config!.quality!.judge!.model_id = 'another-judge'
    const before = JSON.stringify([first, second])
    for (const rule of [first, second]) {
      expect(buildQualityRulePatch(rule, draft(), ['restore'])).toEqual({
        pelican_config: { ...rule.pelican_config, quality: { ...rule.pelican_config!.quality, auto_restore: true } },
      })
    }
    expect(JSON.stringify([first, second])).toBe(before)
  })

  it('does not require untouched grading configuration when changing another field', () => {
    const input = draft()
    delete input.pelican_config.quality.judge
    input.pelican_config.quality.remove_group_ids = []
    expect(buildQualityRulePatch(plan(), input, ['enabled'])).toEqual({ enabled: false })
    expect(() => buildQualityRulePatch(plan(), input, ['test'])).toThrow('qualityOps.configureJudge')
    expect(() => buildQualityRulePatch(plan(), input, ['action'])).toThrow('qualityOps.selectGroups')
    expect(() => buildQualityRulePatch(plan(), input, [])).toThrow('qualityOps.selectBulkFields')
  })

  it('strips legacy BPS policy fields when updating a quality action', () => {
    const input = draft()
    Object.assign(input.pelican_config.quality, { bps: { enabled: true } })
    const updated = qualityActionPayload(input.pelican_config.quality)
    expect(updated).not.toHaveProperty('bps')
    expect(updated.action).toBe('remove_groups')
  })

  it('requires groups for group removal and model targets for cooldowns', () => {
    const input = draft()
    input.pelican_config.quality.remove_group_ids = []
    expect(() => qualityActionPayload(input.pelican_config.quality)).toThrow('qualityOps.selectGroups')
    input.pelican_config.quality.action = 'remove_models'
    input.pelican_config.quality.remove_models = ['gpt-6-astra']
    expect(qualityActionPayload(input.pelican_config.quality)).toMatchObject({ remove_models: ['gpt-6-astra'], recovery_concurrency: 5 })
  })
})

describe('composed model isolation', () => {
  it('uses tested models and selected groups without legacy cooldown fields', () => {
    const input = draft()
    input.pelican_config.quality.action = 'isolate_models'
    input.pelican_config.quality.remove_models = ['stale-target']
    input.pelican_config.quality.recovery_concurrency = 2
    input.pelican_config.model_ids = ['sol', 'astra']
    const patch = buildQualityRulePatch(plan(), input, ['action', 'model'])
    expect(patch.pelican_config).toMatchObject({ model_ids: ['new-model', 'sol', 'astra'], quality: {
      action: 'isolate_models', remove_group_ids: [21], expected_answer: '7', auto_restore: false,
    } })
    expect(patch.pelican_config!.quality).not.toHaveProperty('remove_models')
    expect(patch.pelican_config!.quality).not.toHaveProperty('recovery_concurrency')
    expect(patch.pelican_config!.quality).not.toHaveProperty('bps')
  })

  it('requires explicit groups and a supported answer question', () => {
    const input = draft()
    input.pelican_config.quality.action = 'isolate_models'
    input.pelican_config.quality.remove_group_ids = []
    expect(() => buildQualityRulePatch(plan(), input, ['action'])).toThrow('qualityOps.selectGroups')
    input.pelican_config.quality.remove_group_ids = [21]
    const htmlRule = plan()
    htmlRule.pelican_config!.question_kind = 'pelican'
    expect(() => buildQualityRulePatch(htmlRule, input, ['action'])).toThrow('qualityOps.isolationRequiresCandy')
  })
})


describe('complete template selected fields', () => {
  it('imports retention and 5xx trigger only with their explicit bulk checkboxes', () => {
    const source = { ...draft(), max_results: 173 }
    source.pelican_config.quality.trigger_on_upstream_5xx = true
    const untouched = buildQualityRulePatch(plan(), source, ['enabled'])
    expect(untouched).toEqual({ enabled: false })
    const retention = buildQualityRulePatch(plan(), source, ['retention'])
    expect(retention).toEqual({ max_results: 173 })
    const triggered = buildQualityRulePatch(plan(), source, ['trigger'])
    expect(triggered.pelican_config!.quality!.trigger_on_upstream_5xx).toBe(true)
    expect(triggered.pelican_config!.quality!.action).toBe(plan().pelican_config!.quality!.action)
    expect(triggered).not.toHaveProperty('max_results')
  })
  it('rejects retention too small for the targets existing samples when only retention is selected', () => {
    const target = plan(); target.pelican_config!.model_ids = ['first', 'second']; target.pelican_config!.parallel_count = 3
    expect(() => buildQualityRulePatch(target, { ...draft(), max_results: 5 }, ['retention'])).toThrow('qualityOps.retentionTooSmall')
    expect(buildQualityRulePatch(target, { ...draft(), max_results: 6 }, ['retention'])).toEqual({ max_results: 6 })
  })
  it('never inherits a punitive action for old presets and removes hidden restrictions', () => {
    const source = draft().pelican_config
    delete (source.quality as any).action
    const copied = copyQualityRuleConfig(source)
    expect(copied.quality.action).toBe('observe_only'); expect(copied.quality.remove_group_ids).toEqual([]); expect(copied.quality.auto_restore).toBe(false)
    expect(copied.quality.remove_models).toBeUndefined(); expect(copied.quality.bps).toBeUndefined()
  })
})
