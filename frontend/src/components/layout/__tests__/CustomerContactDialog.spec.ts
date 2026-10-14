import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import CustomerContactDialog from '../CustomerContactDialog.vue'
import { afterSalesGroupContact, customerSupportContacts } from '@/config/customerContact'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('CustomerContactDialog', () => {
  it('uses Kitty contact destinations and switches categories from the keyboard', async () => {
    const wrapper = mount(CustomerContactDialog, {
      props: { show: true },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, RouterLink: true } },
    })
    expect(wrapper.get('img').attributes('src')).toBe(customerSupportContacts.wechat.qrSrc)
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(3)
    await wrapper.get('[role="tab"][aria-selected="true"]').trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('nav.contactAfterSalesTab')
    expect(wrapper.get('a').attributes('href')).toBe(afterSalesGroupContact.href)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, initialTab: 'service' })
    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('nav.contactServiceTab')
    wrapper.unmount()
  })
})
