import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProviderHubView from '../ProviderHubView.vue'
import ProviderOperationsView from '../ProviderOperationsView.vue'
import { getProviderHub, getProviderKeys, getProviderOperations, providerHubWrite } from '@/api/admin/providerHub'
import { providerHubFixture, providerOperationsFixture } from './providerHubFixtures'
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { props: ['show'], template: '<section v-if="show"><slot /><slot name="footer" /></section>' } }))
vi.mock('vue-router', () => ({ RouterLink: { template: '<a><slot /></a>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({ Line: { template: '<canvas />' } }))
vi.mock('@/api/admin/providerHub', () => ({ getProviderHub: vi.fn(), providerHubWrite: vi.fn(), getProviderPurchases: vi.fn(), getProviderKeys: vi.fn(), getProviderGroups: vi.fn(), getProviderOperations: vi.fn() }))
beforeEach(() => { vi.resetAllMocks(); vi.mocked(getProviderHub).mockResolvedValue(providerHubFixture()); vi.mocked(providerHubWrite).mockResolvedValue({ committed: true }); vi.mocked(getProviderOperations).mockResolvedValue(providerOperationsFixture()); vi.stubGlobal('crypto', { randomUUID: () => '00000000-0000-0000-0000-000000000001' }) })

describe('supplier workflow', () => {
  it('clears key selections when switching wallets', async () => {
    vi.mocked(getProviderKeys).mockResolvedValue([{ id: 'remote-1', name: 'Supplier key', group: '1', available: true, masked_key: 'sk-****' }])
    const w = mount(ProviderHubView); await flushPromises()
    await w.findAll('nav button').find(b => b.text() === 'providerHub.keys')!.trigger('click')
    await w.findAll('section button').find(b => b.text() === 'providerHub.refresh')!.trigger('click'); await flushPromises()
    const checkbox = w.findAll('tbody input[type="checkbox"]')[0]!; await checkbox.setValue(true)
    expect(w.findAll('button').find(b => b.text() === 'providerHub.import')!.attributes('disabled')).toBeUndefined()
    await w.findAll('button').find(b => b.text() === 'Wallet 2')!.trigger('click'); await flushPromises()
    expect(w.findAll('button').find(b => b.text() === 'providerHub.import')!.attributes('disabled')).toBeDefined(); expect(w.text()).not.toContain('Supplier key')
    w.unmount()
  })
  it('keeps explicit zero cost and submits a new version with wallet revision', async () => {
    const w = mount(ProviderHubView); await flushPromises()
    await w.findAll('nav button').find(b => b.text() === 'providerHub.rules')!.trigger('click')
    await w.findAll('button').find(b => b.text() === 'providerHub.addRule')!.trigger('click')
    await w.get('#provider-hub-form select').setValue('request')
    await w.get('#provider-hub-form').trigger('submit'); await flushPromises()
    expect(providerHubWrite).toHaveBeenCalledWith('/wallets/1/rules', expect.objectContaining({ revision: 3, unit_price: 0, method: 'request', currency: 'USD' }), 'post')
    w.unmount()
  })
  it.each([['', null], ['0', 0], ['3.5', 3.5]])('preserves nullable 12h procurement price %s', async (input, expected) => {
    const w = mount(ProviderHubView); await flushPromises()
    await w.findAll('nav button').find(b => b.text() === 'providerHub.rules')!.trigger('click')
    await w.findAll('button').find(b => b.text() === 'providerHub.addRule')!.trigger('click')
    await w.get('#provider-hub-form select').setValue('tokens')
    await w.get('input[placeholder="providerHub.cacheWrite12hFallback"]').setValue(input)
    await w.get('#provider-hub-form').trigger('submit'); await flushPromises()
    expect(providerHubWrite).toHaveBeenCalledWith('/wallets/1/rules', expect.objectContaining({ cache_write_12h_price: expected, cache_write_price: 0 }), 'post')
    w.unmount()
  })
  it('does not retain a typed secret after closing the editor', async () => {
    const w = mount(ProviderHubView); await flushPromises()
    await w.findAll('button[aria-label="providerHub.authorization"]')[0]!.trigger('click')
    await w.get('input[type="password"]').setValue('synthetic-editor-only')
    await w.findAll('button').find(b => b.text() === 'providerHub.cancel')!.trigger('click')
    await w.findAll('button[aria-label="providerHub.authorization"]')[0]!.trigger('click')
    expect((w.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(providerHubWrite).not.toHaveBeenCalled(); w.unmount()
  })
})
describe('operations cost coverage', () => {
  it('shows unavailable cost and profit as unknown while keeping cash separate', async () => {
    const data = providerOperationsFixture(); data.summary.usd_priced = 0; data.summary.comparable_count = 0; data.summary.unknown = 120
    vi.mocked(getProviderOperations).mockResolvedValue(data)
    const w = mount(ProviderOperationsView); await flushPromises()
    expect(w.text()).toContain('providerHub.activation'); expect(w.text()).toContain('providerHub.unknown'); expect(w.text()).toContain('providerHub.cashOverview')
    const cards = w.findAll('.rounded-lg.border'); expect(cards[2]!.text()).toContain('providerHub.unknown'); expect(cards[3]!.text()).toContain('providerHub.unknown')
    w.unmount()
  })
  it('does not display a failed business query as zero revenue', async () => {
    vi.mocked(getProviderOperations).mockRejectedValue(new Error('unavailable'))
    const w = mount(ProviderOperationsView); await flushPromises(); expect(w.get('[role="alert"]').text()).toContain('providerHub.failed'); expect(w.find('table').exists()).toBe(false); w.unmount()
  })
})
