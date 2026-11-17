import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CustomerPageTabs from '../CustomerPageTabs.vue'
import type { PublicSettings } from '@/types'

const appStore = reactive({ cachedPublicSettings: {} as Partial<PublicSettings> })
const authStore = reactive({ isSimpleMode: false })

vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

async function render(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(CustomerPageTabs, { global: { plugins: [router], stubs: { Icon: { props: ['name'], template: '<span :data-icon="name" />' } } } })
  const tabs = () => wrapper.findAll('[data-customer-page-tabs] a').map(link => link.attributes('href') + (link.attributes('aria-current') ? '*' : ''))
  return { router, wrapper, tabs }
}

beforeEach(() => {
  appStore.cachedPublicSettings = { available_channels_enabled: true }
  authStore.isSimpleMode = false
})

describe('CustomerPageTabs', () => {
  it('shows the sibling pages of the current route and marks the current one', async () => {
    const { router, wrapper, tabs } = await render('/orders')
    expect(tabs()).toEqual(['/billing', '/orders*', '/invoices'])
    await router.push('/coupons')
    await wrapper.vm.$nextTick()
    expect(tabs()).toEqual(['/purchase', '/redeem', '/coupons*'])
    wrapper.unmount()
  })

  it('lists channel status before available channels and follows nested routes', async () => {
    const { wrapper, tabs } = await render('/monitor/12')
    expect(tabs()).toEqual(['/monitor*', '/available-channels'])
    wrapper.unmount()
  })

  it('renders nothing on a page that belongs to no group', async () => {
    const { wrapper } = await render('/dashboard')
    expect(wrapper.find('[data-customer-page-tabs]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('drops pages whose feature is switched off and hides a strip with a single page left', async () => {
    appStore.cachedPublicSettings = { available_channels_enabled: false, payment_enabled: false }
    const channels = await render('/monitor')
    expect(channels.wrapper.find('[data-customer-page-tabs]').exists()).toBe(false)
    channels.wrapper.unmount()

    const billing = await render('/billing')
    expect(billing.wrapper.find('[data-customer-page-tabs]').exists()).toBe(false)
    billing.wrapper.unmount()

    const recharge = await render('/redeem')
    expect(recharge.tabs()).toEqual(['/redeem*', '/coupons'])
    recharge.wrapper.unmount()
  })

  it('hides the billing and recharge strips in simple mode', async () => {
    authStore.isSimpleMode = true
    const { wrapper } = await render('/billing')
    expect(wrapper.find('[data-customer-page-tabs]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('marks the guide as a page outside the console', async () => {
    const { wrapper, tabs } = await render('/custom/docs')
    expect(tabs()).toEqual(['/custom/docs*', '/guide'])
    expect(wrapper.get('a[href="/guide"]').find('[data-icon="externalLink"]').exists()).toBe(true)
    expect(wrapper.get('a[href="/custom/docs"]').find('[data-icon]').exists()).toBe(false)
    wrapper.unmount()
  })
})
