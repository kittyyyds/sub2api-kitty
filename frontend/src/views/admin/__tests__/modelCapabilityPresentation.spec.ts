import { describe, expect, it } from 'vitest'
import {
  auditCatalogLabel,
  auditEvidenceLevel,
  auditIdentityDecision,
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

  it('keeps a single mismatch in retest state and upgrades repeated evidence', () => {
    expect(auditIdentityDecision('mismatch', 'medium', true, true)).toContain('需要复测')
    expect(auditEvidenceLevel('mismatch', 'medium')).toBe('中等异常证据')
    expect(auditIdentityDecision('mismatch_suspected', 'high', true, true)).toContain('高度疑似')
    expect(auditEvidenceLevel('mismatch_suspected', 'high')).toBe('强异常证据')
  })

  it('presents model-directory evidence as weak evidence only', () => {
    expect(auditCatalogLabel('checked', true)).toContain('已声明')
    expect(auditCatalogLabel('checked', false)).toContain('未声明')
    expect(auditIdentityDecision('unverified', 'low', true, false)).toContain('仅模型目录声明')
  })
})
