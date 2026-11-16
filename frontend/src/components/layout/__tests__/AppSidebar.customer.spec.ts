import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppSidebar from '../AppSidebar.vue'
import type { PublicSettings } from '@/types'

const appStore = reactive({
  backendModeEnabled: false,
  sidebarCollapsed: true,
  mobileOpen: false,
  siteName: 'KittyAPI',
  siteLogo: '',
  siteVersion: 'test',
  publicSettingsLoaded: true,
  cachedPublicSettings: { custom_menu_items: [] } as Partial<PublicSettings>,
  sidebarScrollTop: 0,
  toggleSidebar: vi.fn(),
  setMobileOpen: vi.fn()
})

const authStore = reactive({
  isAdmin: false,
  isSimpleMode: false
})

const adminSettingsStore = reactive({
  opsMonitoringEnabled: true,
  paymentEnabled: true,
  customMenuItems: [],
  fetch: vi.fn()
})

const onboardingStore = {
  isCurrentStep: vi.fn(() => false),
  nextStep: vi.fn()
}

const canUseBatchImage = ref(true)
const refreshBatchImageAccess = vi.fn()

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useSupportTicketStore: () => ({ userUnread: 0, adminPending: 0, refresh: vi.fn() }),
  useAuthStore: () => authStore,
  useAdminSettingsStore: () => adminSettingsStore,
  useOnboardingStore: () => onboardingStore
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage, refreshBatchImageAccess })
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    channelQualityPublic: 'channelQualityPublic',
    channelMonitor: 'channelMonitor',
    payment: 'payment',
    subscription: 'subscription',
    availableChannels: 'availableChannels',
    modelPlaza: 'modelPlaza',
    affiliate: 'affiliate',
    riskControl: 'riskControl',
    pluginManagement: 'pluginManagement',
    pelicanShowcase: 'pelicanShowcase'
  },
  makeSidebarFlag: (flag: string) => () => flag !== 'channelQualityPublic' || appStore.cachedPublicSettings.channel_quality_public_enabled !== false,
  resolveFeatureFlag: () => true
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'nav.appCenter': 'Kitty生图',
    'nav.onlineChat': '在线聊天',
    'nav.hotRecommendation': '火爆推荐',
    'nav.affiliateBadge': '邀请返现',
    'nav.limitedOffer': '充值有礼',
    'nav.canvasMediaBadge': '生图+视频'
  }
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => messages[key] ?? key, locale: ref('zh') })
  }
})

beforeEach(() => {
  appStore.backendModeEnabled = false
  appStore.sidebarCollapsed = true
  appStore.mobileOpen = false
  appStore.sidebarScrollTop = 0
  appStore.cachedPublicSettings = { custom_menu_items: [] }
  authStore.isAdmin = false
  authStore.isSimpleMode = false
  localStorage.setItem('theme', 'light')
  vi.clearAllMocks()
})

