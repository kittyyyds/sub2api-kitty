import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppSidebar from '../AppSidebar.vue'

const appStore = reactive({
  backendModeEnabled: false,
  sidebarCollapsed: true,
  mobileOpen: false,
  siteName: 'KittyAPI',
  siteLogo: '',
  siteVersion: 'test',
  publicSettingsLoaded: true,
  cachedPublicSettings: { custom_menu_items: [] },
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
  useAuthStore: () => authStore,
  useAdminSettingsStore: () => adminSettingsStore,
  useOnboardingStore: () => onboardingStore
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage, refreshBatchImageAccess })
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    channelMonitor: 'channelMonitor',
    payment: 'payment',
    subscription: 'subscription',
    availableChannels: 'availableChannels',
    modelPlaza: 'modelPlaza',
    affiliate: 'affiliate',
    riskControl: 'riskControl',
    pluginManagement: 'pluginManagement'
  },
  makeSidebarFlag: () => () => true,
  resolveFeatureFlag: () => true
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'nav.appCenter': 'Kitty生图',
    'nav.onlineChat': '在线聊天',
    'nav.hotRecommendation': '火爆推荐',
    'nav.affiliateBadge': '返利'
  }
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => messages[key] ?? key })
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

describe('AppSidebar customer flat navigation', () => {
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
    expect(wrapper.find('a[href="/model-plaza?embedded=1"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/custom/billing-rules"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/custom/docs"]').exists()).toBe(false)

    const appsLink = wrapper.get('a[href="/apps"]')
    expect(appsLink.text()).toContain('Kitty生图')
    expect(wrapper.get('a[href="/chat"]').text()).toContain('在线聊天')
    const hotBadge = appsLink.get('[data-sidebar-promo="hot"]')
    expect(hotBadge.text()).toContain('🔥')
    expect(hotBadge.text()).toContain('火爆推荐')
    const dealBadge = wrapper.get('a[href="/purchase"] [data-sidebar-promo="deal"]')
    expect(dealBadge.text()).toContain('🎁')
    expect(dealBadge.text()).toContain('nav.limitedOffer')
    const rebateBadge = wrapper.get('a[href="/affiliate"] [data-sidebar-promo="rebate"]')
    expect(rebateBadge.text()).toContain('💸')
    expect(rebateBadge.text()).toContain('返利')

    const directUserPaths = wrapper
      .findAll('.sidebar-nav > .sidebar-section > a')
      .map((link) => link.attributes('href'))
    expect(directUserPaths).toEqual([
      '/dashboard',
      '/keys',
      '/chat',
      '/batch-image',
      '/usage',
      '/apps',
      '/available-channels',
      '/monitor',
      '/candy-test',
      '/custom/billing-rules',
      '/subscriptions',
      '/purchase',
      '/orders',
      '/redeem',
      '/affiliate',
      '/profile'
    ])

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
