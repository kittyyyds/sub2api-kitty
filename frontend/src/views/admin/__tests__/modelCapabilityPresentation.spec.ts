import { describe, expect, it } from 'vitest'
import { normalizeCapabilityIdentityStatus } from '../modelCapabilityPresentation'

describe('model capability identity presentation', () => {
  it('maps the persisted protocol mismatch state to the summary warning state', () => {
    expect(normalizeCapabilityIdentityStatus('mismatch')).toBe('mismatch_suspected')
  })

  it('preserves public states and defaults missing values to unverified', () => {
    expect(normalizeCapabilityIdentityStatus('self_report_consistent')).toBe('self_report_consistent')
    expect(normalizeCapabilityIdentityStatus('mismatch_suspected')).toBe('mismatch_suspected')
    expect(normalizeCapabilityIdentityStatus()).toBe('unverified')
  })
})
