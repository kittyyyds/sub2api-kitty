import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import SupportedModelChip from '../SupportedModelChip.vue'
import type { UserAvailableGroup, UserSupportedModel } from '@/api/channels'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const PricingRowStub = {
  props: ['label', 'value', 'unit', 'scale'],
  template: '<div data-pricing-row>{{ label }}|{{ value }}|{{ unit }}|{{ scale }}</div>',
}

function mountChip(model: UserSupportedModel, pricingGroups: UserAvailableGroup[] = []) {
  return mount(SupportedModelChip, {
    props: { model, pricingGroups, pricingKeyPrefix: 'availableChannels.pricing' },
    global: {
      stubs: {
        Teleport: true,
        PlatformIcon: true,
        PricingRow: PricingRowStub,
      },
    },
  })
}

function pricingGroup(overrides: Partial<UserAvailableGroup> = {}): UserAvailableGroup {
  return {
    id: 24,
    name: 'Grok media',
    platform: 'grok',
    subscription_type: 'standard',
    rate_multiplier: 0.65,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    image_rate_independent: true,
    image_rate_multiplier: 1.5,
    video_rate_independent: true,
    video_rate_multiplier: 1.5,
    ...overrides,
  }
}

function mediaModel(overrides: Partial<NonNullable<UserSupportedModel['pricing']>>): UserSupportedModel {
  return {
    name: 'grok-imagine',
    platform: 'grok',
    pricing: {
      billing_mode: 'video',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: [],
      ...overrides,
    },
  }
}

describe('SupportedModelChip media pricing', () => {
  it.each(['video', 'per_request'] as const)('keeps video-platform %s prices final despite group and personal multipliers', async (billingMode) => {
    const model = { ...mediaModel({ billing_mode: billingMode, per_request_price: 0.25 }), platform: 'video' }
    const wrapper = mountChip(model, [pricingGroup({ platform: 'video', rate_multiplier: 3, video_rate_multiplier: 5 })])
    await wrapper.setProps({ userGroupRates: { 24: 7 } })
    expect(wrapper.get('[data-pricing-row]').text()).toMatch(/\|0\.25\|.*\|1$/)
    expect(wrapper.text()).toContain('videoModule.finalPrice')
    expect(wrapper.text()).not.toContain('×7')
    expect(wrapper.text()).not.toContain('×5')
  })

  it('shows video billing as a per-second price, including resolution tiers', () => {
    const wrapper = mountChip(mediaModel({
      billing_mode: 'video',
      per_request_price: 0.08,
      intervals: [{
        min_tokens: 0,
        max_tokens: null,
        tier_label: '720p',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.05,
      }],
    }))

    const text = wrapper.text()
    expect(text).toContain('availableChannels.pricing.billingModeVideo')
    expect(text).toContain('availableChannels.pricing.perSecondPrice|0.08|availableChannels.pricing.unitPerSecond')
    expect(text).toContain('720p')
    expect(text).toContain('$0.05 availableChannels.pricing.unitPerSecond')
  })

  it('uses the configured per-image price instead of the image-token output field', () => {
    const wrapper = mountChip(mediaModel({
      billing_mode: 'image',
      per_request_price: 0.04,
      image_output_price: 0.00003,
    }))

    const text = wrapper.text()
    expect(text).toContain('availableChannels.pricing.billingModeImage')
    expect(text).toContain('availableChannels.pricing.perImagePrice|0.04|availableChannels.pricing.unitPerImage')
    expect(text).not.toContain('|0.00003|')
  })

  it('shows customer image prices using the independent media multiplier', () => {
    const wrapper = mountChip(mediaModel({
      billing_mode: 'image',
      per_request_price: 0.01,
      intervals: [{
        min_tokens: 0,
        max_tokens: null,
        tier_label: '2K',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: 0.007,
      }],
    }), [pricingGroup()])

    const text = wrapper.text()
    expect(text).toContain('Grok media')
    expect(text).toContain('availableChannels.pricing.actualPrice ×1.5')
    expect(text).toContain('availableChannels.pricing.perImagePrice|0.01|availableChannels.pricing.unitPerImage|1.5')
    expect(text).toContain('$0.0105 availableChannels.pricing.unitPerImage')
  })

  it('shows each group with its own effective video price', () => {
    const wrapper = mountChip(mediaModel({
      billing_mode: 'video',
      per_request_price: 0.25,
    }), [
      pricingGroup(),
      pricingGroup({ id: 25, name: 'Standard', video_rate_independent: false, rate_multiplier: 0.8 }),
    ])

    const rows = wrapper.findAll('[data-pricing-row]')
    expect(wrapper.text()).toContain('Grok media')
    expect(wrapper.text()).toContain('Standard')
    expect(rows.map((row) => row.text())).toEqual([
      'availableChannels.pricing.perSecondPrice|0.25|availableChannels.pricing.unitPerSecond|1.5',
      'availableChannels.pricing.perSecondPrice|0.25|availableChannels.pricing.unitPerSecond|0.8',
    ])
  })
})
