import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import type { PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function model(name: string, billingMode: 'token' | 'image' | 'video' | 'per_request' = 'token'): PlazaModel {
  return {
    name,
    platform: 'grok',
    pricing: {
      billing_mode: billingMode,
      input_price: billingMode === 'token' ? 1e-6 : null,
      output_price: billingMode === 'token' ? 2e-6 : null,
      cache_write_price: billingMode === 'token' ? 5e-7 : null,
      cache_read_price: billingMode === 'token' ? 2e-7 : null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: null
  }
}

function mountTable(models: PlazaModel[], props: Record<string, unknown> = {}) {
  return mount(PlazaModelPricingTable, {
    props: { models, rateMultiplier: 0.5, ...props }
  })
}

describe('PlazaModelPricingTable customer pricing ledger', () => {
  it.each(['video', 'per_request'] as const)('shows video-platform %s balance prices without multipliers or currency conversion', (billingMode) => {
    const video = { ...model('特价/官方h3-2k', billingMode), platform: 'video' }
    video.pricing!.per_request_price = 0.25
    video.time_pricing = { timezone: 'Asia/Shanghai', periods: [{ start_time: '00:00', end_time: '23:59', multiplier: 4 }] }
    const wrapper = mountTable([video], { platform: 'video', rateMultiplier: 3, userRateMultiplier: 7, videoRateIndependent: true, videoRateMultiplier: 5 })
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    const prices = wrapper.get('tbody tr').findAll('.customer-price').map(cell => cell.text())
    expect(prices).toEqual(billingMode === 'video' ? ['$0.25', '$1.25', '$2.50', '$3.75'] : ['$0.25'])
    expect(wrapper.text()).not.toContain('modelPlaza.table.personalPrice')
    expect(wrapper.text()).toContain('videoModule.customerPriceHint')
    expect(wrapper.text()).not.toContain('modelPlaza.table.actualPriceNote')
  })

  it('shows reference and effective token prices and keeps newer model names first', () => {
    const wrapper = mountTable([model('z-model'), model('a-model')])
    const rows = wrapper.findAll('tbody tr')

    expect(rows.map((row) => row.get('.model-identity > span').text())).toEqual(['z-model', 'a-model'])
    expect(rows[0].findAll('.reference-price').map((cell) => cell.text())).toEqual([
      'modelPlaza.table.officialShort ¥7.00',
      'modelPlaza.table.officialShort ¥14.00',
      'modelPlaza.table.officialShort ¥3.50',
      'modelPlaza.table.officialShort ¥1.40'
    ])
    expect(rows[0].findAll('.customer-price').map((cell) => cell.text())).toEqual([
      '¥3.50', '¥7.00', '¥1.75', '¥0.70'
    ])
    expect(rows[0].text()).toContain('modelPlaza.table.savings')
    expect(wrapper.text()).toContain('modelPlaza.table.actualPriceNote')
    expect(wrapper.text()).toContain('modelPlaza.table.officialPrice')
    expect(wrapper.text()).toContain('modelPlaza.table.paidPrice')
  })

  it('uses the personal text rate when configured', () => {
    const wrapper = mountTable([model('text-model')], { userRateMultiplier: 0.8 })
    expect(wrapper.get('tbody tr').text()).toContain('¥5.60')
    expect(wrapper.get('tbody tr').text()).toContain('¥11.20')
    expect(wrapper.text()).toContain('modelPlaza.table.personalPrice')
    expect(wrapper.text()).not.toContain('modelPlaza.table.paidPrice')
  })

  it('shows valid reasoning billing tiers without changing base prices', () => {
    const item = model('claude-fable-5-1')
    item.pricing!.reasoning_effort_multipliers = { max: 3, high: 1.5, low: 0, medium: Number.NaN }

    const wrapper = mountTable([item])

    expect(wrapper.text()).toContain('modelPlaza.table.reasoningMultiplierBadge')
    expect(wrapper.findAll('[data-reasoning-effort]').map(b => b.attributes('data-reasoning-effort'))).toEqual(['high', 'max'])
    expect(wrapper.get('tbody tr').text()).toContain('¥7.00')
  })

  it('sorts token models by official output price before falling back to model name', () => {
    const expensive = model('model-expensive')
    expensive.official_pricing = {
      input_price: 10e-6,
      output_price: 75e-6,
      cache_write_price: null,
      cache_write_1h_price: null,
      cache_read_price: null
    }
    const cheap = model('model-cheap')
    cheap.official_pricing = {
      input_price: 1e-6,
      output_price: 5e-6,
      cache_write_price: null,
      cache_write_1h_price: null,
      cache_read_price: null
    }
    const noOfficial = model('model-no-official')

    const wrapper = mountTable([cheap, noOfficial, expensive])
    expect(wrapper.findAll('tbody tr').map((row) => row.get('.model-identity > span').text())).toEqual([
      'model-expensive',
      'model-cheap',
      'model-no-official'
    ])
  })

  it('shows the 1h cache-write price when the pricing catalog provides it', () => {
    const cached = model('cached-model')
    cached.pricing!.cache_write_1h_price = 7e-6
    const wrapper = mountTable([cached])
    const headers = wrapper.findAll('thead th').map((cell) => cell.text())
    const prices = wrapper.get('tbody tr').findAll('.customer-price').map((cell) => cell.text())

    expect(headers).toContain('modelPlaza.table.cacheWrite (1h)modelPlaza.table.cnyPerMillion')
    expect(prices).toContain('¥24.50')
  })

  it('uses matching official long-context tiers and labels marginal billing', () => {
    const longContext = model('long-context-model')
    longContext.pricing!.intervals = [
      { min_tokens: 200000, max_tokens: null, tier_label: '', input_price: 3e-6, output_price: 4e-6, cache_write_price: 2e-6, cache_read_price: 1e-6, per_request_price: null },
      { min_tokens: 0, max_tokens: 200000, tier_label: '', input_price: 1e-6, output_price: 2e-6, cache_write_price: 5e-7, cache_read_price: 2e-7, per_request_price: null }
    ]
    longContext.official_pricing = {
      input_price: 1e-6,
      output_price: 2e-6,
      cache_write_price: 5e-7,
      cache_read_price: 2e-7,
      intervals: [
        { min_tokens: 0, max_tokens: 200000, tier_label: '', input_price: 2e-6, output_price: 6e-6, cache_write_price: 1e-6, cache_read_price: 5e-7, per_request_price: null },
        { min_tokens: 200000, max_tokens: null, tier_label: '', input_price: 4e-6, output_price: 8e-6, cache_write_price: 2e-6, cache_read_price: 1e-6, per_request_price: null }
      ]
    }
    longContext.long_context_basis = 'marginal'

    const rows = mountTable([longContext]).findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows.map((row) => row.get('.tier-value').text())).toEqual(['<=200K', '>200K'])
    expect(rows[1].text()).toContain('modelPlaza.table.officialShort ¥56.00')
    expect(rows[1].find('.context-basis-badge').attributes('title')).toBe('modelPlaza.table.tierHintMarginal')
  })

  it('resolves long-context multiplier-only prices from the base token prices', () => {
    const longContext = model('multiplier-only-model')
    longContext.pricing = {
      billing_mode: 'token',
      input_price: 10e-6,
      output_price: 50e-6,
      cache_write_price: 12.5e-6,
      cache_write_1h_price: 12.5e-6,
      cache_read_price: 2e-6,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: [{
        min_tokens: 272000,
        max_tokens: null,
        tier_label: '>272K',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null,
        input_multiplier: 2,
        output_multiplier: 1.5,
        cache_write_multiplier: 2,
        cache_read_multiplier: 2,
        per_request_price: null
      }]
    }

    const prices = mountTable([longContext]).get('tbody tr')
      .findAll('.customer-price')
      .map((cell) => cell.text())

    expect(prices).toEqual(['¥70.00', '¥262.50', '¥87.50', '¥87.50', '¥14.00'])
  })

  it('adds a separately priced row for each time-pricing period without applying the peak multiplier', () => {
    const timed = model('timed-model')
    timed.time_pricing = {
      timezone: 'Asia/Shanghai',
      weekdays_only: true,
      periods: [{ start_time: '00:30', end_time: '08:30:00', multiplier: 0.4 }]
    }

    const rows = mountTable([timed], {
      rateMultiplier: 0.5,
      peakWindow: '14:00-18:00 x1.5',
      peakRateMultiplier: 1.5
    }).findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[1].find('.time-period-badge').text()).toContain('00:30-08:30')
    expect(rows[1].find('.time-period-badge').attributes('title')).toContain('modelPlaza.table.timePricingRowHintWeekdays')
    expect(rows[1].find('.time-period-badge').attributes('title')).toContain('modelPlaza.table.timePricingRowHintPeak')
    expect(rows[1].findAll('.customer-price').map((cell) => cell.text())).toContain('¥1.40')
  })

  it('renders image tiers with the independent image multiplier', () => {
    const image = model('grok-imagine', 'image')
    image.pricing!.intervals = [
      { min_tokens: 0, max_tokens: null, tier_label: '1K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.01 },
      { min_tokens: 0, max_tokens: null, tier_label: '2K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.02 }
    ]
    const wrapper = mountTable([image], { imageRateIndependent: true, imageRateMultiplier: 1.5 })

    expect(wrapper.text()).toContain('modelPlaza.table.imageModels')
    expect(wrapper.text()).toContain('1K')
    expect(wrapper.text()).toContain('¥0.105')
    expect(wrapper.text()).toContain('2K')
    expect(wrapper.text()).toContain('¥0.21')
  })

  it('does not multiply image tiers marked as final customer prices', () => {
    const image = model('grok-imagine-image-2.0', 'image')
    image.final_image_price_tiers = ['1K']
    image.pricing!.intervals = [
      { min_tokens: 0, max_tokens: null, tier_label: '1K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.01 },
      { min_tokens: 0, max_tokens: null, tier_label: '2K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.02 }
    ]

    const wrapper = mountTable([image], { imageRateIndependent: true, imageRateMultiplier: 0.15 })
    const rows = wrapper.findAll('tbody tr')

    expect(rows[0].text()).toContain('1K')
    expect(rows[0].text()).toContain('¥0.07')
    expect(rows[0].find('.reference-price').exists()).toBe(false)
    expect(rows[1].text()).toContain('2K')
    expect(rows[1].text()).toContain('¥0.021')
  })

  it('shows video per-second and common duration totals using the independent rate', () => {
    const video = model('grok-imagine-video-1.5', 'video')
    video.pricing!.intervals = [
      { min_tokens: 0, max_tokens: null, tier_label: '1080p', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.25 }
    ]
    const wrapper = mountTable([video], { videoRateIndependent: true, videoRateMultiplier: 1.5 })
    const row = wrapper.get('tbody tr').text()

    expect(wrapper.text()).toContain('modelPlaza.table.videoModels')
    expect(row).toContain('¥2.625')
    expect(row).toContain('¥13.125')
    expect(row).toContain('¥26.25')
    expect(row).toContain('¥39.375')
  })

  it('shows final video per-second prices with duration but without multiplier stacking', () => {
    const video = model('grok-imagine-video-1.5', 'video')
    video.final_video_price_tiers = ['720p']
    video.pricing!.intervals = [
      { min_tokens: 0, max_tokens: null, tier_label: '720p', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.01 }
    ]
    const wrapper = mountTable([video], { videoRateIndependent: true, videoRateMultiplier: 0.15 })
    const row = wrapper.get('tbody tr')

    expect(row.text()).toContain('modelPlaza.table.finalPerSecond')
    expect(row.findAll('.customer-price').map((cell) => cell.text())).toEqual([
      '¥0.07',
      '¥0.35',
      '¥0.70',
      '¥1.05'
    ])
    expect(row.text()).not.toContain('¥0.011')
  })

  it('keeps per-request models in their own section', () => {
    const request = model('search-tool', 'per_request')
    request.pricing!.per_request_price = 0.04
    const wrapper = mountTable([request])

    expect(wrapper.text()).toContain('modelPlaza.table.requestModels')
    expect(wrapper.get('tbody tr').text()).toContain('¥0.14')
  })

  it('keeps same-name models distinguished by platform in composite groups', () => {
    const anthropic = { ...model('shared-model'), platform: 'anthropic' }
    const openai = { ...model('shared-model'), platform: 'openai' }
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [anthropic, openai],
        platform: 'composite',
        rateMultiplier: 1
      }
    })

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows.map((row) => row.find('td').text())).toEqual([
      'shared-modelAnthropic',
      'shared-modelOpenAI'
    ])
    expect(wrapper.text()).toContain('Anthropic')
    expect(wrapper.text()).toContain('OpenAI')
  })

  it('hides the tier column when every row uses the default tier', () => {
    const wrapper = mountTable([model('plain-model')])

    expect(wrapper.findAll('thead th').map((cell) => cell.text())).not.toContain('modelPlaza.table.tier')
  })

  it('keeps the savings value in the final table column', () => {
    const wrapper = mountTable([model('discounted-model')])
    const headers = wrapper.findAll('thead th')
    const cells = wrapper.get('tbody tr').findAll('td')

    expect(headers.at(-1)?.text()).toBe('modelPlaza.table.savingsColumn')
    expect(cells.at(-1)?.find('.savings-badge').exists()).toBe(true)
  })
})
