import { describe, expect, it, vi } from 'vitest'
import { videoUpstreamAPI } from '@/api/admin/videoUpstream'

const post = vi.hoisted(() => vi.fn())
vi.mock('@/api/client', () => ({ apiClient: { post } }))

describe('group pricing catalog refresh API', () => {
  it('posts to probe and returns the catalog without using the repricing endpoint', async () => {
    const catalog = { version: '2', models: {} }
    post.mockResolvedValue({ data: catalog })
    expect(await videoUpstreamAPI.probe()).toBe(catalog)
    expect(post).toHaveBeenCalledOnce()
    expect(post).toHaveBeenCalledWith('/admin/video-upstream/probe', {}, { timeout: 30000 })
  })
})
