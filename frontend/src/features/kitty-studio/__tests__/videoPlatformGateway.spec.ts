import { afterEach, describe, expect, it, vi } from 'vitest'
import { generateStudioAsset } from '../studioGateway'
import { videoGenerationAPI } from '@/api/videoGeneration'

vi.mock('@/api/videoGeneration', async () => {
  const actual = await vi.importActual<typeof import('@/api/videoGeneration')>('@/api/videoGeneration')
  return { ...actual, videoGenerationAPI: { ...actual.videoGenerationAPI, submitGateway: vi.fn(), status: vi.fn(), content: vi.fn() } }
})

describe('independent video platform gateway', () => {
  afterEach(() => vi.clearAllMocks())
  it('submits custom exact video model IDs and retrieves the authenticated result', async () => {
    vi.mocked(videoGenerationAPI.submitGateway).mockResolvedValue({ requestId: 'video-custom-task' })
    vi.mocked(videoGenerationAPI.status).mockResolvedValue({ requestId: 'video-custom-task', status: 'done', errorMessage: '', videoUrl: '' })
    const blob = new Blob(['fixture'], { type: 'video/mp4' })
    vi.mocked(videoGenerationAPI.content).mockResolvedValue(blob)
    const signal = new AbortController().signal
    const result = await generateStudioAsset({
      apiKey: 'video-fixture-key', signal, references: [],
      model: { id: 'Custom-Video-Exact', label: 'Custom', description: '', type: 'video', strategy: 'video-gateway' },
      form: { mode: 'text-video', model: 'Custom-Video-Exact', prompt: 'A cyclist', keyId: '1', count: 1, ratio: '16:9', resolution: '2k', videoResolution: '720p', videoDuration: 8, format: 'png', background: 'auto' },
    })
    expect(videoGenerationAPI.submitGateway).toHaveBeenCalledWith('video-fixture-key', '/v1/videos/generations', { model: 'Custom-Video-Exact', prompt: 'A cyclist', duration: 8, resolution: '720p', aspect_ratio: '16:9' }, { signal })
    expect(videoGenerationAPI.content).toHaveBeenCalledWith('video-fixture-key', 'video-custom-task', signal)
    expect(result[0]).toMatchObject({ type: 'video', requestId: 'video-custom-task', blob })
  })
})
