import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'

import TokenUsageTrend from '../TokenUsageTrend.vue'

const messages: Record<string, string> = {
  'admin.dashboard.tokenUsageTrend': 'Token Usage Trend',
  'admin.dashboard.noDataAvailable': 'No data available',
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

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>',
  },
}))

enableAutoUnmount(afterEach)

afterEach(() => {
  document.documentElement.classList.remove('dark')
})

describe('TokenUsageTrend', () => {
  it('uses the eligible backend rate despite synchronous input in token totals', () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 100500,
            cache_hit_rate: 0.75,
            output_tokens: 100,
            cache_creation_tokens: 0,
            cache_read_tokens: 1500,
            cost: 0.01,
            actual_cost: 0.005,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    const hitRateDataset = chartData.datasets.find(
      (ds: any) => ds.label === 'Cache Hit Rate'
    )
    // Hit rate = 1500 / (500 + 1500 + 0) * 100 = 75%
    expect(hitRateDataset.data[0]).toBe(75)
  })

  it('leaves a gap when no eligible prompt tokens exist', () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 0,
            input_tokens: 0,
            output_tokens: 0,
            cache_creation_tokens: 0,
            cache_read_tokens: 0,
            cost: 0,
            actual_cost: 0,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    const hitRateDataset = chartData.datasets.find(
      (ds: any) => ds.label === 'Cache Hit Rate'
    )
    expect(hitRateDataset.data[0]).toBeNull()
  })

  it('includes cache_creation_tokens in denominator for Anthropic models', () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 200,
            cache_hit_rate: 0.5,
            output_tokens: 50,
            cache_creation_tokens: 300,
            cache_read_tokens: 500,
            cost: 0.02,
            actual_cost: 0.01,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    const hitRateDataset = chartData.datasets.find(
      (ds: any) => ds.label === 'Cache Hit Rate'
    )
    // Hit rate = 500 / (200 + 500 + 300) * 100 = 50%
    expect(hitRateDataset.data[0]).toBe(50)
  })

  it('uses the scoped customer palette without changing the default chart variant', () => {
    document.documentElement.classList.add('dark')
    const trendData = [
      {
        date: '2026-07-16',
        requests: 1,
        input_tokens: 100,
        output_tokens: 50,
        cache_creation_tokens: 20,
        cache_read_tokens: 30,
        cost: 0.01,
        actual_cost: 0.005,
      },
    ]

    const customerWrapper = mount(TokenUsageTrend, {
      props: { trendData, variant: 'customer' },
      global: { stubs: { LoadingSpinner: true } },
    })
    const customerData = JSON.parse(customerWrapper.find('.chart-data').text())

    expect(customerData.datasets.map((dataset: any) => dataset.borderColor)).toEqual([
      '#8eeaff',
      '#84d9b0',
      '#ffad68',
      '#a2b2b6',
      '#5dbfd0',
    ])
    expect(customerData.datasets[2].borderDash).toEqual([10, 6])
    expect(customerData.datasets[4].borderDash).toEqual([2, 3, 8, 3])

    const defaultWrapper = mount(TokenUsageTrend, {
      props: { trendData },
      global: { stubs: { LoadingSpinner: true } },
    })
    const defaultData = JSON.parse(defaultWrapper.find('.chart-data').text())
    expect(defaultData.datasets[0].borderColor).toBe('#3b82f6')
    expect(defaultData.datasets[4].borderColor).toBe('#8b5cf6')
  })

  it('updates the customer palette when the document theme changes', async () => {
    const trendData = [
      {
        date: '2026-07-16',
        requests: 1,
        input_tokens: 100,
        output_tokens: 50,
        cache_creation_tokens: 20,
        cache_read_tokens: 30,
        cost: 0.01,
        actual_cost: 0.005,
      },
    ]
    const wrapper = mount(TokenUsageTrend, {
      props: { trendData, variant: 'customer' },
      global: { stubs: { LoadingSpinner: true } },
    })

    expect(JSON.parse(wrapper.find('.chart-data').text()).datasets[0].borderColor).toBe(
      '#0a7f90'
    )

    document.documentElement.classList.add('dark')

    await vi.waitFor(() => {
      expect(JSON.parse(wrapper.find('.chart-data').text()).datasets[0].borderColor).toBe(
        '#8eeaff'
      )
    })
  })
})
