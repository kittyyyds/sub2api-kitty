import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import type { PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function tokenModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'claude-sonnet',
    platform: 'anthropic',
    pricing: {
      billing_mode: 'token',
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_write_1h_price: null,
      cache_read_price: 3e-7,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_write_1h_price: 6e-6,
      cache_read_price: 3e-7
    },
    ...overrides
  }
}

function model(name: string, billingMode: 'token' | 'image' | 'video' | 'per_request' = 'token'): PlazaModel {
  return tokenModel({
    name,
    platform: 'grok',
    pricing: {
      billing_mode: billingMode,
      input_price: billingMode === 'token' ? 1e-6 : null,
      output_price: billingMode === 'token' ? 2e-6 : null,
      cache_write_price: billingMode === 'token' ? 5e-7 : null,
      cache_write_1h_price: null,
      cache_read_price: billingMode === 'token' ? 2e-7 : null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: null
  })
}

function mountTable(models: PlazaModel[], props: Record<string, unknown> = {}) {
  return mount(PlazaModelPricingTable, {
    props: { models, rateMultiplier: 0.5, userRateMultiplier: null, ...props }
  })
}

describe('PlazaModelPricingTable customer pricing ledger', () => {
  it('shows reference and effective token prices in RMB and keeps newer model names first', () => {
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

  it('shows the max reasoning billing multiplier', () => {
    const item = model('claude-fable-5-1')
    item.pricing!.max_reasoning_effort_multiplier = 3

    const wrapper = mountTable([item])

    expect(wrapper.text()).toContain('modelPlaza.table.maxReasoningMultiplierBadge')
    expect(wrapper.find('[title="modelPlaza.table.maxReasoningMultiplierHint"]').exists()).toBe(true)
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

  it('shows platform badges for composite groups', () => {
    const anthropic = tokenModel({ name: 'shared-model', platform: 'anthropic' })
    const openai = tokenModel({ name: 'shared-model', platform: 'openai' })
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

describe('PlazaModelPricingTable long-context tiers', () => {
  function ladderIntervals() {
    return [
      {
        min_tokens: 0,
        max_tokens: 272000,
        tier_label: '≤272K',
        input_price: 5e-6,
        output_price: 3e-5,
        cache_write_price: 6.25e-6,
        cache_write_1h_price: null,
        cache_read_price: 5e-7,
        per_request_price: null
      },
      {
        min_tokens: 272000,
        max_tokens: null,
        tier_label: '>272K',
        input_price: 1e-5,
        output_price: 4.5e-5,
        cache_write_price: 1.25e-5,
        cache_write_1h_price: null,
        cache_read_price: 1e-6,
        per_request_price: null
      }
    ]
  }

  function ladderModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
    return tokenModel({
      name: 'gpt-5.6-sol',
      platform: 'openai',
      pricing: {
        billing_mode: 'token',
        input_price: 5e-6,
        output_price: 3e-5,
        cache_write_price: 6.25e-6,
        cache_write_1h_price: null,
        cache_read_price: 5e-7,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: ladderIntervals()
      },
      official_pricing: {
        input_price: 5e-6,
        output_price: 3e-5,
        cache_write_price: 6.25e-6,
        cache_read_price: 5e-7,
        intervals: ladderIntervals()
      },
      long_context_basis: 'whole_request',
      ...overrides
    })
  }

  it('renders tiered RMB rows and keeps tier labels on the tier column', () => {
    const wrapper = mountTable([ladderModel()])
    const rows = wrapper.findAll('tbody tr')

    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('≤272K')
    expect(rows[0].text()).toContain('¥17.50')
    expect(rows[0].text()).toContain('¥105.00')
    expect(rows[1].text()).toContain('>272K')
    expect(rows[1].text()).toContain('¥35.00')
    expect(rows[1].text()).toContain('¥157.50')
  })

  it('shows 1h cache write prices when present on flat pricing', () => {
    const m = tokenModel()
    m.pricing!.cache_write_1h_price = 7e-6

    const wrapper = mountTable([m], { rateMultiplier: 1 })
    expect(wrapper.text()).toContain('1h')
    expect(wrapper.text()).toContain('¥49.00')
    expect(wrapper.text()).toContain('¥42.00')
  })

  it('adds marginal long-context badges and tooltip hints', () => {
    const wrapper = mountTable([ladderModel({ long_context_basis: 'marginal' })], { rateMultiplier: 1 })

    expect(wrapper.find('.context-badge').text()).toBe('modelPlaza.table.marginalBadge')
    expect(wrapper.find('.context-badge').attributes('title')).toBe('modelPlaza.table.tierHintMarginal')
    expect(wrapper.find('.tier-value span').attributes('title')).toBe('modelPlaza.table.tierHintMarginal')
  })

  it('generates tier labels from unordered interval bounds', () => {
    const m = ladderModel({
      pricing: {
        ...ladderModel().pricing!,
        intervals: [
          { ...ladderIntervals()[1], min_tokens: 1000000, tier_label: '' },
          { ...ladderIntervals()[0], min_tokens: 100000, max_tokens: 200000, tier_label: '' },
          { ...ladderIntervals()[0], max_tokens: 100000, tier_label: '' },
          { ...ladderIntervals()[1], min_tokens: 200000, max_tokens: 1000000, tier_label: '' }
        ]
      }
    })

    const tiers = mountTable([m], { rateMultiplier: 1 })
      .findAll('tbody tr')
      .map((row) => row.find('.tier-value').text())
    expect(tiers).toEqual(['≤100K', '≤200K', '≤1M', '>1M'])
  })
})

describe('PlazaModelPricingTable time pricing', () => {
  function timePricedModel() {
    return tokenModel({
      name: 'deepseek-chat',
      platform: 'deepseek',
      time_pricing: {
        timezone: 'Asia/Shanghai',
        periods: [
          { start_time: '00:30', end_time: '08:30:00', multiplier: 0.5 },
          { start_time: '18:00', end_time: '22:00', multiplier: 1.2 }
        ]
      }
    })
  }

  it('expands time-priced token models into base and period rows', () => {
    const wrapper = mountTable([timePricedModel()], { rateMultiplier: 0.8 })
    const trs = wrapper.findAll('tbody tr')
    expect(trs).toHaveLength(3)

    expect(trs[0].text()).toContain('¥16.80')
    expect(trs[1].text()).toContain('00:30–08:30')
    expect(trs[1].text()).toContain('¥8.40')
    expect(trs[2].text()).toContain('18:00–22:00')
    expect(trs[2].text()).toContain('¥20.16')
  })

  it('uses weekday copy and peak disclosure in period tooltips', () => {
    const m = timePricedModel()
    m.time_pricing!.weekdays_only = true
    const wrapper = mountTable([m], {
      peakWindow: '14:00-18:00 ×1.5 (UTC+08:00)',
      peakRateMultiplier: 1.5
    })
    const badge = wrapper.find('.period-badge')

    expect(badge.text()).toContain('modelPlaza.table.timePricingWeekdays')
    expect(badge.attributes('title')).toContain('modelPlaza.table.timePricingRowHintWeekdays')
    expect(badge.attributes('title')).toContain('modelPlaza.table.timePricingRowHintPeak')
  })
})
