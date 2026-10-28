import { nextTick, shallowRef } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import { priceTableRows, groupPriceTableRows } from '../catalogTable'

const { copyToClipboard } = vi.hoisted(() => ({
  copyToClipboard: vi.fn().mockResolvedValue(true)
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (
        params?.group ? `${key}:${params.group}` : key
      )
    })
  }
})
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true })
}))

vi.mock('@/composables/useDocumentDarkMode', () => ({
  useDocumentDarkMode: () => ({ isDarkMode: shallowRef(false) })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard })
}))

function tokenModel(name: string): PlazaModel {
  return {
    name,
    platform: 'openai',
    pricing: {
      billing_mode: 'token',
      input_price: 1e-6,
      output_price: 2e-6,
      cache_write_price: null,
      cache_write_1h_price: null,
      cache_read_price: 2e-7,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 1e-6,
      output_price: 2e-6,
      cache_write_price: null,
      cache_read_price: 2e-7
    }
  }
}

function unitModel(name: string, platform: string, kind: 'image' | 'video' | 'per_request', unitPrice: number): PlazaModel {
  const model = tokenModel(name)
  model.platform = platform
  model.pricing!.billing_mode = kind
  model.pricing!.per_request_price = unitPrice
  model.official_pricing = null
  return model
}

function group(id: number, name: string, rate: number, models: PlazaModel[]): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: rate,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    long_context_pricing_enabled: true,
    models
  }
}

function mountContent(response: ModelPlazaResponse) {
  return mount(ModelPlazaContent, {
    props: { response, loading: false },
    global: {
      stubs: {
        Icon: true,
        ModelIcon: true,
        PlatformIcon: true
      }
    }
  })
}

