/** Normalize persisted per-protocol values to the public summary vocabulary. */
export function normalizeCapabilityIdentityStatus(value?: string): string {
  if (value === 'mismatch') return 'mismatch_suspected'
  return value || 'unverified'
}
