import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'

const { api, profile, quotas, stats } = vi.hoisted(() => {
  const stats = { total_api_keys: 0, today_requests: 12, today_actual_cost: 0.125 }
  return { stats, profile: vi.fn(), quotas: vi.fn(), api: { getDashboardStats: vi.fn(), getDashboardTrend: vi.fn(), getDashboardModels: vi.fn(), getByDateRange: vi.fn() } }
})
vi.mock('@/api/usage', () => ({ usageAPI: api }))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: quotas }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { username: '真实用户', balance: 25 }, isSimpleMode: false, refreshUser: profile }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: { payment_enabled: true } }) }))
vi.mock('@/composables/useRelayBrand', () => ({ useRelayBrand: () => ({ siteName: '澄川 AI' }) }))
vi.mock('@/composables/useRelayEndpoint', () => ({ useRelayEndpoint: () => ({ endpoint: 'https://relay.example/v1' }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn().mockResolvedValue(true) }) }))

function mountDashboard() {
  return mount(DashboardView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    RouterLink: RouterLinkStub, Icon: true, LoadingSpinner: true, RelayQuickStart: true, UserDashboardQuickActions: true, LowBalanceNotice: true,
    UserDashboardStats: { props: ['stats', 'balance'], template: '<div data-testid="stats">{{ stats.today_requests }} / {{ stats.today_actual_cost }} / {{ balance }}</div>' },
    UserDashboardCharts: { props: ['trend', 'models'], emits: ['refresh', 'dateRangeChange'], template: '<div data-testid="charts">{{ trend }} {{ models }}</div>' },
    UserDashboardRecentUsage: { props: ['data'], template: '<div data-testid="recent">{{ data }}</div>' },
  } } })
}

describe('native dashboard server data', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getDashboardStats.mockResolvedValue(stats)
    api.getDashboardTrend.mockResolvedValue({ trend: [] })
    api.getDashboardModels.mockResolvedValue({ models: [] })
    api.getByDateRange.mockResolvedValue({ items: [] })
    profile.mockResolvedValue(undefined)
    quotas.mockResolvedValue({ platform_quotas: [] })
  })

  afterEach(() => vi.restoreAllMocks())

  it('renders the real server statistics and native quick actions without synthetic counts', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="stats"]').text()).toBe('12 / 0.125 / 25')
    expect(wrapper.find('user-dashboard-quick-actions-stub').exists()).toBe(true)
    expect(wrapper.find('[data-testid="charts"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="recent"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not present invented account statistics after a failed statistics request', async () => {
    const failure = new Error('network unavailable')
    const reportError = vi.spyOn(console, 'error').mockImplementation(() => {})
    api.getDashboardStats.mockRejectedValueOnce(failure)
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="stats"]').exists()).toBe(false)
    expect(wrapper.find('loading-spinner-stub').exists()).toBe(false)
    expect(reportError).toHaveBeenCalledWith('Failed to load dashboard stats:', failure)
    wrapper.unmount()
  })

  it('does not combine fresh statistics with an account profile that failed to refresh', async () => {
    const failure = new Error('unavailable')
    const reportError = vi.spyOn(console, 'error').mockImplementation(() => {})
    profile.mockRejectedValue(failure)
    const wrapper = mountDashboard()
    await flushPromises()
    expect(api.getDashboardStats).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="stats"]').exists()).toBe(false)
    expect(wrapper.find('loading-spinner-stub').exists()).toBe(false)
    expect(reportError).toHaveBeenCalledWith('Failed to load dashboard stats:', failure)
    wrapper.unmount()
  })
})
