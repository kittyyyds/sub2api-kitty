import { describe, expect, it } from 'vitest'
import {
  normalizeCapabilityIdentityStatus,
  passiveCapabilityStatus,
  passiveConfidence,
  passiveIdentityStatus,
} from '../modelCapabilityPresentation'

describe('model capability identity presentation', () => {
  it('maps the persisted protocol mismatch state to the summary warning state', () => {
    expect(normalizeCapabilityIdentityStatus('mismatch')).toBe('mismatch_suspected')
  })

  it('preserves public states and defaults missing values to unverified', () => {
    expect(normalizeCapabilityIdentityStatus('self_report_consistent')).toBe('self_report_consistent')
    expect(normalizeCapabilityIdentityStatus('mismatch_suspected')).toBe('mismatch_suspected')
    expect(normalizeCapabilityIdentityStatus()).toBe('unverified')
  })

  it('derives passive-only labels without borrowing active probe evidence', () => {
    expect(passiveCapabilityStatus(3)).toBe('observed')
    expect(passiveIdentityStatus(3, 0, ['gpt-5'])).toBe('self_report_consistent')
    expect(passiveConfidence(3, 0, ['gpt-5'])).toBe('medium')

    expect(passiveIdentityStatus(3, 1, ['other-model'])).toBe('mismatch_suspected')
    expect(passiveConfidence(3, 1, ['other-model'])).toBe('high')

    expect(passiveCapabilityStatus(0)).toBe('unverified')
    expect(passiveIdentityStatus(0, 0)).toBe('unverified')
    expect(passiveConfidence(0, 0)).toBe('low')
  })
})
