import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import DashboardOverview from '../DashboardOverview.vue'
import { homeMessages } from '@/i18n/homeOverview'
import type { UserDashboardStats } from '@/api/usage'

const M = 1e6
const stats: UserDashboardStats = {
  total_api_keys: 11, active_api_keys: 9, total_requests: 96420,
  total_input_tokens: 1053.56 * M, total_output_tokens: 91.59 * M, total_cache_creation_tokens: 130.83 * M, total_cache_read_tokens: 7200 * M,
  total_tokens: 8475.98 * M, total_cost: 7074.5, total_actual_cost: 1414.9,
  today_requests: 1263, today_input_tokens: 7.96 * M, today_output_tokens: 817560, today_cache_creation_tokens: 2.44 * M, today_cache_read_tokens: 127 * M,
  today_tokens: 138.21 * M, today_cost: 21.28, today_actual_cost: 4.2571, average_duration_ms: 1240, rpm: 1, tpm: 9870,
}

function render(overrides: Partial<UserDashboardStats> = {}, balance = 997969.0194) {
  return mount(DashboardOverview, {
    props: { stats: { ...stats, ...overrides }, balance, isSimple: false, rechargeEnabled: true },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { homeOverview: homeMessages.zh } } })],
      stubs: { Icon: true, DashboardAnnouncementFeed: { template: '<section data-testid="feed" />' } },
    },
  })
}
const metric = (wrapper: ReturnType<typeof render>, key: string) => {
  const card = wrapper.get(`[data-metric="${key}"]`)
  return {
    value: card.get('strong').text(),
    hint: card.get('p').text(),
    details: Object.fromEntries(card.findAll('.home-metric-details > div').map(row => [row.get('dt').text(), row.get('dd').text()])),
  }
}

describe('dashboard overview figures', () => {
  it('shows each figure with its breakdown', () => {
    const wrapper = render()
    expect(metric(wrapper, 'today-cost')).toEqual({ value: '$4.26', hint: '按实际扣费统计', details: { 今日请求: '1,263' } })
    expect(metric(wrapper, 'total-cost')).toEqual({ value: '$1,414.90', hint: '账户累计实际消费', details: { 累计请求: '96,420' } })
    expect(metric(wrapper, 'today-tokens')).toEqual({ value: '138.21M', hint: '含缓存 Token 129.44M', details: { 输入: '7.96M', 输出: '817.56K', 缓存占比: '93.7%' } })
    expect(metric(wrapper, 'total-tokens')).toEqual({ value: '8.48B', hint: '含缓存 Token 7.33B', details: { 输入: '1.05B', 输出: '91.59M', 缓存占比: '86.5%' } })
    wrapper.unmount()
  })

  it('keeps four decimals only for sub-dollar amounts and does not divide by zero', () => {
    const wrapper = render({ today_actual_cost: 0.0034, today_tokens: 0, today_input_tokens: 0, today_output_tokens: 0, today_cache_creation_tokens: 0, today_cache_read_tokens: 0, today_requests: 0 })
    expect(metric(wrapper, 'today-cost').value).toBe('$0.0034')
    expect(metric(wrapper, 'today-tokens')).toEqual({ value: '0', hint: '含缓存 Token 0', details: { 输入: '0', 输出: '0', 缓存占比: '—' } })
    wrapper.unmount()
  })

  it('writes the balance like the header and spells out the key count', () => {
    const wrapper = render()
    expect(wrapper.get('[data-testid="home-balance"] dd').text()).toBe('$997,969.02')
    expect(wrapper.get('.home-account-rows').text()).toContain('启用 9 / 共 11')
    expect(wrapper.get('[data-testid="home-rpm"]').text()).toBe('1')
    expect(wrapper.get('[data-testid="home-tpm"]').text()).toBe('9.87K')
    wrapper.unmount()
  })

  it('leaves the shortcuts to the welcome card', () => {
    const wrapper = render()
    expect(wrapper.find('[data-dashboard-quick-actions]').exists()).toBe(false)
    expect(wrapper.get('.home-aside').element.children).toHaveLength(1)
    wrapper.unmount()
  })
})
