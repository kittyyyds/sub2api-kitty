import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import UserDashboardCharts from './UserDashboardCharts.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('vue-chartjs', () => ({
  Doughnut: {
    props: ['data'],
    template: '<div class="model-chart-data">{{ JSON.stringify(data) }}</div>'
  },
  Line: { template: '<div />' }
}))

enableAutoUnmount(afterEach)

afterEach(() => {
  document.documentElement.classList.remove('dark')
})

describe('UserDashboardCharts customer palette', () => {
  it('updates the model chart when the document theme changes', async () => {
    const wrapper = mount(UserDashboardCharts, {
      props: {
        loading: false,
        startDate: '2026-07-01',
        endDate: '2026-07-16',
        granularity: 'day',
        trend: [],
        models: [
          {
            model: 'gpt-test',
            requests: 1,
            input_tokens: 10,
            output_tokens: 5,
            cache_creation_tokens: 0,
            cache_read_tokens: 0,
            total_tokens: 15,
            cost: 0.01,
            actual_cost: 0.005
          }
        ]
      },
      global: {
        stubs: {
          DateRangePicker: true,
          LoadingSpinner: true,
          Select: true,
          TokenUsageTrend: true
        }
      }
    })

    const readDataset = () =>
      JSON.parse(wrapper.get('.model-chart-data').text()).datasets[0]

    expect(readDataset().backgroundColor[0]).toBe('#0a7f90')
    expect(readDataset().borderColor).toBe('#f8fbfc')

    document.documentElement.classList.add('dark')

    await vi.waitFor(() => {
      expect(readDataset().backgroundColor[0]).toBe('#8eeaff')
      expect(readDataset().borderColor).toBe('#061014')
    })
  })
})
