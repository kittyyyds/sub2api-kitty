import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('../client', () => ({
  apiClient: { post },
}))

import {
  estimateBehavior,
  estimateProbes,
  runBehavior,
  runProbes,
  type BehaviorAuditEstimate,
  type CapabilityAuditEstimate,
} from '@/api/admin/modelCapabilityAudit'

describe('admin model capability audit preview contract', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('carries the capability preview token into the billable start request', async () => {
    const estimate: CapabilityAuditEstimate = {
      target_count: 1,
      protocol_count: 3,
      request_count: 3,
      estimated_max_seconds: 120,
      max_items: 60,
      preview_token: 'capability-preview-token',
      expires_at: '2026-09-05T10:05:00Z',
    }
    post.mockResolvedValueOnce({ data: estimate }).mockResolvedValueOnce({ data: { id: 7 } })

    const preview = await estimateProbes({ active_only: true, max_items: 60 })
    await runProbes({ active_only: true, max_items: 60, preview_token: preview.preview_token })

    expect(post).toHaveBeenNthCalledWith(1, '/admin/model-capability-audit/probe-preview', {
      active_only: true,
      max_items: 60,
    })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/model-capability-audit/probe', {
      active_only: true,
      max_items: 60,
      preview_token: 'capability-preview-token',
    })
  })

  it('carries the behavior preview token into the paired start request', async () => {
    const estimate: BehaviorAuditEstimate = {
      target_count: 1,
      available_target_count: 1,
      anchor_count: 1,
      pair_count: 3,
      request_count: 6,
      estimated_max_seconds: 180,
      can_run: true,
      targets_truncated: false,
      preview_token: 'behavior-preview-token',
      expires_at: '2026-09-05T10:05:00Z',
    }
    post.mockResolvedValueOnce({ data: estimate }).mockResolvedValueOnce({ data: { id: 8 } })

    const request = { model_id: 'gpt-5', case_count: 3, reasoning_effort: 'low' as const }
    const preview = await estimateBehavior(request)
    await runBehavior({ ...request, preview_token: preview.preview_token })

    expect(post).toHaveBeenNthCalledWith(1, '/admin/model-capability-audit/behavior-preview', request)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/model-capability-audit/behavior', {
      ...request,
      preview_token: 'behavior-preview-token',
    })
  })
})
