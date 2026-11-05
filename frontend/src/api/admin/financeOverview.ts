import { apiClient } from '../client'
export interface FinanceOverview {
  from: string; until: string; queried_at: string
  usage: { records: number; valuation: number; invalid_records: number; subscription_valuation: number }
  cash: Array<{ currency: string; order_type: string; paid_orders: number; received: number; invalid_receipts: number; refunded_orders: number; refunded_credits: number }>
  balances: { users: number; positive: number; negative: number; invalid: number }
  credits: { affiliate_transfers: number; recharge_bonus_granted: number; recharge_bonus_revoked: number }
  monitoring: Array<{ source: string; records: number; priced: number; recorded_cost: number; unknown: number; earliest_retained: string | null }>
}
export interface PermanentUsage {
  queried_at: string
  state: { started_at: string; baseline_highwater: number; baseline_cursor: number; baseline_complete: boolean; baseline_completed_at: string | null }
  totals: { requests: number; input_tokens: number; output_tokens: number; cache_creation_tokens: number; cache_read_tokens: number; recorded_valuation: number; standard_valuation: number; subscription_valuation: number; invalid_records: number; oldest_observation: string | null; newest_observation: string | null }
}
export async function readFinanceOverview(from: string, until: string, signal?: AbortSignal) {
  return (await apiClient.get<FinanceOverview>('/admin/finance-overview', { params: { from, until }, signal })).data
}
export async function readPermanentUsage(signal?: AbortSignal) { return (await apiClient.get<PermanentUsage>('/admin/permanent-usage', { signal })).data }
export async function backfillPermanentUsage() { return (await apiClient.post<{ processed: number }>('/admin/permanent-usage/backfill')).data }
