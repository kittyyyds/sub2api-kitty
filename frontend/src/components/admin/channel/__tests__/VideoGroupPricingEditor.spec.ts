import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import VideoGroupPricingEditor from '../VideoGroupPricingEditor.vue'
import { newVideoGroupPricing, validateVideoGroupPricing } from '../videoGroupPricing'
import type { PricingFormEntry } from '../types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('video group customer prices', () => {
  it('edits exact custom IDs and both supported billing modes without token or tier controls', async () => {
    const wrapper = mount(VideoGroupPricingEditor, { props: { modelValue: [] } })
    await wrapper.get('[data-testid="video-pricing-add"]').trigger('click')
    const update = async () => wrapper.setProps({ modelValue: wrapper.emitted('update:modelValue')!.at(-1)![0] as PricingFormEntry[] })
    await update()
    expect(wrapper.findAll('option').map(item => item.attributes('value'))).toEqual(['per_request', 'video'])
    await wrapper.get('[data-testid="video-pricing-model"]').setValue('特价/官方h3-2k')
    await update()
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('0.25')
    await update()
    expect(validateVideoGroupPricing(wrapper.props('modelValue'))).toBeNull()
    await wrapper.get('[data-testid="video-pricing-billing"]').setValue('per_request')
    await update()
    expect(wrapper.props('modelValue')[0]).toMatchObject({ models: ['特价/官方h3-2k'], billing_mode: 'per_request', per_request_price: '0.25', intervals: [] })
    expect(wrapper.find('option[value="token"]').exists()).toBe(false)
    expect(wrapper.find('option[value="image"]').exists()).toBe(false)
  })

  it('requires complete exact prices while allowing zero and case-sensitive IDs', () => {
    const entry = { ...newVideoGroupPricing(), models: ['Video-A'], per_request_price: 0 }
    expect(validateVideoGroupPricing([])).toBe('videoModule.emptyPricing')
    expect(validateVideoGroupPricing([entry])).toBeNull()
    expect(validateVideoGroupPricing([entry, { ...entry, models: ['video-a'] }])).toBeNull()
    expect(validateVideoGroupPricing([entry, entry])).toBe('videoModule.duplicateModel')
    expect(validateVideoGroupPricing([{ ...entry, models: ['Video-*'] }])).toBe('videoModule.invalidModel')
    expect(validateVideoGroupPricing([{ ...entry, models: ['Video-A', 'Video-B'] }])).toBe('videoModule.invalidModel')
    expect(validateVideoGroupPricing([{ ...entry, billing_mode: 'token' }])).toBe('videoModule.invalidBilling')
    for (const price of [null, '', -1, NaN, Infinity]) {
      expect(validateVideoGroupPricing([{ ...entry, per_request_price: price }])).toBe('videoModule.invalidPrice')
    }
  })
})
