import { describe, expect, it } from 'vitest'

import {
  createVideoModelPricesForm,
  getConfiguredVideoModelPrice,
  resolveVideoModelCustomerPrice,
  serializeVideoModelPrices,
  videoModelPriceFamilyRows
} from '../groupsVideoModelPricing'

describe('Grok video model pricing form', () => {
  it('provides editable rows for both canonical Grok video families', () => {
    const form = createVideoModelPricesForm()

    expect(videoModelPriceFamilyRows(form).map(({ key }) => key)).toEqual([
      'grok-imagine-video',
      'grok-imagine-video-1.5'
    ])
    expect(form['grok-imagine-video']['480p']).toBeNull()
    expect(form['grok-imagine-video-1.5']['1080p']).toBeNull()
  })

  it('serializes only finite non-negative prices and preserves future families', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-2': { '1080p': 0.4 }
    })
    form['grok-imagine-video']['480p'] = 0.05
    form['grok-imagine-video']['720p'] = ''
    form['grok-imagine-video-1.5']['1080p'] = -1

    expect(serializeVideoModelPrices(form)).toEqual({
      'grok-imagine-video': { '480p': 0.05 },
      'grok-imagine-video-2': { '1080p': 0.4 }
    })
  })

  it('round-trips unknown model families so editing does not discard them', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-2': { '480p': 0.2 }
    })

    expect(videoModelPriceFamilyRows(form).map(({ key }) => key)).toContain(
      'grok-imagine-video-2'
    )
    expect(serializeVideoModelPrices(form)).toMatchObject({
      'grok-imagine-video-2': { '480p': 0.2 }
    })
  })

  it('treats a populated model tier as the exact final per-second price', () => {
    const form = createVideoModelPricesForm({
      'grok-imagine-video-1.5': { '720p': 0.01 }
    })

    expect(getConfiguredVideoModelPrice(form, 'grok-imagine-video-1.5', '720p')).toBe(0.01)
    expect(resolveVideoModelCustomerPrice(
      form,
      'grok-imagine-video-1.5',
      '720p',
      0.5,
      0.15,
      0.14
    )).toBe(0.01)
  })

  it('uses flat/default per-second pricing and the multiplier only for empty tiers', () => {
    const form = createVideoModelPricesForm()

    expect(resolveVideoModelCustomerPrice(
      form,
      'grok-imagine-video-1.5',
      '720p',
      0.2,
      0.5,
      0.07
    )).toBeCloseTo(0.7)
    expect(resolveVideoModelCustomerPrice(
      form,
      'grok-imagine-video-1.5',
      '1080p',
      null,
      0.5,
      0.07
    )).toBeCloseTo(0.875)
  })
})
