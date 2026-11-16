import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ siteName: 'Kitty奇妙屋', backendModeEnabled: false, cachedPublicSettings: null }),
}))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }),
}))

describe('AI workbench routes', () => {
  it('registers Kitty Studio but leaves online chat retired', async () => {
    const { default: router } = await import('@/router')
    const studio = router.getRoutes().find((route) => route.name === 'AppCenter')
    const chat = router.getRoutes().find((route) => route.name === 'KittyChat')

    expect(studio?.path).toBe('/apps')
    expect(studio?.meta.requiresAuth).toBe(true)
    expect(studio?.meta.titleKey).toBe('nav.creativeCanvas')
    expect(chat).toBeUndefined()
  })

  it('keeps both legacy image-generator URLs as Studio redirects', async () => {
    const { default: router } = await import('@/router')
    const legacy = router.getRoutes().find((route) => route.path === '/image-generator')
    const customLegacy = router.getRoutes().find((route) => route.path === '/custom/image-generator')

    expect(legacy?.redirect).toBe('/apps')
    expect(customLegacy?.redirect).toBe('/apps')
  })

  it('sends retired operations URLs to NotFound while retaining supplier and operations pages', async () => {
    const { default: router } = await import('@/router')
    for (const path of ['/admin/channel-operations', '/admin/profit-operations', '/admin/finance-overview']) {
      expect(router.resolve(path).matched.at(-1)?.name).toBe('NotFound')
    }
    expect(router.resolve('/admin/supplier-balances').name).toBe('NotFound')
    expect(router.resolve('/admin/operations').name).toBe('AdminProviderOperations')
    expect(router.resolve('/admin/accounts').name).toBe('AdminAccounts')
  })
})
