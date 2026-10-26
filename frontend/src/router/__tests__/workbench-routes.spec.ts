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
  it('registers Kitty Studio and online chat as authenticated routes', async () => {
    const { default: router } = await import('@/router')
    const studio = router.getRoutes().find((route) => route.name === 'AppCenter')
    const chat = router.getRoutes().find((route) => route.name === 'KittyChat')

    expect(studio?.path).toBe('/apps')
    expect(studio?.meta.requiresAuth).toBe(true)
    expect(studio?.meta.titleKey).toBe('nav.creativeCanvas')
    expect(chat?.path).toBe('/chat')
    expect(chat?.meta.requiresAuth).toBe(true)
    expect(chat?.meta.titleKey).toBe('nav.onlineChat')
  })

  it('keeps both legacy image-generator URLs as Studio redirects', async () => {
    const { default: router } = await import('@/router')
    const legacy = router.getRoutes().find((route) => route.path === '/image-generator')
    const customLegacy = router.getRoutes().find((route) => route.path === '/custom/image-generator')

    expect(legacy?.redirect).toBe('/apps')
    expect(customLegacy?.redirect).toBe('/apps')
  })
})
