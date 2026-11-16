import type { ProviderHubState, OperationsSnapshot, OperationsMetrics, ProcurementReceipt } from '@/api/admin/providerHub'
export function providerHubFixture(): ProviderHubState {
  return {
    queried_at: new Date().toISOString(), started_at: '2026-11-06T00:00:00Z',
    suppliers: [{ id: 1, name: '示例供应商 A', site_type: 'sub2api', base_url: 'https://supplier.example', notes: '测试采购资料', active: true, revision: 1 }, { id: 2, name: '示例供应商 B', site_type: 'newapi', base_url: 'https://provider.example', notes: '', active: true, revision: 1 }],
    wallets: [1, 2].map(id => ({ id, supplier_id: 1, name: `Wallet ${id}`, identity: `upstream-user-${id}`, currency: 'USD', credit_currency: 'USD', low_balance: 10, sync_interval_minutes: 15, auth_mode: 'token', username: '', remote_user_id: '', has_secret: true, balance: id === 1 ? 8.125 : 150, balance_unit: 'USD', balance_source: 'sub2api_profile', observed_at: new Date().toISOString(), attempted_at: new Date().toISOString(), sync_error: '', fresh: true, active: true, revision: 3, credit_cost: 0.1, purchases: 1 })),
    bindings: [{ id: 1, account_id: 11, wallet_id: 1, account_name: '示例采购账号', started_at: '2026-11-06T01:00:00Z' }],
    purchases: [{ id: 1, wallet_id: 1, kind: 'topup', cash: 100, cash_currency: 'USD', fx_rate: 1, credits: 900, bonus: 100, reference: 'ORDER-EXAMPLE-20261106', note: '', occurred_at: '2026-11-06T00:00:00Z', voided_at: null, revision: 1 }], purchase_total: 1,
    rules: [{ id: 1, wallet_id: 1, model: '*', method: 'standard_multiplier', multiplier: 0.1, input_price: 0, output_price: 0, cache_read_price: 0, cache_write_price: 0, unit_price: 0, currency: 'USD', fx_to_usd: 1, note: '', started_at: '2026-11-06T01:00:00Z' }],
    accounts: [{ id: 11, name: '示例采购账号', type: 'apikey', platform: 'openai', status: 'active' }, { id: 12, name: '另一个尚未关联的账号', type: 'apikey', platform: 'anthropic', status: 'active' }], account_total: 2,
  }
}
export function operationsMetrics(overrides: Partial<OperationsMetrics> = {}): OperationsMetrics { return { requests: 120, request_valuation: 24.5, valuation_unknown: 0, priced: 100, reconciled: 60, estimated: 40, unknown: 20, usd_priced: 100, usd_cost: 12.5, comparable_count: 100, comparable_valuation: 20.5, comparable_profit: 8, ...overrides } }
export function procurementReceipt(): ProcurementReceipt { return { api_key_id: 1, request_key: 'request:fixture', source_log_id: 1, account_id: 11, group_id: 3, model: 'gpt-6.1-sol', upstream_model: 'gpt-6.1-sol', upstream_request_id: 'req-example-only', supplier_id: 1, supplier_name: '示例供应商 A', wallet_id: 1, wallet_name: 'Wallet 1', rule_id: 1, rule_snapshot: { method: 'standard_multiplier', multiplier: 0.1, currency: 'USD', fx_to_usd: 1 }, credit_cost: null, measurements: { input: 1000, output: 200, standard: 0.5 }, estimated_cost: 0.05, cost_currency: 'USD', request_valuation: 0.1, effective_cost: 0.05, effective_currency: 'USD', usd_cost: 0.05, evidence: 'estimated', invoice_reference: null, observed_at: new Date().toISOString() } }
export function providerOperationsFixture(): OperationsSnapshot { return {
  started_at: '2026-11-06T00:00:00Z', queried_at: new Date().toISOString(), summary: operationsMetrics(),
  trend: Array.from({ length: 7 }, (_, i) => ({ day: `2026-11-${String(i + 1).padStart(2, '0')}`, metrics: operationsMetrics({ request_valuation: 5 + i * 2, usd_cost: 3 + i, comparable_profit: 2 + i }) })),
  breakdown: [{ key: '1', name: '示例供应商 A', requests: 120, metrics: operationsMetrics() }], breakdown_limit: 200,
  receipts: [procurementReceipt(), { ...procurementReceipt(), request_key: 'request:unknown', source_log_id: 2, model: 'grok-imagine-video-1.5', supplier_name: null, wallet_name: null, effective_cost: null, usd_cost: null, evidence: 'unknown' }],
  cost_currencies: [{ currency: 'USD', records: 100, amount: 12.5 }], cash: [{ currency: 'CNY', order_type: 'balance', paid_orders: 5, received: 300, refunded_orders: 1, refunded_credits: 10 }],
  balances: { users: 20, positive: 1000, negative: 0 }, monitoring: [{ source: 'pelican_group_result', records: 5, recorded_cost: 0.08, unknown: 0 }], purchases: [{ currency: 'USD', paid: 100, refunded: 0, records: 1 }],
} }
