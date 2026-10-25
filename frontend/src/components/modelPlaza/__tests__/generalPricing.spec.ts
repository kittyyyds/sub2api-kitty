import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { generalPricingResponse } from '../generalPricing'
import CatalogViewSwitch from '../CatalogViewSwitch.vue'
import { fixture } from './mediaPricing.fixture'

describe('general versus media pricing', () => {
  it('removes images and non-Grok video without changing original catalog or prices', () => {
    const data = fixture()
    const base = data.groups[0]!.models[0]!
    data.groups[0]!.models = [
      { ...base, name: 'grok-imagine-video' },
      { ...base, name: 'sora-2' },
      { ...base, name: 'GROK-video-hd', platform: 'grok' },
    ]
    data.groups.push({ ...data.groups[0]!, id: 50, platform: 'openai', models: [
      { ...base, name: 'gpt-image-2', platform: 'openai', pricing: { ...base.pricing!, billing_mode: 'image' } },
      { ...base, name: 'gpt-6.1-sol', platform: 'openai', pricing: { ...base.pricing!, billing_mode: 'token' } },
    ] })
    const before = structuredClone(data)
    const result = generalPricingResponse(data)!
    expect(result.groups[0]!.models.map(model => model.name)).toEqual(['GROK-video-hd'])
    expect(result.groups[1]!.models.map(model => model.name)).toEqual(['gpt-6.1-sol'])
    expect(result.groups[0]!.models[0]!.pricing).toEqual(base.pricing)
    expect(data).toEqual(before)
  })
  it('handles missing data and removes empty video groups', () => {
    expect(generalPricingResponse(null)).toBeNull()
    expect(generalPricingResponse(fixture())!.groups).toEqual([])
  })
  it('provides two explicit accessible switching buttons', async () => {
    const wrapper = mount(CatalogViewSwitch, { props: { modelValue: 'all' } })
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    expect(buttons[0]!.attributes('aria-pressed')).toBe('true')
    expect(buttons[1]!.text()).toContain('点击查看')
    await buttons[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['media'])
    await wrapper.setProps({ modelValue: 'media' })
    expect(buttons[1]!.attributes('aria-pressed')).toBe('true')
    expect(buttons[1]!.text()).toContain('正在查看')
  })
})
