import { apiClient } from '../client'
export type BalanceThresholds = Record<string, { wallet: number | null; remaining: number | null }>
export interface SupplierBalance {
  account_id: number; name: string; platform: string; host: string; source: string; status: string
  unit: string; wallet: number | null; remaining: number | null; unlimited: boolean
  received_at: string | null; fresh_until: string | null; last_attempt_at: string | null; alerts: string[]
}
export interface SupplierBalances {
  rows: SupplierBalance[]; total: number; page: number; page_size: number; queried_at: string; thresholds: BalanceThresholds
}
export async function readSupplierBalances(page: number, signal?: AbortSignal) {
  return (await apiClient.get<SupplierBalances>('/admin/supplier-balances', { params: { page, page_size: 20 }, signal })).data
}
export async function saveSupplierThresholds(value: BalanceThresholds) {
  return (await apiClient.put<BalanceThresholds>('/admin/supplier-balances/thresholds', value)).data
}
