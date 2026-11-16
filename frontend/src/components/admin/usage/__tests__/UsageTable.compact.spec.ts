import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UsageTable from '../UsageTable.vue'
import zh from '@/i18n/locales/zh/dashboard'

vi.mock('@/stores/app', () => ({ useAppStore: () => ({}) }))
vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key.startsWith('usage.') ? (zh.usage as Record<string, string>)[key.slice(6)] || key : key }),
}))

const baseRow = {
  id: 1, first_token_ms: 2790, duration_ms: 5350, output_tokens: 143,
  input_tokens: 20, cache_read_tokens: 975, cache_creation_tokens: 5,
}

const render = (overrides = {}) => mount(UsageTable, {
  props: { data: [{ ...baseRow, ...overrides }], columns: [{ key: 'latency', label: '延迟' }] },
  global: { stubs: {
    DataTable: { props: ['data'], template: '<div><slot name="cell-latency" :row="data[0]" /></div>' },
    EmptyState: true, Icon: true, Teleport: true,
  } },
})

describe('compact usage latency layout', () => {
  it('places cache hit rate under speed with short Chinese labels', () => {
    const wrapper = render()
    const text = wrapper.text()
    for (const label of ['首字', '总耗时', '平均速度', '缓存命中率']) expect(text).toContain(label)
    expect(text).not.toContain('含转发前等待')
    expect(text).not.toContain('服务端首字')
    expect(wrapper.get('[data-testid="cache-hit-rate"]').text()).toBe('97.5%')
    expect(wrapper.get('[data-testid="output-tps"]').text()).toBe('26.7 tok/s')
    expect(text.indexOf('平均速度')).toBeLessThan(text.indexOf('缓存命中率'))
    wrapper.unmount()
  })

  it('distinguishes no cache hits from an unavailable prompt denominator', () => {
    const miss = render({ cache_read_tokens: 0 })
    expect(miss.get('[data-testid="cache-hit-rate"]').text()).toBe('0.0%')
    miss.unmount()
    const empty = render({ input_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0 })
    expect(empty.get('[data-testid="cache-hit-rate"]').text()).toBe('—')
    empty.unmount()
  })
})