describe('ModelPlazaContent model catalog', () => {
  beforeEach(() => copyToClipboard.mockClear())

  it('shows the 500K context tiers under one model identity without changing prices', async () => {
    const model = tokenModel('gpt-6.1-sol')
    model.pricing!.intervals = [
      { min_tokens: 0, max_tokens: 500000, tier_label: '≤500K', input_price: 2e-6, output_price: 10e-6, cache_write_price: null, cache_read_price: 0.1e-6, per_request_price: null },
      { min_tokens: 500000, max_tokens: null, tier_label: '>500K', input_price: 4e-6, output_price: 20e-6, cache_write_price: null, cache_read_price: 0.2e-6, per_request_price: null }
    ]
    const first = { ...group(44, 'Global fallback', 0.18, [model]), long_context_pricing_enabled: false }
    const wrapper = mountContent({ description: '', groups: [first, group(2, 'Single tier', 0.3, [tokenModel('gpt-other')])] })
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('.model-id-cell code').map(el => el.text())).toEqual(['gpt-6.1-sol'])
    expect(wrapper.get('.model-id-cell').attributes('rowspan')).toBe('2')
    expect(wrapper.get('.model-id-cell').attributes('scope')).toBe('rowgroup')
    expect(wrapper.findAll('.context-tier-range').map(el => el.text())).toEqual(['≤500K', '>500K'])
    expect(wrapper.findAll('.context-tier-badge').map(el => el.text())).toEqual(['modelPlaza.reference.standardContext', 'modelPlaza.reference.extendedContext'])
    expect(wrapper.findAll('tbody tr').map(row => row.get('.price-primary strong').text())).toEqual(['$0.36', '$0.72'])
    expect(wrapper.text()).not.toContain('272K')
    expect(wrapper.findAll('.copy-id-button')).toHaveLength(1)
    await wrapper.get('.copy-id-button').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('gpt-6.1-sol', 'modelPlaza.catalog.copySuccess')
    await wrapper.findAll('.pricing-group-card')[1].trigger('click')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.get('.model-id-cell').attributes('rowspan')).toBe('1')
    expect(wrapper.find('.context-tier-cell').exists()).toBe(false)
  })

  it('keeps time periods attached to their own prices inside the shared model block', () => {
    const model = tokenModel('timed-model')
    model.time_pricing = { timezone: 'Asia/Shanghai', weekdays_only: true, periods: [{ start_time: '00:00', end_time: '08:00', multiplier: 0.5 }] }
    const wrapper = mountContent({ description: '', groups: [group(2, 'Timed', 0.2, [model])] })
    expect(wrapper.findAll('.model-id-cell')).toHaveLength(1)
    expect(wrapper.get('.model-id-cell').attributes('rowspan')).toBe('2')
    const rows = wrapper.findAll('tbody tr')
    expect(rows[0].find('.context-period').exists()).toBe(false)
    expect(rows[1].get('.context-period').text()).toContain('00:00–08:00')
    expect(rows.map(row => row.get('.price-primary strong').text())).toEqual(['$0.20', '$0.10'])
  })

  it('starts with the first real group and renders its price table', () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, 'Stable', 0.8, [tokenModel('gpt-5.6-sol')]),
        group(2, 'Value', 0.2, [tokenModel('gpt-5.6-sol')])
      ]
    })

    expect(wrapper.get('.pricing-group-card').classes()).toContain('active')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.get('.pricing-group-card.active').text()).toContain('Stable')
    expect(wrapper.text()).toContain('$0.80')
    expect(wrapper.text()).toContain('modelPlaza.table.savings')
  })

  it('switches to a concrete group and only displays that group models', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, 'Stable', 0.8, [tokenModel('gpt-stable')]),
        group(2, 'Value', 0.2, [tokenModel('gpt-value')])
      ]
    })

    await wrapper.findAll('.pricing-group-card')[1].trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('gpt-value')
    expect(wrapper.text()).not.toContain('gpt-stable')
    expect(wrapper.get('.pricing-group-card.active').text()).toContain('Value')
  })

  it('shows a personal multiplier on the group chip', () => {
    const personalized = group(1, 'Personal', 0.8, [tokenModel('gpt-5.6')])
    personalized.user_rate_multiplier = 0.13
    const wrapper = mountContent({ description: '', groups: [personalized] })

    expect(wrapper.findAll('.pricing-group-card')[0].get('small').text()).toBe('×0.13')
    expect(wrapper.findAll('.pricing-group-card')[0].get('small').attributes('title'))
      .toBe('modelPlaza.catalog.personalRate')
  })

  it('switches official view without applying the customer multiplier', async () => {
    const model = tokenModel('gpt-test')
    model.pricing!.input_price = 3e-6
    const wrapper = mountContent({ description: '', groups: [group(1, 'Stable', 0.2, [model])] })
    expect(wrapper.get('.price-primary').text()).toContain('$0.60')
    await wrapper.findAll('.price-mode-switch button')[1].trigger('click')
    expect(wrapper.get('.price-primary').text()).toContain('$1.00')
  })

  it('copies the normalized model ID' , async () => {
    const model = tokenModel('publishers/openai/models/gpt-5.6-sol')
    const wrapper = mountContent({ description: '', groups: [group(1, 'Stable', 0.8, [model])] })

    await wrapper.get('.copy-id-button').trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith(
      'gpt-5.6-sol',
      'modelPlaza.catalog.copySuccess'
    )
  })

  it('emits retry from the error state', async () => {
    const wrapper = mount(ModelPlazaContent, {
      props: { response: null, loading: false, error: true },
      global: { stubs: { Icon: true } }
    })

    await wrapper.get('.retry-button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it.each(['openai', 'grok'])('renders real per-request prices beside token prices for %s', async (platform) => {
    const token = tokenModel('token-model')
    token.platform = platform
    const request = unitModel('request-model', platform, 'per_request', 0.15)
    const wrapper = mountContent({ description: '', groups: [group(1, 'Mixed billing', 0.2, [token, request])] })

    const tokenRow = wrapper.get('tbody tr[data-billing-kind="token"]')
    const requestRow = wrapper.get('tbody tr[data-billing-kind="per_request"]')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(tokenRow.findAll('td')).toHaveLength(5)
    expect(tokenRow.get('.price-primary').text()).toContain('$0.20')
    expect(requestRow.findAll('td')).toHaveLength(1)
    expect(requestRow.get('td').attributes('colspan')).toBe('5')
    expect(requestRow.text()).toContain('modelPlaza.catalog.kinds.per_request')
    expect(requestRow.get('.price-primary').text()).toBe('$0.03 / modelPlaza.catalog.units.request')

    await wrapper.findAll('.price-mode-switch button')[1].trigger('click')
    expect(tokenRow.get('.price-primary').text()).toContain('$1.00')
    expect(requestRow.get('.price-primary').text()).toBe('-')
    expect(requestRow.text()).toContain('modelPlaza.reference.noOfficial')
    expect(requestRow.text()).not.toContain('$0.03')
  })

  it('keeps mixed per-request and per-second video prices and units distinct', () => {
    const wrapper = mountContent({ description: '', groups: [{
      ...group(1, 'Video', 10, [
        unitModel('request-video', 'video', 'per_request', 9),
        unitModel('second-video', 'video', 'video', 0.25),
      ]),
      platform: 'video',
    }] })

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].get('.price-primary').text()).toBe('$9.00 / modelPlaza.video.perVideoUnit')
    expect(rows[1].get('.price-primary').text()).toBe('$0.25 / modelPlaza.catalog.units.second')
    expect(rows.every(row => row.findAll('td').length === 2)).toBe(true)
  })

  it.each(['image', 'video'] as const)('keeps same-name %s models distinguishable and correct after reordering', async (kind) => {
    const first = unitModel('shared-model', 'openai', kind, 0.1)
    const second = unitModel('shared-model', 'gemini', kind, 0.25)
    const catalogGroup = group(7, 'Multiple platforms', 0.5, [first, second])
    const wrapper = mountContent({ description: '', groups: [catalogGroup] })

    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('.model-platform').map(label => label.text())).toEqual(['OpenAI', 'Gemini'])
    expect(wrapper.findAll('tbody tr').map(row => row.get('.price-primary strong').text())).toEqual(['$0.05', '$0.125'])

    await wrapper.setProps({ response: { description: '', groups: [{ ...catalogGroup, models: [second, first] }] } })
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('.model-platform').map(label => label.text())).toEqual(['Gemini', 'OpenAI'])
    expect(wrapper.findAll('tbody tr').map(row => row.get('.price-primary strong').text())).toEqual(['$0.125', '$0.05'])
  })
})

