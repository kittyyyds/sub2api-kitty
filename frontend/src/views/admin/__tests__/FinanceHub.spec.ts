import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import SupplierBalancesView from '../SupplierBalancesView.vue'
import FinanceOverviewView from '../FinanceOverviewView.vue'
import { readSupplierBalances, saveSupplierThresholds, type SupplierBalances } from '@/api/admin/supplierBalances'
import { readFinanceOverview, readPermanentUsage, type FinanceOverview, type PermanentUsage } from '@/api/admin/financeOverview'
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/admin/operations/SmartOpsNav.vue', () => ({ default: { template: '<nav />' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/supplierBalances', () => ({ readSupplierBalances: vi.fn(), saveSupplierThresholds: vi.fn() }))
vi.mock('@/api/admin/financeOverview', () => ({ readFinanceOverview: vi.fn(), readPermanentUsage: vi.fn(), backfillPermanentUsage: vi.fn() }))
const supplier = (): SupplierBalances => ({ rows: [{ account_id: 1, name: 'supplier', platform: 'openai', host: 'vendor.example', source: 'sub2api_usage', status: 'stale', unit: 'USD', wallet: 4, remaining: null, unlimited: false, received_at: null, fresh_until: null, last_attempt_at: null, alerts: [] }], total: 1, page: 1, page_size: 20, queried_at: '', thresholds: { CNY: { wallet: 12, remaining: null } } })
beforeEach(() => { vi.resetAllMocks(); vi.mocked(readSupplierBalances).mockResolvedValue(supplier()) })
describe('supplier balance separation', () => {
  it('keeps stale wallet observations, unknown key allowance and preserves other currency thresholds', async () => {
    vi.mocked(saveSupplierThresholds).mockResolvedValue({})
    const w = mount(SupplierBalancesView); await flushPromises()
    expect(w.text()).toContain('financeHub.states.stale'); expect(w.text()).toContain('4 USD'); expect(w.text()).toContain('financeHub.unknown')
    await w.findAll('input[type="number"]')[0]!.setValue(5)
    await w.get('form').trigger('submit'); await flushPromises()
    expect(saveSupplierThresholds).toHaveBeenCalledWith({ CNY: { wallet: 12, remaining: null }, USD: { wallet: 5, remaining: null } })
    w.unmount()
  })
  it('does not render a failed read as zero balance or expose stale rows after failure', async () => {
    const w = mount(SupplierBalancesView); await flushPromises()
    vi.mocked(readSupplierBalances).mockRejectedValueOnce(new Error('failure'))
    await w.get('header button').trigger('click'); await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('financeHub.failed'); expect(w.find('table').exists()).toBe(false)
    w.unmount()
  })
})
describe('finance coverage and independent failures', () => {
  it('keeps permanent totals available when period finance fails and labels incomplete history', async () => {
    vi.mocked(readFinanceOverview).mockRejectedValue(new Error('failed'))
    vi.mocked(readPermanentUsage).mockResolvedValue({ state: { started_at: '2026-11-06T00:00:00Z', baseline_complete: false }, totals: { requests: 27, input_tokens: 10, output_tokens: 20, cache_creation_tokens: 0, cache_read_tokens: 0, recorded_valuation: 2, invalid_records: 0, oldest_observation: null } } as PermanentUsage)
    const w = mount(FinanceOverviewView, { global: { stubs: { RouterLink: true } } }); await flushPromises()
    expect(w.text()).toContain('financeHub.failed'); expect(w.text()).toContain('financeHub.baselinePending'); expect(w.text()).toContain('27'); expect(w.text()).toContain('financeHub.coverageNote')
    w.unmount()
  })
  it('separates currencies and refund credits and never sums them into fake net cash', async () => {
    vi.mocked(readFinanceOverview).mockResolvedValue({ queried_at: '', usage: { valuation: 2 }, cash: [{ currency: 'CNY', order_type: 'balance', paid_orders: 1, received: 70, refunded_orders: 1, refunded_credits: 100 }, { currency: 'USD', order_type: 'balance', paid_orders: 1, received: 10, refunded_orders: 0, refunded_credits: 0 }], balances: { positive: 99, negative: -1 }, credits: { affiliate_transfers: 5, recharge_bonus_granted: 1, recharge_bonus_revoked: 0 }, monitoring: [] } as unknown as FinanceOverview)
    vi.mocked(readPermanentUsage).mockRejectedValue(new Error('unavailable'))
    const w = mount(FinanceOverviewView, { global: { stubs: { RouterLink: true } } }); await flushPromises()
    expect(w.findAll('tbody tr')).toHaveLength(2); expect(w.text()).toContain('financeHub.cashNote'); expect(w.text()).toContain('financeHub.walletNote'); expect(w.text()).toContain('financeHub.failed')
    expect(w.text()).not.toMatch(/NaN|Infinity/); w.unmount()
  })
})
