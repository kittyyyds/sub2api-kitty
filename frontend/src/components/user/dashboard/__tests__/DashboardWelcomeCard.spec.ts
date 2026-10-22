import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import DashboardWelcomeCard from '../DashboardWelcomeCard.vue'
import { homeMessages } from '@/i18n/homeOverview'

beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 9, 8, 19)) })
afterEach(() => { vi.useRealTimers(); localStorage.clear() })
function render(rechargeEnabled = true, isSimple = false, username = '张三') {
  return mount(DashboardWelcomeCard, {
    props: { accountId: 82001, username, rechargeEnabled, isSimple },
    global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { homeOverview: homeMessages.zh } } })], stubs: { Icon: true, RouterLink: RouterLinkStub, CustomerContactDialog: { props: ['show'], emits: ['close'], template: '<div v-if="show" role="dialog"><button @click="$emit(\'close\')">关闭</button></div>' } } },
  })
}
describe('welcome card actions and clock', () => {
  it('greets the current username instead of a hardcoded brand', async () => {
    const wrapper = render(true, false, '  张三  ')
    expect(wrapper.get('h1').text()).toBe('晚上好，张三')
    await wrapper.setProps({ accountId: 82002, username: '李四' })
    expect(wrapper.get('h1').text()).toBe('晚上好，李四')
    expect(wrapper.get('h1').text()).not.toContain('张三')
    wrapper.unmount()
  })
  it('does not invent a username before the profile loads or when it is empty', async () => {
    const wrapper = render(true, false, '')
    expect(wrapper.get('h1').text()).toBe('晚上好')
    await wrapper.setProps({ username: '   ' })
    expect(wrapper.get('h1').text()).toBe('晚上好')
    await wrapper.setProps({ username: undefined })
    expect(wrapper.get('h1').text()).toBe('晚上好')
    await wrapper.setProps({ username: '真实用户' })
    expect(wrapper.get('h1').text()).toBe('晚上好，真实用户')
    wrapper.unmount()
  })
  it('uses only the current account email when its username is missing', async () => {
    const wrapper = render(true, false, '')
    await wrapper.setProps({ email: 'alice@example.test' })
    expect(wrapper.get('h1').text()).toBe('晚上好，alice')
    await wrapper.setProps({ accountId: 82002, username: undefined, email: 'bob@example.test' })
    expect(wrapper.get('h1').text()).toBe('晚上好，bob')
    expect(wrapper.get('h1').text()).not.toContain('alice')
    await wrapper.setProps({ username: '李四' })
    expect(wrapper.get('h1').text()).toBe('晚上好，李四')
    await wrapper.setProps({ accountId: undefined, username: undefined, email: undefined })
    expect(wrapper.get('h1').text()).toBe('晚上好')
    wrapper.unmount()
  })
  it('renders the username as text rather than HTML', () => {
    const wrapper = render(true, false, '<b>小明</b>')
    expect(wrapper.get('h1').text()).toBe('晚上好，<b>小明</b>')
    expect(wrapper.find('h1 b').exists()).toBe(false)
    wrapper.unmount()
  })
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
    expect(wrapper.get('h1').text()).toBe('晚上好，张三')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(wrapper.get('h1').text()).toBe('夜深了，张三')
    expect(wrapper.get('[data-testid="daily-quote"]').text()).not.toBe(before)
    const activeTimers = vi.getTimerCount()
    wrapper.unmount()
    // vue-i18n may keep its own deferred devtools task. Only this card's
    // clock should disappear when the card is unmounted.
    expect(vi.getTimerCount()).toBe(activeTimers - 1)
  })
})
