import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import DashboardWelcomeCard from '../DashboardWelcomeCard.vue'
import { homeMessages } from '@/i18n/homeOverview'

afterEach(() => { vi.useRealTimers(); localStorage.clear() })
function render(rechargeEnabled = true, isSimple = false) {
  return mount(DashboardWelcomeCard, {
    props: { accountId: 82001, username: 'Kitty', rechargeEnabled, isSimple },
    global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { homeOverview: homeMessages.zh } } })], stubs: { Icon: true, RouterLink: RouterLinkStub, CustomerContactDialog: { props: ['show'], emits: ['close'], template: '<div v-if="show" role="dialog"><button @click="$emit(\'close\')">关闭</button></div>' } } },
  })
}
describe('welcome card actions and clock', () => {
  it('opens the existing support component and links to the balance purchase page', async () => {
    const wrapper = render()
    expect(wrapper.getComponent(RouterLinkStub).props('to')).toBe('/purchase')
    await wrapper.get('.home-welcome-contact').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    await wrapper.get('[role="dialog"] button').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('respects payment and simple-mode permissions', () => {
    for (const [enabled, simple] of [[false, false], [true, true]]) {
      const wrapper = render(enabled, simple)
      expect(wrapper.findComponent(RouterLinkStub).exists()).toBe(false)
      expect(wrapper.find('.home-welcome-contact').exists()).toBe(true)
      wrapper.unmount()
    }
  })
  it('updates a page kept open across midnight and stops its timer on unmount', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 9, 8, 23, 59, 50))
    const wrapper = render(), before = wrapper.get('[data-testid="daily-quote"]').text()
    expect(wrapper.get('h1').text()).toContain('晚上好')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(wrapper.get('h1').text()).toContain('夜深了')
    expect(wrapper.get('[data-testid="daily-quote"]').text()).not.toBe(before)
    const activeTimers = vi.getTimerCount()
    wrapper.unmount()
    // vue-i18n may keep its own deferred devtools task. Only this card's
    // clock should disappear when the card is unmounted.
    expect(vi.getTimerCount()).toBe(activeTimers - 1)
  })
})
