import { describe, expect, it } from 'vitest'
import { defaultModelBillingConfig, newModelBillingRule, modelBillingConfigError, modelBillingPayload, normalizeModelBillingConfig } from '../modelBilling'
describe('scoped model billing', () => {
 const make = () => ({ schema_version: 2, enabled: true, rules: [{ ...newModelBillingRule(), group_id: 2, model: 'gpt-test' }] })
 it('has no enabled default charge', () => { expect(defaultModelBillingConfig()).toEqual({ schema_version: 2, enabled: false, rules: [] }); expect(normalizeModelBillingConfig()).toEqual(defaultModelBillingConfig()) })
 it.each(['', '*', 'a*b', 'x y'])('rejects invalid model %s', model => { const c=make(); c.rules[0].model=model; expect(modelBillingConfigError(c)).not.toBeNull() })
 it('requires explicit group and unique group/model pair', () => { const c=make(); expect(modelBillingConfigError(c)).toBeNull(); c.rules[0].group_id=0; expect(modelBillingConfigError(c)).not.toBeNull(); c.rules[0].group_id=2; c.rules.push({...c.rules[0]}); expect(modelBillingConfigError(c)).not.toBeNull(); c.rules[1].group_id=3; expect(modelBillingConfigError(c)).toBeNull() })
 it('validates context threshold and normalizes always mode', () => { const c=make(); c.rules[0].threshold_tokens=0; expect(modelBillingConfigError(c)).not.toBeNull(); c.rules[0].mode='always'; expect(modelBillingConfigError(c)).toBeNull(); expect(modelBillingPayload(c).rules[0].threshold_tokens).toBe(0) })
})
