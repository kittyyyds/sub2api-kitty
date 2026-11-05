import { apiClient } from '../client'

export interface Supplier { id: number; name: string; site_type: string; base_url: string; notes: string; active: boolean; revision: number }
export interface SupplierWallet { id: number; supplier_id: number; name: string; identity: string; currency: string; credit_currency: string; low_balance: number | null; sync_interval_minutes: number; auth_mode: string; username: string; remote_user_id: string; has_secret: boolean; balance: number | null; balance_unit: string; balance_source: string; observed_at: string | null; attempted_at: string | null; sync_error: string; fresh: boolean | null; active: boolean; revision: number; credit_cost: number | null; purchases: number }
export interface SupplierBinding { id: number; account_id: number; wallet_id: number; account_name: string | null; started_at: string }
export interface SupplierPurchase { id: number; wallet_id: number; kind: string; cash: number; cash_currency: string; fx_rate: number; credits: number; bonus: number; reference: string; note: string; occurred_at: string; voided_at: string | null; revision: number }
export interface SupplierRule { id: number; wallet_id: number; model: string; method: string; multiplier: number; input_price: number; output_price: number; cache_read_price: number; cache_write_price: number; unit_price: number; currency: string; fx_to_usd: number | null; note: string; started_at: string }
export interface SupplierAccount { id: number; name: string; type: string; platform: string; status: string }
export interface ProviderHubState { queried_at: string; started_at: string; suppliers: Supplier[]; wallets: SupplierWallet[]; bindings: SupplierBinding[]; purchases: SupplierPurchase[]; purchase_total: number; rules: SupplierRule[]; accounts: SupplierAccount[]; account_total: number }
export interface ProviderKey { id: string; name: string; group: string; available: boolean; masked_key: string }
export interface ProviderGroup { id: string; name: string }
export interface OperationsMetrics { requests: number; request_valuation: number; valuation_unknown: number; priced: number; reconciled: number; estimated: number; unknown: number; usd_priced: number; usd_cost: number; comparable_count: number; comparable_valuation: number; comparable_profit: number }
export interface ProcurementReceipt { api_key_id: number; request_key: string; source_log_id: number; account_id: number; group_id: number | null; model: string; upstream_model: string; upstream_request_id: string | null; supplier_id: number | null; supplier_name: string | null; wallet_id: number | null; wallet_name: string | null; rule_id: number | null; rule_snapshot: Record<string, unknown>; credit_cost: number | null; measurements: Record<string, number | null>; estimated_cost: number | null; cost_currency: string | null; request_valuation: number | null; effective_cost: number | null; effective_currency: string | null; usd_cost: number | null; evidence: string; invoice_reference: string | null; observed_at: string }
export interface OperationsSnapshot {
 started_at: string; queried_at: string; summary: OperationsMetrics; trend: { day: string; metrics: OperationsMetrics }[]
 breakdown: { key: string; name: string; requests: number; metrics: OperationsMetrics }[]; breakdown_limit: number
 receipts: ProcurementReceipt[]; cost_currencies: { currency: string; records: number; amount: number }[]
 cash: { currency: string; order_type: string; paid_orders: number; received: number; refunded_orders: number; refunded_credits: number }[]
 balances: { users: number; positive: number; negative: number }
 monitoring: { source: string; records: number; recorded_cost: number; unknown: number }[]
 purchases: { currency: string; paid: number | null; refunded: number | null; records: number }[]
}
export async function getProviderHub(signal?: AbortSignal) { return (await apiClient.get<ProviderHubState>('/admin/provider-hub', { signal })).data }
export interface ProviderPurchasePage { items: SupplierPurchase[]; total: number; page: number; page_size: number }
export async function getProviderPurchases(id: number, page = 1) { return (await apiClient.get<ProviderPurchasePage>(`/admin/provider-hub/wallets/${id}/purchases`, { params: { page, page_size: 20 } })).data }
export async function providerHubWrite(path: string, body: Record<string, unknown>, method: 'post' | 'put' = 'post') { return (await apiClient[method](`/admin/provider-hub${path}`, body, { timeout: 120000 })).data }
export async function getProviderKeys(id: number) { return (await apiClient.get<ProviderKey[]>(`/admin/provider-hub/wallets/${id}/keys`, { timeout: 120000 })).data }
export async function getProviderGroups(id: number) { return (await apiClient.get<ProviderGroup[]>(`/admin/provider-hub/wallets/${id}/groups`)).data }
export async function getProviderOperations(params: Record<string, string | number>, signal?: AbortSignal) { return (await apiClient.get<OperationsSnapshot>('/admin/operations', { params, signal })).data }
