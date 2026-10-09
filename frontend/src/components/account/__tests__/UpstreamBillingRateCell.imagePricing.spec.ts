import { expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import UpstreamBillingRateCell from '../UpstreamBillingRateCell.vue'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'
import type { Account } from '@/types'

it('shows the image account selected model costs without interpreting legacy token rates', () => {
  const account = {
    platform: 'openai', type: 'apikey',
    credentials: {}, // The compact account table intentionally excludes credentials.
    extra: { upstream_billing_probe: {
      kind: 'image_model_prices', status: 'ok', received_at: '2026-10-10T00:00:00Z', fresh_until: '2026-10-10T02:00:00Z',
      last_attempt_at: '', next_probe_at: '', synced_rate_multiplier: 99,
      image_pricing: { version: 'v', last_success: '', last_attempt: '', last_error: '', models: {
        'Image-A': { model: 'Image-A', unit_price: 0.08, billing_mode: 'per_request' },
      } },
    } },
  } as Account
  const wrapper = mount(UpstreamBillingRateCell, {
    props: { account, now: Date.parse('2026-10-10T00:30:00Z') },
    global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { admin: zhAccounts } } })],
      stubs: { HelpTooltip: { template: '<div><slot name="trigger" /><slot /></div>' } } },
  })
  expect(wrapper.text()).toContain('0.08/次')
  expect(wrapper.text()).toContain('模型价格（1）')
  expect(wrapper.text()).not.toContain('Unselected')
  expect(wrapper.text()).not.toContain('99')
  wrapper.unmount()
})
