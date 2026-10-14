export type AccountTierPool = 'premium' | 'standard'

export function readAccountTierRouting(extra?: Record<string, unknown> | null) {
  const configuredPool = typeof extra?.pool === 'string' ? extra.pool.trim().toLowerCase() : ''
  const pool: AccountTierPool = configuredPool === 'premium' ? 'premium' : 'standard'
  const enabled = extra?.tiered_routing_enabled === undefined
    ? configuredPool === 'premium' || configuredPool === 'standard'
    : extra.tiered_routing_enabled === true
  return { enabled, pool }
}

export function withAccountTierRouting(extra: Record<string, unknown> | null | undefined, enabled: boolean, pool: AccountTierPool) {
  return { ...extra, tiered_routing_enabled: enabled, pool }
}
