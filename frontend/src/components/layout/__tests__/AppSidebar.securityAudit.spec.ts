import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppSidebar from '../AppSidebar.vue'

const appStore = reactive({
  backendModeEnabled: false,
  sidebarCollapsed: false,
  mobileOpen: false,
  siteName: 'KittyAPI',
  siteLogo: '',
  siteVersion: 'test',
  publicSettingsLoaded: true,
  cachedPublicSettings: { risk_control_enabled: false, custom_menu_items: [] },
  sidebarScrollTop: 0,
  toggleSidebar: vi.fn(),
  setMobileOpen: vi.fn(),
})
const authStore = reactive({ isAdmin: true, isSimpleMode: false })
const adminSettingsStore = {
  opsMonitoringEnabled: false,
  paymentEnabled: false,
  customMenuItems: [],
  fetch: vi.fn(),
}

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
  useAdminSettingsStore: () => adminSettingsStore,
  useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() }),
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const auditPaths = ['/admin/risk-control', '/admin/prompt-audit', '/admin/security-policy']

async function mountSidebar(path = '/admin/dashboard') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push(path)
  await router.isReady()
  return mount(AppSidebar, {
    global: { plugins: [router], stubs: { VersionBadge: true } },
  })
}

describe('Security Audit admin navigation', () => {
  beforeEach(() => {
    authStore.isAdmin = true
    appStore.cachedPublicSettings.risk_control_enabled = false
    vi.clearAllMocks()
  })

  it.each([false, true])('shows the three configuration links when risk control is %s', async (enabled) => {
    appStore.cachedPublicSettings.risk_control_enabled = enabled
    const wrapper = await mountSidebar()
    const group = wrapper.findAll('button').find((button) => button.text().includes('nav.securityAudit'))
    expect(group).toBeDefined()
    await group!.trigger('click')

    for (const path of auditPaths) {
      expect(wrapper.find(`a[href="${path}"]`).exists()).toBe(true)
    }
    expect(appStore.cachedPublicSettings.risk_control_enabled).toBe(enabled)
    wrapper.unmount()
  })

  it('keeps the audit menus hidden from regular users', async () => {
    authStore.isAdmin = false
    const wrapper = await mountSidebar('/dashboard')

    expect(wrapper.text()).not.toContain('nav.securityAudit')
    for (const path of auditPaths) {
      expect(wrapper.find(`a[href="${path}"]`).exists()).toBe(false)
    }
    wrapper.unmount()
  })
})
