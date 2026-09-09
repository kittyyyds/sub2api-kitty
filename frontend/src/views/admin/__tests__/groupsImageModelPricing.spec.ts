import { describe, expect, it } from 'vitest'

import {
  createImageModelPricesForm,
  getImageModelPricePlaceholder,
  imageModelPriceRows,
  openAIImagePriceModels,
  resolveImageModelCustomerPrice,
  resolveImageModelPrice,
  serializeImageModelPrices,
  supportsImageModelPricingPlatform
} from '../groupsImageModelPricing'

describe('Grok image model pricing form', () => {
  it('provides independent rows for every supported Grok image model', () => {
    const form = createImageModelPricesForm()

    expect(imageModelPriceRows(form).map(({ key }) => key)).toEqual([
      'grok-imagine',
      'grok-imagine-image',
      'grok-imagine-image-2.0',
      'grok-imagine-image-quality'
    ])
    expect(form['grok-imagine']['1K']).toBeNull()
    expect(form['grok-imagine-image-quality']['4K']).toBeNull()
  })

  it('serializes only finite non-negative prices and preserves zero', () => {
    const form = createImageModelPricesForm()
    form['grok-imagine']['1K'] = 0
    form['grok-imagine-image']['2K'] = 0.03
    form['grok-imagine-image-quality']['4K'] = -1

    expect(serializeImageModelPrices(form)).toEqual({
      'grok-imagine': { '1K': 0 },
      'grok-imagine-image': { '2K': 0.03 }
    })
  })

  it('round-trips future model rows without merging model IDs', () => {
    const form = createImageModelPricesForm({
      'grok-imagine-image-next': { '4k': 0.12 }
    })

    expect(imageModelPriceRows(form).map(({ key }) => key)).toContain(
      'grok-imagine-image-next'
    )
    expect(serializeImageModelPrices(form)).toMatchObject({
      'grok-imagine-image-next': { '4K': 0.12 }
    })
  })

  it('shows model-aware default price placeholders', () => {
    expect(getImageModelPricePlaceholder('grok-imagine-image', '2K')).toBe('0.02')
    expect(getImageModelPricePlaceholder('grok-imagine-image-quality', '2K')).toBe('0.07')
    expect(getImageModelPricePlaceholder('grok-imagine-image-2.0', '1K')).toBe('0.06')
  })

  it('resolves final preview prices in billing priority order', () => {
    const form = createImageModelPricesForm()
    form['grok-imagine-image-2.0']['1K'] = 0.01
    form['grok-imagine-image-quality']['1K'] = 0

    expect(resolveImageModelPrice(form, 'grok-imagine-image-2.0', '1K', 0.02)).toBe(0.01)
    expect(resolveImageModelPrice(form, 'grok-imagine-image-quality', '1K', 0.02)).toBe(0)
    expect(resolveImageModelPrice(form, 'grok-imagine-image', '1K', 0.03)).toBe(0.03)
    expect(resolveImageModelPrice(form, 'grok-imagine-image-2.0', '2K', null)).toBe(0.08)
  })

  it('treats configured model cells as final customer prices', () => {
    const form = createImageModelPricesForm()
    form['grok-imagine-image-2.0']['1K'] = 0.01
    form['grok-imagine-image-quality']['1K'] = 0

    expect(resolveImageModelCustomerPrice(
      form,
      'grok-imagine-image-2.0',
      '1K',
      0.02,
      0.15
    )).toBe(0.01)
    expect(resolveImageModelCustomerPrice(
      form,
      'grok-imagine-image-quality',
      '1K',
      0.02,
      0.15
    )).toBe(0)
  })

  it('still applies the image multiplier to inherited prices', () => {
    const form = createImageModelPricesForm()

    expect(resolveImageModelCustomerPrice(
      form,
      'grok-imagine-image',
      '1K',
      0.02,
      0.15
    )).toBeCloseTo(0.003)
    expect(resolveImageModelCustomerPrice(
      form,
      'grok-imagine-image-2.0',
      '2K',
      null,
      0.15
    )).toBeCloseTo(0.012)
  })

  it('provides independent rows for every supported OpenAI image model', () => {
    const form = createImageModelPricesForm()

    expect(imageModelPriceRows(form, 'openai').map(({ key }) => key)).toEqual(
      openAIImagePriceModels.map(({ key }) => key)
    )
    expect(form['gpt-image-1']['1K']).toBeNull()
    expect(form['gpt-image-1.5']['2K']).toBeNull()
    expect(form['gpt-image-2']['4K']).toBeNull()
    expect(supportsImageModelPricingPlatform('openai')).toBe(true)
  })

  it('keeps OpenAI model cells as final customer prices', () => {
    const form = createImageModelPricesForm()
    form['gpt-image-1']['1K'] = 0.011
    form['gpt-image-1.5']['2K'] = 0.022
    form['gpt-image-2']['4K'] = 0.01

    expect(serializeImageModelPrices(form)).toMatchObject({
      'gpt-image-1': { '1K': 0.011 },
      'gpt-image-1.5': { '2K': 0.022 },
      'gpt-image-2': { '4K': 0.01 }
    })
    expect(resolveImageModelCustomerPrice(
      form,
      'gpt-image-2',
      '4K',
      0.268,
      0.15
    )).toBe(0.01)
  })

  it('uses the platform default only when no model or flat price is configured', () => {
    const form = createImageModelPricesForm()

    expect(resolveImageModelCustomerPrice(
      form,
      'gpt-image-2',
      '2K',
      null,
      0.5,
      0.201
    )).toBeCloseTo(0.1005)
  })

  it('exposes Image 2.5 tiers without creating prices or inheriting Image 2 overrides', () => {
    const form = createImageModelPricesForm({ 'gpt-image-2': { '1K': 0.02 } })
    const models = ['gpt-image-2.5', 'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst']
    const rows = imageModelPriceRows(form, 'openai').map(({ key }) => key)

    for (const model of models) {
      expect(rows.filter((key) => key === model)).toHaveLength(1)
      expect(form[model]).toEqual({ '1K': null, '2K': null, '4K': null })
      expect(resolveImageModelCustomerPrice(form, model, '1K', 0.1, 0.5)).toBe(0.05)
    }
    expect(serializeImageModelPrices(form)).toEqual({ 'gpt-image-2': { '1K': 0.02 } })
    expect(imageModelPriceRows(form, 'grok').map(({ key }) => key)).not.toEqual(
      expect.arrayContaining(models)
    )
  })

  it('keeps Image 2.5 variant prices independent after save and reopen', () => {
    const form = createImageModelPricesForm({ 'gpt-image-2': { '1K': 0.02 } })
    form['gpt-image-2.5']['1K'] = '0.03'
    form['gpt-image-2.5-flare']['1K'] = '0.04'
    form['gpt-image-2.5-sunburst']['1K'] = '0.05'
    form['gpt-image-2.5']['2K'] = '0.08'
    form['gpt-image-2.5-flare']['4K'] = '0'
    form['gpt-image-2.5-sunburst']['4K'] = ''

    const saved = JSON.parse(JSON.stringify(serializeImageModelPrices(form)))
    expect(saved).toEqual({
      'gpt-image-2': { '1K': 0.02 },
      'gpt-image-2.5': { '1K': 0.03, '2K': 0.08 },
      'gpt-image-2.5-flare': { '1K': 0.04, '4K': 0 },
      'gpt-image-2.5-sunburst': { '1K': 0.05 }
    })
    const reopened = createImageModelPricesForm(saved)
    for (const [model, tiers] of Object.entries(saved)) {
      for (const [tier, price] of Object.entries(tiers as Record<string, number>)) {
        expect(resolveImageModelCustomerPrice(reopened, model, tier, 1, 3)).toBe(price)
      }
    }
    expect(reopened['gpt-image-2.5-sunburst']['4K']).toBeNull()
  })
})
