/** Normalize persisted per-protocol values to the public summary vocabulary. */
export function normalizeCapabilityIdentityStatus(value?: string): string {
  if (value === 'mismatch') return 'mismatch_suspected'
  return value || 'unverified'
}

export function passiveCapabilityStatus(passiveRequests: number): string {
  return passiveRequests > 0 ? 'observed' : 'unverified'
}

export function passiveIdentityStatus(
  passiveRequests: number,
  passiveMismatches: number,
  responseModels: string[] = []
): string {
  if (passiveMismatches > 0) return 'mismatch_suspected'
  if (passiveRequests > 0 && responseModels.length > 0) return 'self_report_consistent'
  return 'unverified'
}

export function passiveConfidence(
  passiveRequests: number,
  passiveMismatches: number,
  responseModels: string[] = []
): string {
  if (passiveMismatches > 0) return 'high'
  if (passiveRequests > 0 && responseModels.length > 0) return 'medium'
  return 'low'
}