describe('AppSidebar customer grouped navigation', () => {
  it.each([false, true])('hides the user-facing quality link for admin=%s while keeping monitor navigation', async (admin) => {
    authStore.isAdmin = admin
    appStore.sidebarCollapsed = false
    appStore.cachedPublicSettings.channel_quality_public_enabled = false
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
    await router.push(admin ? '/admin/dashboard' : '/dashboard')
    await router.isReady()
    const wrapper = mount(AppSidebar, { global: { plugins: [router], stubs: { VersionBadge: true } } })
    expect(wrapper.find('a[href="/test-results"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/monitor"]').exists()).toBe(true)
    appStore.cachedPublicSettings.channel_quality_public_enabled = true
    await wrapper.vm.$nextTick()
    expect(wrapper.find('a[href="/test-results"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps the current route visible and active in collapsed and expanded states', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: '/:pathMatch(.*)*',
          component: { template: '<div />' }
        }
      ]
    })
    await router.push('/usage')
    await router.isReady()

    const wrapper = mount(AppSidebar, {
      global: {
        plugins: [router],
        stubs: { VersionBadge: true }
      }
    })

    const usageLink = wrapper.get('a[href="/usage"]')
    expect(usageLink.classes()).toContain('sidebar-link-active')
    expect(usageLink.classes()).toContain('sidebar-link-collapsed')

    appStore.sidebarCollapsed = false
    await wrapper.vm.$nextTick()

    expect(usageLink.classes()).toContain('sidebar-link-active')
    expect(usageLink.classes()).not.toContain('sidebar-link-collapsed')

    wrapper.unmount()
  })

  it('shows each native feature once when the backend still has matching custom menu IDs', async () => {
    appStore.sidebarCollapsed = false
    appStore.cachedPublicSettings = {
      custom_menu_items: [
        { id: 'reset-radar', label: 'Retired radar', icon_svg: '', url: 'https://legacy.example.test/radar', visibility: 'user', sort_order: 0 },
        {
          id: 'image-generator',
          label: 'Kitty媒体',
          icon_svg: '',
          url: 'https://legacy.example.test',
          visibility: 'user',
          sort_order: 1
        }
      ]
    }
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
    })
    await router.push('/dashboard')
    await router.isReady()

    const wrapper = mount(AppSidebar, {
      global: { plugins: [router], stubs: { VersionBadge: true } }
    })

    expect(wrapper.find('a[href="/custom/image-generator"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Kitty媒体')
    expect(wrapper.find('a[href="/custom/reset-radar"]').exists()).toBe(false)
    expect(wrapper.findAll('a[href="/model-plaza?embedded=1"]')).toHaveLength(1)
    expect(wrapper.find('a[href="/custom/billing-rules"]').exists()).toBe(false)
    expect(wrapper.findAll('a[href="/custom/docs"]')).toHaveLength(1)

    for (const retiredMenu of ['/chat']) {
      expect(wrapper.find(`a[href="${retiredMenu}"]`).exists()).toBe(false)
    }
    const paths = (section: string) => wrapper.findAll(`[data-customer-section="${section}"] > div > a`).map(link => link.attributes('href'))
    expect(paths('api')).toEqual(expect.arrayContaining(['/keys','/usage','/model-plaza?embedded=1','/monitor','/custom/docs']))
    expect(new Set(paths('api')).size).toBe(paths('api').length)
    expect(wrapper.findAll('[data-customer-section]').map(section => section.attributes('data-customer-section'))).toEqual(['overview', 'api', 'media', 'billing', 'other'])
    expect(paths('billing')).toEqual(['/billing','/subscriptions','/purchase','/orders','/coupons','/invoices','/redeem','/affiliate'])
    expect(paths('media')).toEqual(['/canvas/canvas?kitty_version=test','/video-tasks'])
    expect(wrapper.find('a[href="/apps"]').exists()).toBe(false)
    const canvasLink = wrapper.get('a[href="/canvas/canvas?kitty_version=test"]')
    expect(router.resolve(canvasLink.attributes('href')!).matched[0]?.path).toBe('/:pathMatch(.*)*')
    const push = vi.spyOn(router, 'push')
    const canvasClick = new MouseEvent('click', { bubbles: true, cancelable: true })
    canvasLink.element.dispatchEvent(canvasClick)
    await wrapper.vm.$nextTick()
    expect(canvasClick.defaultPrevented).toBe(false)
    expect(push).not.toHaveBeenCalled()
    expect(router.currentRoute.value.fullPath).toBe('/dashboard')
    push.mockRestore()
    expect(paths('other')).toEqual(['/guide','/community','/settings/mcp','/profile','/support-tickets'])
    expect(wrapper.find('button.sidebar-category').exists()).toBe(false)
    for (const section of ['api', 'billing', 'media', 'other']) expect(wrapper.get(`#sidebar-category-${section}`).isVisible()).toBe(true)
    const contacts = wrapper.findAll('[data-customer-section="other"] > div > button')
    expect(contacts.map(button => button.attributes('aria-label'))).toEqual(['nav.contactUs'])
    await contacts[0]!.trigger('click')
    expect(wrapper.findComponent({ name: 'CustomerContactDialog' }).props('initialTab')).toBe('service')

    wrapper.unmount()
  })

  it('shows the three requested badges only while expanded, without a studio link', async () => {
    appStore.sidebarCollapsed = false
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
    await router.push('/dashboard')
    await router.isReady()
    const wrapper = mount(AppSidebar, { global: { plugins: [router], stubs: { VersionBadge: true } } })
    expect(wrapper.get('a[href="/purchase"] .sidebar-promo-badge').text()).toBe('🎁充值有礼')
    expect(wrapper.get('a[href="/affiliate"] .sidebar-promo-badge').text()).toBe('💸邀请返现')
    expect(wrapper.get('a[href^="/canvas/canvas"] .sidebar-promo-badge').text()).toBe('🎨生图+视频')
    expect(wrapper.find('a[href="/apps"]').exists()).toBe(false)
    appStore.sidebarCollapsed = true
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('.sidebar-promo-badge')).toHaveLength(0)
    wrapper.unmount()
  })

  it('keeps administrator navigation groups collapsible', async () => {
    authStore.isAdmin = true
    appStore.sidebarCollapsed = false
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
    })
    await router.push('/admin/dashboard')
    await router.isReady()

    const wrapper = mount(AppSidebar, {
      global: { plugins: [router], stubs: { VersionBadge: true } }
    })

    expect(wrapper.find('a[href="/admin/profit-analysis"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/admin/harvest-flow"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/admin/candy-test"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/candy-test"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/admin/model-capability"]').exists()).toBe(false)

    expect(wrapper.find('a[href="/apps"]').exists()).toBe(false)
    const canvasLink = wrapper.get('a[href="/canvas/canvas?kitty_version=test"]')
    expect(router.resolve(canvasLink.attributes('href')!).matched[0]?.path).toBe('/:pathMatch(.*)*')
    const push = vi.spyOn(router, 'push')
    const canvasClick = new MouseEvent('click', { bubbles: true, cancelable: true })
    canvasLink.element.dispatchEvent(canvasClick)
    await wrapper.vm.$nextTick()
    expect(canvasClick.defaultPrevented).toBe(false)
    expect(push).not.toHaveBeenCalled()
    expect(router.currentRoute.value.fullPath).toBe('/admin/dashboard')
    push.mockRestore()

    const orderGroup = wrapper
      .findAll('.sidebar-nav > .sidebar-section:first-child > button.sidebar-link')
      .find((button) => button.text().includes('nav.orderManagement'))
    expect(orderGroup).toBeDefined()
    expect(wrapper.find('a[href="/admin/orders/plans"]').exists()).toBe(false)
    await orderGroup!.trigger('click')
    expect(wrapper.find('a[href="/admin/orders/plans"]').exists()).toBe(true)
    await orderGroup!.trigger('click')
    expect(wrapper.find('a[href="/admin/orders/plans"]').exists()).toBe(false)

    wrapper.unmount()
  })
})
