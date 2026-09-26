import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppHeader from '../AppHeader.vue'

const shortcutSource = readFileSync(resolve(__dirname, '../CustomerHeaderShortcuts.vue'), 'utf8')
const contactSource = readFileSync(resolve(__dirname, '../CustomerContactActions.vue'), 'utf8')
const announcementSource = readFileSync(resolve(__dirname, '../../common/AnnouncementBell.vue'), 'utf8')

const appStore = reactive({
  contactInfo: '',
  docUrl: 'https://docs.example.test',
  cachedPublicSettings: { custom_menu_items: [], model_plaza_enabled: true },
  toggleMobileSidebar: vi.fn()
})

const authStore = reactive<{
  user: null | {
    username: string
    email: string
    role: 'user' | 'admin'
    balance: number
    frozen_balance: number
    avatar_url: string
  }
  isAdmin: boolean
  isSimpleMode: boolean
  logout: ReturnType<typeof vi.fn>
}>({
  user: null,
  isAdmin: false,
  isSimpleMode: false,
  logout: vi.fn()
})

const onboardingStore = { replay: vi.fn() }
const adminSettingsStore = { customMenuItems: [] }

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
  useOnboardingStore: () => onboardingStore
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => adminSettingsStore
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'nav.gettingStartedHeader': '新手必看',
    'nav.resetRadar': 'GPT智力测评',
    'nav.modelPlaza': '模型价格',
    'nav.customerShortcuts': '快捷功能'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

function createTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: { template: '<div />' } },
      { path: '/custom/billing-rules', component: { template: '<div />' } },
      { path: '/custom/docs', component: { template: '<div />' } },
      { path: '/custom/reset-radar', component: { template: '<div />' } },
      { path: '/model-plaza', component: { template: '<div />' } }
    ]
  })
}

async function mountHeader(role: 'user' | 'admin' | null) {
  authStore.user = role
    ? {
        username: 'tester',
        email: 'tester@example.test',
        role,
        balance: 10,
        frozen_balance: 0,
        avatar_url: ''
      }
    : null
  authStore.isAdmin = role === 'admin'
  const router = createTestRouter()
  await router.push('/dashboard')
  await router.isReady()
  const wrapper = mount(AppHeader, {
    global: {
      plugins: [router],
      mocks: { $t: (key: string) => key },
      stubs: {
        AnnouncementBell: { template: '<button data-testid="announcement-bell" />' },
        CustomerContactActions: { template: '<div data-testid="customer-contact-actions" />' },
        LocaleSwitcher: true,
        SubscriptionProgressMini: true,
        Icon: { props: ['name'], template: '<span :data-icon="name" />' }
      }
    }
  })
  return { router, wrapper }
}

beforeEach(() => {
  vi.clearAllMocks()
  authStore.user = null
  authStore.isAdmin = false
  authStore.isSimpleMode = false
  appStore.cachedPublicSettings.model_plaza_enabled = true
})

describe('AppHeader customer shortcuts', () => {
  it('keeps announcements and billing rules without the retired capability shortcut', async () => {
    const { router, wrapper } = await mountHeader('user')
    expect(wrapper.find('[data-customer-header-toolbar]').exists()).toBe(false)
    expect(wrapper.find('[data-header-shortcut]').exists()).toBe(false)
    expect(wrapper.find('[data-customer-header-guide]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="customer-contact-actions"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="announcement-bell"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/custom/billing-rules"]').exists()).toBe(true)
    expect(wrapper.findAll('.console-media-link').map(link => link.attributes('href'))).toEqual([])
    expect(wrapper.find('details').exists()).toBe(false)
    expect(wrapper.find('a[href="/intelligence-test"]').exists()).toBe(false)
    const billingRules = wrapper.get('a[href="/custom/billing-rules"]')
    expect(billingRules.classes()).toContain('console-intelligence-button')
    await billingRules.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/custom/billing-rules')
    wrapper.unmount()
  })

  it('does not duplicate pricing or documentation links in the customer header', async () => {
    const { wrapper } = await mountHeader('user')
    expect(wrapper.find('a[href="/model-plaza?embedded=1"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/custom/docs"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not show customer shortcuts to administrators or visitors', async () => {
    const admin = await mountHeader('admin')
    expect(admin.wrapper.find('a[href="/custom/docs"]').exists()).toBe(false)
    expect(admin.wrapper.find('[data-header-shortcut]').exists()).toBe(false)
    expect(admin.wrapper.find('[data-testid="customer-contact-actions"]').exists()).toBe(false)
    expect(admin.wrapper.find('[data-testid="announcement-bell"]').exists()).toBe(true)
    const adminPricing = admin.wrapper.get('a[href="/model-plaza?embedded=1"]')
    expect(adminPricing.classes()).toContain('flex')
    expect(adminPricing.classes()).not.toContain('hidden')
    expect(adminPricing.find('[data-icon="grid"]').exists()).toBe(true)
    admin.wrapper.unmount()

    const visitor = await mountHeader(null)
    expect(visitor.wrapper.find('[data-header-shortcut]').exists()).toBe(false)
    expect(visitor.wrapper.find('a[href="/model-plaza?embedded=1"]').exists()).toBe(false)
    visitor.wrapper.unmount()
  })

  it('uses the internal guide for customers and keeps external docs for administrators', async () => {
    const customer = await mountHeader('user')
    expect(customer.wrapper.find('a[href="https://docs.example.test/"]').exists()).toBe(false)
    expect(customer.wrapper.find('[data-customer-header-guide]').exists()).toBe(false)
    customer.wrapper.unmount()

    const { wrapper } = await mountHeader('admin')
    const externalDocs = wrapper.get('a[href="https://docs.example.test/"]')
    expect(externalDocs.attributes('target')).toBe('_blank')
    expect(externalDocs.attributes('rel')).toContain('noopener')
    wrapper.unmount()
  })

  it('never hides customer action labels at narrower desktop widths', () => {
    expect(shortcutSource).not.toMatch(/customer-header-shortcut__label\s*\{[^}]*display:\s*none/s)
    expect(contactSource).not.toMatch(/customer-contact-action__label\s*\{[^}]*display:\s*none/s)
    expect(announcementSource).not.toMatch(/announcement-header-button__label\s*\{[^}]*display:\s*none/s)
  })
})
