import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import UpstreamBillingRateCell from '../UpstreamBillingRateCell.vue'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'
import enAccounts from '@/i18n/locales/en/admin/accounts'
import type { Account } from '@/types'

describe('video model price units', () => {
  it.each([
    { locale: 'zh', messages: zhAccounts, costLabel: '接口原值', second: '/秒', request: '/次' },
    { locale: 'en', messages: enAccounts, costLabel: 'API value', second: '/second', request: '/request' }
  ])('renders unrounded API values and units without assuming currency ($locale)', ({ locale, messages, costLabel, second, request }) => {
    const account = {
      id: 42, platform: 'video', type: 'apikey',
      extra: {
        upstream_billing_probe: {
          kind: 'video_model_prices', status: 'ok',
          received_at: '2026-10-09T00:00:00Z', fresh_until: '2026-10-09T02:00:00Z',
          last_attempt_at: '2026-10-09T00:00:00Z', next_probe_at: '2026-10-09T01:00:00Z',
          video_pricing: {
            version: 'new-catalog', last_attempt: '', last_success: '2026-10-09T00:00:00Z', last_error: '', changes: [],
            models: {
              'new-video-model': { model: 'new-video-model', unit_price: 0.0123456789, billing_mode: 'video', reference_per_second: 0.00123, resolution_prices: { '1080p': 0.025 } },
              'new-request-model': { model: 'new-request-model', unit_price: 2.5, billing_mode: 'per_request', reference_per_second: 0 }
            }
          }
        }
      }
    } as Account
    const wrapper = mount(UpstreamBillingRateCell, {
      props: { account, now: Date.parse('2026-10-09T00:30:00Z') },
      global: {
        plugins: [createI18n({ legacy: false, locale, messages: { [locale]: { admin: messages } } })],
        stubs: { HelpTooltip: { template: '<div><slot name="trigger" /><slot /></div>' } }
      }
    })
    const table = wrapper.get('[data-testid="video-pricing-models"]').text()
    expect(table).toContain(costLabel)
    expect(table).toContain('new-video-model')
    expect(table).toContain(`0.0123456789${second}`)
    expect(table).toContain(`0.00123${second}`)
    expect(table).toContain(`1080p: 0.025${second}`)
    expect(table).toContain(`2.5${request}`)
    expect(wrapper.text()).not.toMatch(/USD|CNY|\$|￥/)
    expect(wrapper.get('[data-testid="video-pricing-public-default"]').text()).toBe(messages.accounts.upstreamBilling.video.publicDefault)
    wrapper.unmount()
  })
})
