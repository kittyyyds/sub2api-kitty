import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick, shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getModelPlaza, type ModelPlazaGroup } from '@/api/modelPlaza'
import { buildHomePricingSummaries, useHomePricingPreview } from '../useHomePricingPreview'

vi.mock('@/api/modelPlaza', () => ({
  getModelPlaza: vi.fn(),
}))

const getModelPlazaMock = vi.mocked(getModelPlaza)

beforeEach(() => {
  getModelPlazaMock.mockReset()
})

function group(overrides: Partial<ModelPlazaGroup>): ModelPlazaGroup {
  return {
    id: 1,
    name: 'Group',
    description: '',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: 1,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    models: [],
    ...overrides,
  }
}

describe('buildHomePricingSummaries', () => {
  it('selects the lowest effective rate for each public platform', () => {
    const summaries = buildHomePricingSummaries([
      group({ id: 1, name: 'OpenAI stable', rate_multiplier: 0.2 }),
      group({ id: 2, name: 'OpenAI welfare', rate_multiplier: 0.08 }),
      group({ id: 3, platform: 'anthropic', name: 'Claude', rate_multiplier: 0.65 }),
      group({ id: 4, platform: 'anthropic', name: 'Kiro', rate_multiplier: 0.1 }),
      group({ id: 5, platform: 'grok', name: 'Grok', rate_multiplier: 0.15 }),
    ])

    expect(summaries.map((summary) => [summary.platform, summary.groupName, summary.rateMultiplier])).toEqual([
      ['openai', 'OpenAI welfare', 0.08],
      ['anthropic', 'Kiro', 0.1],
      ['grok', 'Grok', 0.15],
    ])
  })

  it('uses a user-specific rate and reports media billing modes', () => {
    const summaries = buildHomePricingSummaries([
      group({
        platform: 'grok',
        rate_multiplier: 0.2,
        user_rate_multiplier: 0.12,
        image_rate_independent: true,
        image_rate_multiplier: 1.5,
        video_rate_independent: true,
        video_rate_multiplier: 1.6,
        models: [
          { name: 'grok-4.5', platform: 'grok', pricing: null, official_pricing: null },
          {
            name: 'grok-imagine',
            platform: 'grok',
            pricing: {
              billing_mode: 'image',
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              image_input_price: null,
              image_output_price: null,
              per_request_price: 0.01,
              intervals: [],
            },
            official_pricing: null,
          },
          {
            name: 'grok-imagine-video',
            platform: 'grok',
            pricing: {
              billing_mode: 'video',
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              image_input_price: null,
              image_output_price: null,
              per_request_price: 0.05,
              intervals: [],
            },
            official_pricing: null,
          },
        ],
      }),
    ])

    expect(summaries[0]).toMatchObject({
      platform: 'grok',
      rateMultiplier: 0.12,
      subscriptionType: 'standard',
      featuredModel: 'grok-4.5',
      imageRateMultiplier: 1.5,
      videoRateMultiplier: 1.6,
    })
  })

  it('loads when a runtime setting enables the public catalog', async () => {
    const enabled = shallowRef(false)
    getModelPlazaMock.mockResolvedValue({
      description: '',
      groups: [group({ name: 'Runtime group', rate_multiplier: 0.08 })],
    })

    let preview: ReturnType<typeof useHomePricingPreview> | undefined
    const Host = defineComponent({
      setup() {
        preview = useHomePricingPreview({ enabled })
        return () => null
      },
    })
    const wrapper = mount(Host)

    await flushPromises()
    expect(getModelPlazaMock).not.toHaveBeenCalled()

    enabled.value = true
    await nextTick()
    await flushPromises()

    expect(getModelPlazaMock).toHaveBeenCalledTimes(1)
    expect(preview?.summaries.value[0]).toMatchObject({
      groupName: 'Runtime group',
      rateMultiplier: 0.08,
    })

    enabled.value = false
    await nextTick()
    expect(preview?.summaries.value).toEqual([])

    wrapper.unmount()
  })
})
