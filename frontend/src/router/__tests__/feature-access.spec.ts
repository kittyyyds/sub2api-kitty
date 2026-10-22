import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  routes: [] as RouteRecordRaw[],
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    payment_enabled?: boolean
    risk_control_enabled?: boolean
    channel_monitor_enabled?: boolean
    subscription_enabled?: boolean
    support_ticket_enabled?: boolean
    custom_menu_items?: []
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn((options: { routes: RouteRecordRaw[] }) => {
    routerHarness.routes = options.routes
    return {
      beforeEach: vi.fn((guard: NavigationGuard) => {
        routerHarness.guard = guard
      }),
      afterEach: vi.fn(),
      onError: vi.fn(),
    }
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  }

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {},
      query: {},
      hash: '',
      meta: { requiresAuth: true, ...meta },
    },
    {},
    next
  )
  return { navigation, next }
}

function runRegisteredGuard(path: string) {
  const route = routerHarness.routes.find((entry) => (
    entry.path === path || entry.alias === path ||
    (Array.isArray(entry.alias) && entry.alias.includes(path))
  ))
  if (!route) throw new Error(`Route is not registered: ${path}`)
  return runGuard(route.meta ?? {}, path)
}

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/router')
  })

  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockReset()
  })

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ payment_enabled: boolean }>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    })

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ payment_enabled: true })
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, '/purchase'],
    ['channel monitor', { requiresChannelMonitor: true }, '/monitor'],
    ['subscription', { requiresSubscription: true }, '/subscriptions'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('redirects away from the channel monitor when settings explicitly disable it', async () => {
    appStore.cachedPublicSettings = { channel_monitor_enabled: false }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresChannelMonitor: true }, '/monitor')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it.each([
    ['payment', { requiresPayment: true }, { payment_enabled: false }, '/dashboard'],
    ['subscription', { requiresSubscription: true }, { subscription_enabled: false }, '/dashboard'],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target) => {
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  })

  describe.each([
    '/admin/risk-control',
    '/admin/prompt-audit',
    '/admin/security-policy',
  ])('Security Audit configuration route %s', (path) => {
    it.each([false, true])('allows administrators when risk control is %s', async (enabled) => {
      authStore.isAdmin = true
      appStore.publicSettingsLoaded = true
      appStore.cachedPublicSettings = { risk_control_enabled: enabled }

      const { navigation, next } = runRegisteredGuard(path)
      await navigation

      expect(next).toHaveBeenCalledOnce()
      expect(next).toHaveBeenCalledWith()
      expect(appStore.cachedPublicSettings.risk_control_enabled).toBe(enabled)
    })

    it('rejects authenticated users without administrator access', async () => {
      appStore.publicSettingsLoaded = true
      appStore.cachedPublicSettings = { risk_control_enabled: false }

      const { navigation, next } = runRegisteredGuard(path)
      await navigation

      expect(next).toHaveBeenCalledOnce()
      expect(next).toHaveBeenCalledWith('/dashboard')
    })

    it('requires login before configuration can be viewed', async () => {
      authStore.isAuthenticated = false

      const { navigation, next } = runRegisteredGuard(path)
      await navigation

      expect(next).toHaveBeenCalledOnce()
      expect(next).toHaveBeenCalledWith({ path: '/login', query: { redirect: path } })
    })
  })
})

describe('subscription route guard (opt-out flag)', () => {
  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.publicSettingsLoaded = true
    appStore.fetchPublicSettings.mockReset()
  })

  it.each([
    ['missing key', {}],
    ['explicit true', { subscription_enabled: true }],
  ])('lets /subscriptions through when the flag is %s', async (_name, settings) => {
    appStore.cachedPublicSettings = settings

    const { navigation, next } = runGuard({ requiresSubscription: true }, '/subscriptions')
    await navigation

    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('sends admins to the admin dashboard when subscriptions are disabled', async () => {
    authStore.isAdmin = true
    appStore.cachedPublicSettings = { subscription_enabled: false }

    const { navigation, next } = runGuard({ requiresSubscription: true }, '/subscriptions')
    await navigation

    expect(next).toHaveBeenCalledWith('/admin/dashboard')
  })
})

describe('support ticket route guard (opt-in flag)', () => {
  beforeEach(() => {
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.publicSettingsLoaded = true
    appStore.fetchPublicSettings.mockReset()
  })

  it.each([
    ['missing', {}, false, '/dashboard'],
    ['off', { support_ticket_enabled: false }, false, '/dashboard'],
    ['off for an admin', { support_ticket_enabled: false }, true, '/admin/settings'],
  ])('redirects when the switch is %s', async (_name, settings, admin, target) => {
    authStore.isAdmin = admin
    appStore.cachedPublicSettings = settings
    const { navigation, next } = runGuard({ requiresSupportTickets: true, requiresAdmin: admin }, '/support-tickets')
    await navigation
    expect(next).toHaveBeenCalledWith(target)
  })

  it('opens the pages when the switch is on', async () => {
    appStore.cachedPublicSettings = { support_ticket_enabled: true }
    const { navigation, next } = runGuard({ requiresSupportTickets: true }, '/support-tickets/7')
    await navigation
    expect(next).toHaveBeenCalledWith()
  })

  it('loads public settings before deciding', async () => {
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockImplementation(async () => {
      appStore.cachedPublicSettings = { support_ticket_enabled: true }
      appStore.publicSettingsLoaded = true
    })
    const { navigation, next } = runGuard({ requiresSupportTickets: true }, '/support-tickets')
    await navigation
    expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith()
  })
})
