import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { get, post } }))

import { appCenterAPI } from '../api'

describe('application center API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('uses the exact application and run endpoints', async () => {
    get.mockResolvedValueOnce({ data: { items: [{ slug: 'inspiration-image-refiner' }] } })
    post.mockResolvedValueOnce({ data: { id: 'run/1', status: 'running' } })
    post.mockResolvedValueOnce({ data: { id: 'run/1', status: 'running' } })
    post.mockResolvedValueOnce({ data: { id: 'run/1', status: 'failed' } })
    post.mockResolvedValueOnce({ data: { id: 'run/1', status: 'running' } })
    post.mockResolvedValueOnce({ data: { id: 'run/1', status: 'running' } })
    post.mockResolvedValueOnce({ data: { id: 'run/1', status: 'cancelled' } })

    await expect(appCenterAPI.listApplications()).resolves.toEqual([{ slug: 'inspiration-image-refiner' }])
    await appCenterAPI.startRun('inspiration-image-refiner', {
      planner_api_key_id: 1,
      media_api_key_id: 2,
      input: { brief: 'test' },
    })
    await appCenterAPI.completeStep('run/1', 'plan/step', { output: { prompt: 'ready' } })
    await appCenterAPI.failStep('run/1', 'media/step', { error: 'failed' })
    await appCenterAPI.checkpointStep('run/1', 'media/step', {
      output: { request_id: 'video-1', status: 'processing' },
      gateway_request_id: 'video-1',
    })
    await appCenterAPI.retryRun('run/1')
    await appCenterAPI.cancelRun('run/1')

    expect(post).toHaveBeenNthCalledWith(1, '/apps/inspiration-image-refiner/runs', expect.objectContaining({ input: { brief: 'test' } }), { signal: undefined })
    expect(post).toHaveBeenNthCalledWith(2, '/apps/runs/run%2F1/steps/plan%2Fstep/complete', { output: { prompt: 'ready' } }, { signal: undefined })
    expect(post).toHaveBeenNthCalledWith(3, '/apps/runs/run%2F1/steps/media%2Fstep/fail', { error: 'failed' }, { signal: undefined })
    expect(post).toHaveBeenNthCalledWith(4, '/apps/runs/run%2F1/steps/media%2Fstep/checkpoint', {
      output: { request_id: 'video-1', status: 'processing' },
      gateway_request_id: 'video-1',
    }, { signal: undefined })
    expect(post).toHaveBeenNthCalledWith(5, '/apps/runs/run%2F1/retry', {}, { signal: undefined })
    expect(post).toHaveBeenNthCalledWith(6, '/apps/runs/run%2F1/cancel', {}, { signal: undefined })
  })
})
