import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getBillingCatalog, previewBilling } from '@/api/billingPreview'
import { useBillingEstimator } from '../useBillingEstimator'

vi.mock('@/api/billingPreview', () => ({ getBillingCatalog: vi.fn(), previewBilling: vi.fn() }))

const wrappers: ReturnType<typeof mount>[] = []
let estimator: ReturnType<typeof useBillingEstimator>

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  vi.mocked(getBillingCatalog).mockResolvedValue({ groups: [{
    id: 1, name: 'Existing group', platform: 'openai',
    models: [{ name: 'openai-model', platform: 'openai' }, { name: 'claude-model', platform: 'anthropic' }],
  }] } as Awaited<ReturnType<typeof getBillingCatalog>>)
  vi.mocked(previewBilling).mockResolvedValue({} as Awaited<ReturnType<typeof previewBilling>>)
})
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.useRealTimers()
})

async function setup() {
  wrappers.push(mount(defineComponent({ setup() { estimator = useBillingEstimator(); return () => null } })))
  await flushPromises()
}

describe('billing estimator TTL request fidelity', () => {
  it('keeps OpenAI 30m/12h buckets separate and ignores hidden Anthropic values', async () => {
    await setup()
    Object.assign(estimator.tokens, { cacheWrite5m: 7, cacheWrite1h: 11, cacheWrite30m: 60, cacheWrite12h: 40 })
    await estimator.estimateNow()
    expect(previewBilling).toHaveBeenLastCalledWith(expect.objectContaining({
      cache_creation_tokens: 100, cache_creation_30m_tokens: 60, cache_creation_12h_tokens: 40,
      cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0,
    }), expect.anything())
    estimator.selectedModelKey.value = JSON.stringify(['anthropic', 'claude-model'])
    await estimator.estimateNow()
    const request = vi.mocked(previewBilling).mock.lastCall![0]
    expect(request).toMatchObject({ cache_creation_tokens: 18, cache_creation_5m_tokens: 7, cache_creation_1h_tokens: 11 })
    expect(request).not.toHaveProperty('cache_creation_30m_tokens')
    expect(request).not.toHaveProperty('cache_creation_12h_tokens')
  })

  it('reset clears both new buckets and preserves a zero default estimate', async () => {
    await setup()
    estimator.tokens.cacheWrite30m = 60
    estimator.tokens.cacheWrite12h = 40
    estimator.reset()
    await estimator.estimateNow()
    expect(previewBilling).toHaveBeenLastCalledWith(expect.objectContaining({
      cache_creation_tokens: 0, cache_creation_30m_tokens: 0, cache_creation_12h_tokens: 0,
    }), expect.anything())
  })
})