describe('Kitty price table financial rules', () => {
  it('groups only token identities and leaves media specification rows intact', () => {
    const model = tokenModel('gpt')
    model.time_pricing = { timezone: 'UTC', periods: [{ start_time: '00:00', end_time: '01:00', multiplier: 2 }] }
    const rows = priceTableRows(group(1, 'Text', 0.2, [model, tokenModel('other')]), 'openai')
    expect(groupPriceTableRows(rows).map(block => block.rows.length)).toEqual([2, 1])
    expect(groupPriceTableRows(rows).flatMap(block => block.rows)).toEqual(rows)
    const image = unitModel('image', 'openai', 'image', 0.1)
    image.time_pricing = model.time_pricing
    const mediaRows = priceTableRows(group(1, 'Images', 0.2, [image]), 'image')
    expect(groupPriceTableRows(mediaRows).map(block => block.rows.length)).toEqual([1, 1])
  })
  it.each(['image', 'video'] as const)('uses stable group/platform/kind identities for same-name %s rows', (kind) => {
    const first = unitModel('shared-model', 'openai', kind, 0.1)
    const second = unitModel('shared-model', 'gemini', kind, 0.25)
    const catalogGroup = group(7, 'Multiple platforms', 0.5, [first, second])
    const rows = priceTableRows(catalogGroup, kind)
    const reversed = priceTableRows({ ...catalogGroup, models: [second, first] }, kind)

    expect(rows).toHaveLength(2)
    expect(new Set(rows.map(row => row.key)).size).toBe(2)
    expect(reversed.map(row => row.key)).toEqual([...rows].reverse().map(row => row.key))
    expect(rows[0].key).toContain(JSON.stringify([7, 'openai', kind, 'shared-model']))
    expect(rows[1].key).toContain(JSON.stringify([7, 'gemini', kind, 'shared-model']))
    expect(priceTableRows({ ...catalogGroup, id: 8 }, kind)[0].key).not.toBe(rows[0].key)
    expect(rows.map(row => row.metrics[0].current)).toEqual([0.05, 0.125])
  })

  it('applies the personal rate to channel prices and keeps official reference independent', () => {
    const model = tokenModel('gpt')
    model.pricing!.input_price = 3e-6
    const personalized = { ...group(1, 'Personal', 0.8, [model]), user_rate_multiplier: 0.13 }
    const row = priceTableRows(personalized, 'openai')[0]!
    expect(row.metrics.find(m => m.key === 'input')!.current).toBeCloseTo(0.39e-6)
    expect(row.metrics.find(m => m.key === 'input')!.official).toBe(1e-6)
  })
  it('resolves relative long-context prices and weekday time rates, including 1h cache', () => {
    const model = tokenModel('gpt')
    model.pricing!.cache_write_1h_price = 4e-6
    model.pricing!.intervals = [{ min_tokens: 200000, max_tokens: null, input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: null, input_multiplier: 2, output_multiplier: 3 }]
    model.time_pricing = { timezone: 'Asia/Shanghai', weekdays_only: true, periods: [{ start_time: '18:00', end_time: '23:00', multiplier: 1.5 }] }
    const rows = priceTableRows(group(1, 'Tiered', 0.2, [model]), 'openai')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.metrics.find(m => m.key === 'input')!.current).toBeCloseTo(0.4e-6)
    expect(rows[1]!.metrics.find(m => m.key === 'output')!.current).toBeCloseTo(1.8e-6)
    expect(rows[1]!.metrics.find(m => m.key === 'cache_write_1h')!.current).toBeCloseTo(1.2e-6)
    expect(rows[1]!.period).toContain('Mon–Fri')
  })
  it('preserves final image tier prices while applying independent image rates to other tiers', () => {
    const model = tokenModel('gpt-image-2')
    model.pricing!.billing_mode = 'image'
    model.pricing!.intervals = ['1K', '2K'].map(tier => ({ tier_label: tier, min_tokens: 0, max_tokens: null, input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: .02 }))
    model.final_image_price_tiers = ['1K']
    const imageGroup = { ...group(1, 'Images', 0.01, [model]), image_rate_independent: true, image_rate_multiplier: 2 }
    const rows = priceTableRows(imageGroup, 'image')
    expect(rows[0]!.metrics[0]!.current).toBe(.02)
    expect(rows[1]!.metrics[0]!.current).toBe(.04)
  })
  it('does not apply rates or time surcharges to the independent video module final prices', () => {
    const model = tokenModel('video-model')
    model.platform = 'video'
    model.pricing!.billing_mode = 'per_request'
    model.pricing!.per_request_price = 9
    model.time_pricing = { timezone: 'UTC', periods: [{ start_time: '00:00', end_time: '23:00', multiplier: 3 }] }
    const rows = priceTableRows({ ...group(1, 'Video', 10, [model]), platform: 'video', user_rate_multiplier: 5 }, 'video')
    expect(rows).toHaveLength(1)
    expect(rows[0]!.metrics[0]!.current).toBe(9)
  })
})
