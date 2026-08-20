import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import CustomerContactActions from '../CustomerContactActions.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'nav.customerContact': '客户服务',
      'nav.customerService': '人工客服',
      'nav.customerServiceQrHint': '扫码或点击按钮添加 QQ 好友',
      'nav.customerSupportTitle': '客服支持',
      'nav.customerSupportQq': 'QQ 客服',
      'nav.customerSupportWechat': '微信客服',
      'nav.customerSupportQqQrAlt': 'QQ 客服二维码',
      'nav.customerSupportWechatQrAlt': '微信客服二维码',
      'nav.customerSupportScanHint': '扫码添加客服',
      'nav.afterSalesGroup': '售后交流群',
      'nav.afterSalesGroupQrHint': '扫码或点击按钮加入群聊',
      'common.close': '关闭',
    }[key] ?? key),
  }),
}))

describe('CustomerContactActions', () => {
  it('opens a compact QQ and WeChat support panel while preserving the group link', async () => {
    const wrapper = mount(CustomerContactActions, {
      global: {
        stubs: {
          Icon: { props: ['name'], template: '<span :data-icon="name" />' },
        },
      },
    })

    expect(wrapper.find('[data-customer-support-popover]').exists()).toBe(false)
    const support = wrapper.get('.customer-support')
    await support.trigger('mouseenter')
    await nextTick()

    const trigger = wrapper.get('[data-customer-support-trigger]')
    const popover = wrapper.get('[data-customer-support-popover]')
    const supportImages = popover.findAll('img')
    expect(trigger.text()).toContain('人工客服')
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(trigger.find('[data-icon="userPlus"]').exists()).toBe(true)
    expect(supportImages.map((image) => image.attributes('src'))).toEqual([
      '/images/heyroute/support/qq-qr.png',
      '/images/heyroute/support/wechat-qr.png'
    ])
    expect(supportImages.every((image) => image.attributes('loading') === 'lazy')).toBe(true)
    expect(popover.text()).toContain('1722345668')
    expect(popover.text()).toContain('ALe990411')

    const group = wrapper.get('a[href="https://qm.qq.com/q/oRR8GUdEPK"]')
    expect(group.text()).toContain('售后交流群')
    expect(group.find('[data-icon="users"]').exists()).toBe(true)
    expect(group.get('img').attributes('src')).toBe('/qq-group-qr.png')

    wrapper.unmount()
  })
})
