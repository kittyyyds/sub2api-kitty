import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountUsageSummaryGrid from '../AccountUsageSummaryGrid.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
    t: (key: string) => ({
      'admin.accounts.stats.todayGroup': '今日',
      'admin.accounts.stats.totalGroup': '总计',
      'admin.accounts.stats.summaryRequests': '请求',
      'admin.accounts.stats.summaryTokens': 'Token',
      'admin.accounts.stats.accountBilled': '账号计费',
      'admin.accounts.stats.userBilled': '用户扣费',
      'usage.accountBilled': '账号计费',
      'usage.userBilled': '用户扣费'
    })[key] ?? key
    })
  }
})

describe('AccountUsageSummaryGrid', () => {
  it('shows four today metrics beside four lifetime totals', () => {
    const wrapper = mount(AccountUsageSummaryGrid, {
      props: {
        stats: {
          requests: 12,
          tokens: 345,
          cost: 14.61,
          user_cost: 2.84,
          lifetime_requests: 900,
          lifetime_tokens: 12_345,
          lifetime_cost: 543.22,
          lifetime_user_cost: 123.45
        }
      }
    })

    const today = wrapper.get('[data-testid="today-stats"]')
    const total = wrapper.get('[data-testid="lifetime-stats"]')
    expect(today.text()).toContain('今日')
    expect(today.text()).toContain('12 req')
    expect(today.text()).toContain('345')
    expect(today.text()).toContain('A $14.61')
    expect(today.text()).toContain('U $2.84')
    expect(total.text()).toContain('总计')
    expect(total.text()).toContain('900 req')
    expect(total.text()).toContain('12.3K')
    expect(total.text()).toContain('A $543.22')
    expect(total.text()).toContain('U $123.45')
    expect(today.findAll('.flex')).toHaveLength(4)
    expect(total.findAll('.flex')).toHaveLength(4)
  })

  it('shows a dash when an all-time metric is unavailable', () => {
    const wrapper = mount(AccountUsageSummaryGrid, {
      props: { stats: { requests: 0, tokens: 0, cost: 0 } }
    })

    expect(wrapper.get('[data-testid="lifetime-stats"]').text()).toContain('—')
  })
})
