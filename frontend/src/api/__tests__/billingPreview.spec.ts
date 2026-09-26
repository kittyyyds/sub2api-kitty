import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post },
}))

import { getBillingCatalog, previewBilling, type BillingPreviewRequest } from '@/api/billingPreview'

describe('billing preview api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    get.mockResolvedValue({ data: { groups: [] } })
    post.mockResolvedValue({ data: {} })
  })

  it('loads the authenticated billing catalog', async () => {
    const controller = new AbortController()
    await getBillingCatalog({ signal: controller.signal })

    expect(get).toHaveBeenCalledWith('/billing/catalog', { signal: controller.signal })
  })

  it('posts token categories to the unified preview endpoint', async () => {
    const request: BillingPreviewRequest = {
      group_id: 21,
      model: 'claude-sonnet-4',
      model_platform: 'anthropic',
      input_tokens: 1_000_000,
      output_tokens: 500_000,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      cache_creation_5m_tokens: 0,
      cache_creation_1h_tokens: 0,
      service_tier: '',
    }
    await previewBilling(request)

    expect(post).toHaveBeenCalledWith('/billing/preview', request, { signal: undefined })
  })
})
