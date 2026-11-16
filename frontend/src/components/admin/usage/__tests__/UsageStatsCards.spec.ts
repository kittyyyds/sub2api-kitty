import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheTotal': 'Cache',
  'usage.cacheBreakdown': 'Cache Token Breakdown',
  'usage.cacheCreationTokensLabel': 'Cache Creation',
  'usage.cacheReadTokensLabel': 'Cache Read',
  'usage.cacheHitRate': 'Cache hit rate',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Cost',
  'usage.standardCost': 'Standard',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const stats = {
  cache_hit_rate: 22 / 134,
  total_requests: 1,
  total_input_tokens: 100,
  total_output_tokens: 50,
  total_cache_tokens: 34,
  total_cache_creation_tokens: 12,
  total_cache_read_tokens: 22,
  total_tokens: 184,
  total_cost: 0.001,
  total_actual_cost: 0.001,
  total_account_cost: 0.001,
  average_duration_ms: 250,
}

describe('UsageStatsCards', () => {
  it('keeps total consumption, standard cost and account cost distinct', () => {
    const wrapper = mount(UsageStatsCards, {
      props: { stats: { ...stats, total_actual_cost: 3, total_cost: 12, total_account_cost: 5 } },
      global: { stubs: { Icon: true } },
    })
    const costCard = wrapper.get('.is-cost')
    expect(costCard.get('.usage-summary-value').text()).toBe('$3.0000')
    expect(costCard.text()).toContain('Standard $12.0000')
    expect(costCard.text()).toContain('Cost $5.0000')
    wrapper.unmount()
  })

  it('shows cache token breakdown values', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache 34')
    expect(wrapper.get('ul').attributes('aria-label')).toBe('Cache Token Breakdown')
    expect(text).toContain('Cache Creation')
    expect(text).toContain('12')
    expect(text).toContain('Cache Read')
    expect(text).toContain('22')
    expect(text).toContain('Cache hit rate: 16.4%')
  })

  it('shows every token component without requiring hover', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.get('[role="img"]').attributes('aria-label')).toContain('Cache Creation 12')
    expect(wrapper.findAll('li').map(item => item.text())).toEqual(['In100', 'Out50', 'Cache Creation12', 'Cache Read22'])
  })
})

 it.each([[null, '-'], [undefined, '-'], [0, '0.0%'], [0.8, '80.0%']] as const)(
  'uses eligible rate %s without recomputing from total traffic', (rate, expected) => {
    const wrapper = mount(UsageStatsCards, {
      props: { stats: { ...stats, total_input_tokens: 1000000, cache_hit_rate: rate } },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.get('[data-testid="total-cache-hit-rate"]').text()).toBe('Cache hit rate: ' + expected)
    expect(wrapper.get('ul').text()).toContain('In1.00M')
  },
)
