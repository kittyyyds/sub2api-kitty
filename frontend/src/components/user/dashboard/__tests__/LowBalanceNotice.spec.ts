import { describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import LowBalanceNotice from '../LowBalanceNotice.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('LowBalanceNotice', () => {
  it.each([0, -2])('offers recharge for depleted balance %s', (balance) => {
    const wrapper = mount(LowBalanceNotice, { props: { balance, rechargeEnabled: true }, global: { stubs: { RouterLink: RouterLinkStub, Icon: true } } })
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    expect(wrapper.getComponent(RouterLinkStub).props('to')).toBe('/purchase')
  })
  it.each([0.01, 50, NaN])('does not show an insufficient balance warning for %s', (balance) => {
    const wrapper = mount(LowBalanceNotice, { props: { balance, rechargeEnabled: true } })
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })
  it('does not offer unavailable recharge when balance payments are disabled', () => {
    const wrapper = mount(LowBalanceNotice, { props: { balance: 0, rechargeEnabled: false }, global: { stubs: { RouterLink: RouterLinkStub, Icon: true } } })
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    expect(wrapper.findComponent(RouterLinkStub).exists()).toBe(false)
  })
})
