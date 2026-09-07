import { describe, expect, it } from 'vitest'

import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import {
  buildCatalogSections,
  humanizeModelName,
  normalizeModelId,
  visibleCatalogGroups,
} from '../modelCatalog'

function tokenModel(name: string, platform = 'openai'): PlazaModel {
  return {
    name,
    platform,
    pricing: {
      billing_mode: 'token',
      input_price: 5e-6,
      output_price: 30e-6,
      cache_write_price: 6.25e-6,
      cache_write_1h_price: null,
      cache_read_price: 0.5e-6,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 5e-6,
      output_price: 30e-6,
      cache_write_price: 6.25e-6,
      cache_read_price: 0.5e-6
    }
  }
}

function group(
  id: number,
  name: string,
  rate: number,
  models: PlazaModel[],
  platform = 'openai'
): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform,
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

describe('model catalog calculations', () => {
  it('normalizes provider model paths and creates readable names', () => {
    expect(normalizeModelId('publishers/openai/models/gpt-5.6-sol')).toBe('gpt-5.6-sol')
    expect(humanizeModelName('gpt-5.6-sol')).toBe('GPT 5.6 Sol')
  })

  it('selects the lowest effective token multiplier and calculates savings', () => {
    const expensive = group(1, 'Standard', 0.8, [tokenModel('gpt-5.6')])
    const personal = group(2, 'Personal', 0.4, [tokenModel('gpt-5.6')])
    personal.user_rate_multiplier = 0.1

    const card = buildCatalogSections([expensive, personal], null)[0].cards[0]
    expect(card.groupName).toBe('Personal')
    expect(card.multiplier).toBe(0.1)
    expect(card.metrics.find((metric) => metric.label === 'output')).toMatchObject({
      current: 3e-6,
      official: 30e-6,
      saving: 90
    })
  })

  it('uses only models from the selected group', () => {
    const first = group(1, 'First', 0.8, [tokenModel('gpt-first')])
    const second = group(2, 'Second', 0.2, [tokenModel('gpt-second')])

    const cards = buildCatalogSections([first, second], 1).flatMap((section) => section.cards)
    expect(cards.map((card) => card.modelId)).toEqual(['gpt-first'])
  })

  it('does not multiply per-model final image prices again', () => {
    const image: PlazaModel = {
      name: 'gpt-image-2',
      platform: 'openai',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [{
          min_tokens: 0,
          max_tokens: null,
          tier_label: '1K',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_write_1h_price: null,
          cache_read_price: null,
          per_request_price: 0.02
        }]
      },
      official_pricing: null,
      final_image_price_tiers: ['1K']
    }
    const imageGroup = group(1, 'Image', 0.05, [image])
    const card = buildCatalogSections([imageGroup], null)[0].cards[0]

    expect(card.kind).toBe('image')
    expect(card.metrics[0].current).toBe(0.02)
  })

  it('excludes the reserved default group from customer choices', () => {
    const groups = [
      group(1, 'default', 1, [tokenModel('hidden')]),
      group(2, 'Customer', 0.5, [tokenModel('visible')])
    ]

    expect(visibleCatalogGroups(groups).map((item) => item.name)).toEqual(['Customer'])
    const cards = buildCatalogSections(groups, null).flatMap((section) => section.cards)
    expect(cards.map((card) => card.modelId)).toEqual(['visible'])
  })
})

