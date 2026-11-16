import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import BillingRulesPanel from '../BillingRulesPanel.vue'
import { createTestI18n } from '@/test/createTestI18n'
import type {
  BillingCatalogResponse,
  BillingPreviewRequest,
  BillingPreviewResult,
} from '@/api/billingPreview'

const { getBillingCatalog, previewBilling } = vi.hoisted(() => ({
  getBillingCatalog: vi.fn(),
  previewBilling: vi.fn(),
}))

vi.mock('@/api/billingPreview', () => ({
  getBillingCatalog,
  previewBilling,
}))

const catalog: BillingCatalogResponse = {
  groups: [
    {
      id: 21,
      name: 'CC-Max - Full',
      platform: 'anthropic',
      rate_multiplier: 0.7,
      peak_multiplier: 1,
      effective_rate_multiplier: 0.7,
      rate_source: 'group',
      models: [
        { name: 'claude-sonnet-4', platform: 'anthropic' },
        { name: 'claude-opus-4', platform: 'anthropic' },
      ],
    },
    {
      id: 22,
      name: 'Codex Pro',
      platform: 'openai',
      rate_multiplier: 0.8,
      peak_multiplier: 1,
      effective_rate_multiplier: 0.8,
      rate_source: 'user',
      models: [{ name: 'gpt-5.4', platform: 'openai' }],
    },
  ],
}

function resultFor(request: BillingPreviewRequest): BillingPreviewResult {
  const isDefault = request.input_tokens === 1_000_000 && request.output_tokens === 500_000
  const standardCost = isDefault ? 20 : 40
  const rate = request.group_id === 22 ? 0.8 : 0.7
  return {
    group_id: request.group_id,
    group_name: request.group_id === 22 ? 'Codex Pro' : 'CC-Max - Full',
    model: request.model,
    model_platform: request.model_platform,
    pricing_source: 'group',
    billing_mode: 'token',
    rate_multiplier: rate,
    peak_multiplier: 1,
    effective_rate_multiplier: rate,
    standard_cost: standardCost,
    actual_cost: standardCost * rate,
    breakdown: {
      input_cost: standardCost / 4,
      output_cost: standardCost * 0.75,
      cache_creation_cost: 0,
      cache_read_cost: 0,
    },
    charged_breakdown: {
      input_cost: (standardCost / 4) * rate,
      output_cost: (standardCost * 0.75) * rate,
      cache_creation_cost: 0,
      cache_read_cost: 0,
    },
    long_context_applied: false,
  }
}

async function mountLoadedPanel(): Promise<VueWrapper> {
  const wrapper = mount(BillingRulesPanel, {
    global: {
      plugins: [createTestI18n()],
      stubs: { Teleport: true },
    },
  })
  await flushPromises()
  await vi.advanceTimersByTimeAsync(321)
  await flushPromises()
  return wrapper
}

describe('BillingRulesPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getBillingCatalog.mockReset()
    previewBilling.mockReset()
    getBillingCatalog.mockResolvedValue(catalog)
    previewBilling.mockImplementation(async (request: BillingPreviewRequest) => resultFor(request))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads backend groups and displays the unified billing result', async () => {
    const wrapper = await mountLoadedPanel()

    expect(getBillingCatalog).toHaveBeenCalledTimes(1)
    expect(previewBilling).toHaveBeenCalledWith(
      expect.objectContaining({
        group_id: 21,
        model: 'claude-sonnet-4',
        model_platform: 'anthropic',
        input_tokens: 1_000_000,
        output_tokens: 500_000,
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(wrapper.get('[data-testid="estimated-cost"]').text()).toBe('$14.00')
    expect(wrapper.get('[data-testid="billing-equation"]').text()).toContain('$20.00')
    expect(wrapper.get('[data-testid="billing-equation"]').text()).toContain('0.7x')
    expect(wrapper.text()).toContain('CC-Max - Full')
  })

  it('re-estimates after the customer changes only input and output tokens', async () => {
    const wrapper = await mountLoadedPanel()

    await wrapper.get('[data-testid="input-tokens"]').setValue('2000000')
    await wrapper.get('[data-testid="output-tokens"]').setValue('1000000')
    await vi.advanceTimersByTimeAsync(321)
    await flushPromises()

    expect(previewBilling).toHaveBeenLastCalledWith(
      expect.objectContaining({
        input_tokens: 2_000_000,
        output_tokens: 1_000_000,
        cache_creation_tokens: 0,
        cache_read_tokens: 0,
      }),
      expect.any(Object),
    )
    expect(wrapper.get('[data-testid="estimated-cost"]').text()).toBe('$28.00')
  })

  it('invalidates an in-flight estimate as soon as the inputs change', async () => {
    let resolveInitial: ((result: BillingPreviewResult) => void) | undefined
    previewBilling.mockImplementationOnce(
      () => new Promise<BillingPreviewResult>((resolve) => { resolveInitial = resolve }),
    )

    const wrapper = mount(BillingRulesPanel, {
      global: {
        plugins: [createTestI18n()],
        stubs: { Teleport: true },
      },
    })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(321)
    await flushPromises()

    const initialRequest = previewBilling.mock.calls[0]?.[0] as BillingPreviewRequest
    const initialSignal = previewBilling.mock.calls[0]?.[1]?.signal as AbortSignal
    expect(initialSignal.aborted).toBe(false)

    await wrapper.get('[data-testid="input-tokens"]').setValue('2000000')
    await flushPromises()
    expect(initialSignal.aborted).toBe(true)

    resolveInitial?.(resultFor(initialRequest))
    await flushPromises()
    expect(wrapper.get('[data-testid="estimated-cost"]').text()).toBe('--')

    await vi.advanceTimersByTimeAsync(321)
    await flushPromises()
    expect(wrapper.get('[data-testid="estimated-cost"]').text()).toBe('$28.00')
  })

  it('switches models automatically when a backend-created group is selected', async () => {
    const wrapper = await mountLoadedPanel()

    await wrapper.get('[data-testid="billing-group"] button').trigger('click')
    const option = wrapper
      .findAll('[role="option"]')
      .find((item) => item.text().includes('Codex Pro'))
    expect(option).toBeDefined()
    await option!.trigger('click')
    await vi.advanceTimersByTimeAsync(321)
    await flushPromises()

    expect(previewBilling).toHaveBeenLastCalledWith(
      expect.objectContaining({
        group_id: 22,
        model: 'gpt-5.4',
        model_platform: 'openai',
      }),
      expect.any(Object),
    )
    expect(wrapper.text()).toContain('已应用专属倍率')
  })
})
