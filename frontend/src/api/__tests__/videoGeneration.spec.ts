import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { buildGatewayUrl } = vi.hoisted(() => ({
  buildGatewayUrl: vi.fn((path: string) => `https://gateway.test${path}`),
}))

vi.mock('@/api/url', () => ({ buildGatewayUrl }))

import {
  GROK_VIDEO_15_MODEL,
  GROK_VIDEO_MODEL,
  GROK_VIDEO_MODELS,
  buildVideoContentURL,
  createVideoContentURL,
  getVideoGenerationStatus,
  parseVideoGenerationStatus,
  submitVideoEdit,
  submitVideoExtension,
  submitVideoGeneration,
} from '@/api/videoGeneration'

function jsonResponse(body: unknown, status = 200, contentType = 'application/json'): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (name: string) => name.toLowerCase() === 'content-type' ? contentType : null },
    text: vi.fn().mockResolvedValue(typeof body === 'string' ? body : JSON.stringify(body)),
  } as unknown as Response
}

describe('videoGeneration API', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    buildGatewayUrl.mockClear()
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('submits text-to-video as JSON with the selected Grok video model', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ request_id: 'video-request-1' }))

    const result = await submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: ' ocean waves at sunset ',
      duration: 10,
      resolution: '720p',
      aspectRatio: '16:9',
    })

    expect(result).toEqual({ requestId: 'video-request-1' })
    expect(buildGatewayUrl).toHaveBeenCalledWith('/v1/videos/generations')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('https://gateway.test/v1/videos/generations')
    expect(init.headers).toEqual({
      Authorization: 'Bearer sk-video',
      'Content-Type': 'application/json',
    })
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'grok-imagine-video',
      prompt: 'ocean waves at sunset',
      duration: 10,
      resolution: '720p',
      aspect_ratio: '16:9',
    })
  })

  it('converts an image File to an official image URL object for image-to-video', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ data: { request_id: 'video-request-2' } }))
    const image = new File(['image-bytes'], 'source.png', { type: 'image/png' })

    await submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_15_MODEL,
      prompt: 'animate this frame',
      duration: 8,
      resolution: '1080p',
      image,
    })

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'grok-imagine-video-1.5',
      prompt: 'animate this frame',
      duration: 8,
      resolution: '1080p',
      image: { url: 'data:image/png;base64,aW1hZ2UtYnl0ZXM=' },
    })
  })

  it('sends session affinity and multiple references to video edit and extension endpoints', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ request_id: 'edit-request' }))
      .mockResolvedValueOnce(jsonResponse({ request_id: 'extension-request' }))
    const first = new File(['first'], 'first.png', { type: 'image/png' })
    const second = new File(['second'], 'second.png', { type: 'image/png' })
    const video = new File(['video'], 'clip.mp4', { type: 'video/mp4' })

    await submitVideoEdit('sk-video', {
      model: GROK_VIDEO_15_MODEL,
      prompt: 'edit these frames',
      images: [first, second],
    }, { sessionId: 'session-edit' })
    await submitVideoExtension('sk-video', {
      model: GROK_VIDEO_15_MODEL,
      prompt: 'continue the scene',
      video,
    }, { sessionId: 'session-extension' })

    const [, editInit] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(buildGatewayUrl).toHaveBeenNthCalledWith(1, '/v1/videos/edits')
    expect(editInit.headers).toEqual({
      Authorization: 'Bearer sk-video',
      'Content-Type': 'application/json',
      'X-Session-Id': 'session-edit',
    })
    expect(JSON.parse(String(editInit.body))).toEqual(expect.objectContaining({
      images: [
        { url: 'data:image/png;base64,Zmlyc3Q=' },
        { url: 'data:image/png;base64,c2Vjb25k' },
      ],
    }))

    const [, extensionInit] = fetchMock.mock.calls[1] as [string, RequestInit]
    expect(buildGatewayUrl).toHaveBeenNthCalledWith(2, '/v1/videos/extensions')
    expect(extensionInit.headers).toEqual({
      Authorization: 'Bearer sk-video',
      'Content-Type': 'application/json',
      'X-Session-Id': 'session-extension',
    })
    expect(JSON.parse(String(extensionInit.body))).toEqual(expect.objectContaining({
      video: { url: 'data:video/mp4;base64,dmlkZW8=' },
    }))
  })

  it('accepts either video model for text and image input', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ request_id: 'video-request-legacy-image' }))
      .mockResolvedValueOnce(jsonResponse({ request_id: 'video-request-15-text' }))

    const image = new File(['legacy-image'], 'legacy.png', { type: 'image/png' })
    await submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: 'animate with the classic model',
      duration: 8,
      resolution: '720p',
      image,
    })
    await submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_15_MODEL,
      prompt: 'generate with 1.5 from text',
      duration: 8,
      resolution: '720p',
    })

    expect(GROK_VIDEO_MODELS).toEqual(['grok-imagine-video', 'grok-imagine-video-1.5'])
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual(expect.objectContaining({
      model: 'grok-imagine-video',
      image: { url: 'data:image/png;base64,bGVnYWN5LWltYWdl' },
    }))
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual(expect.objectContaining({
      model: 'grok-imagine-video-1.5',
    }))
  })

  it('accepts 1.5 text-to-video at 1080p and rejects unsupported 1080p models before fetch', async () => {
    await expect(submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: 'too long',
      duration: 16,
      resolution: '480p',
    })).rejects.toThrow('1-15')

    fetchMock.mockResolvedValueOnce(jsonResponse({ request_id: 'video-request-1080p-text' }))
    await expect(submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_15_MODEL,
      prompt: 'text-only 1080p',
      duration: 8,
      resolution: '1080p',
    })).resolves.toEqual({ requestId: 'video-request-1080p-text' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual(expect.objectContaining({
      model: GROK_VIDEO_15_MODEL,
      resolution: '1080p',
    }))
    await expect(submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: 'classic image 1080p',
      duration: 8,
      resolution: '1080p',
      image: new File(['image'], 'source.png', { type: 'image/png' }),
    })).rejects.toThrow('1080p')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('rejects video ratios that the Grok upstream does not accept', async () => {
    await expect(submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: 'legacy ratio should not reach upstream',
      duration: 8,
      resolution: '480p',
      aspectRatio: '5:4',
    })).rejects.toThrow('受支持的 Grok 视频画面比例')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('normalizes task status, video URL, and nested failure details', () => {
    expect(parseVideoGenerationStatus({
      request_id: 'video-request-3',
      status: 'DONE',
      video: { url: '/v1/videos/video-request-3/content', duration: 8 },
    })).toEqual({
      requestId: 'video-request-3',
      status: 'done',
      videoUrl: '/v1/videos/video-request-3/content',
      errorMessage: '',
    })

    expect(parseVideoGenerationStatus({
      data: {
        status: 'failed',
        error: { message: 'upstream rejected the prompt' },
      },
    }, 'video-request-4')).toEqual({
      requestId: 'video-request-4',
      status: 'failed',
      videoUrl: '',
      errorMessage: 'upstream rejected the prompt',
    })

    expect(parseVideoGenerationStatus({
      video: { request_id: 'video-request-5' },
      status: 'completed',
      video_url: 'https://vidgen.x.ai/video-request-5.mp4',
    })).toEqual({
      requestId: 'video-request-5',
      status: 'completed',
      videoUrl: 'https://vidgen.x.ai/video-request-5.mp4',
      errorMessage: '',
    })
  })

  it('queries an encoded task ID with the selected user key', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ status: 'processing' }))

    const result = await getVideoGenerationStatus('sk-video', ' request/5 ')

    expect(result).toEqual({
      requestId: 'request/5',
      status: 'processing',
      videoUrl: '',
      errorMessage: '',
    })
    expect(buildGatewayUrl).toHaveBeenCalledWith('/v1/videos/request%2F5')
    expect(fetchMock).toHaveBeenCalledWith('https://gateway.test/v1/videos/request%2F5', {
      method: 'GET',
      headers: { Authorization: 'Bearer sk-video' },
      signal: undefined,
    })
  })

  it('builds and fetches the authenticated content endpoint as an object URL', async () => {
    const videoBlob = new Blob(['video'], { type: 'video/mp4' })
    fetchMock.mockResolvedValue({
      ...jsonResponse(''),
      blob: vi.fn().mockResolvedValue(videoBlob),
    })
    const createObjectURL = vi.fn().mockReturnValue('blob:video-result')
    vi.stubGlobal('URL', { createObjectURL })

    expect(buildVideoContentURL('video request')).toBe(
      'https://gateway.test/v1/videos/video%20request/content'
    )
    await expect(createVideoContentURL('sk-video', 'video request')).resolves.toBe('blob:video-result')
    expect(fetchMock).toHaveBeenCalledWith(
      'https://gateway.test/v1/videos/video%20request/content',
      {
        method: 'GET',
        headers: { Authorization: 'Bearer sk-video' },
        signal: undefined,
      }
    )
    expect(createObjectURL).toHaveBeenCalledWith(videoBlob)
  })

  it('marks generic upstream video blobs as MP4 for browser playback', async () => {
    const genericBlob = new Blob(['video'], { type: 'application/octet-stream' })
    fetchMock.mockResolvedValue({
      ...jsonResponse(''),
      blob: vi.fn().mockResolvedValue(genericBlob),
    })
    const createObjectURL = vi.fn().mockReturnValue('blob:playable-video')
    vi.stubGlobal('URL', { createObjectURL })

    await expect(createVideoContentURL('sk-video', 'video-request')).resolves.toBe('blob:playable-video')

    const playableBlob = createObjectURL.mock.calls[0]?.[0] as Blob
    expect(playableBlob.type).toBe('video/mp4')
    expect(playableBlob.size).toBe(genericBlob.size)
  })

  it('surfaces bounded API errors and malformed successful responses', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ error: { message: 'x'.repeat(800) } }, 400))
    await expect(submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: 'test',
      duration: 8,
      resolution: '480p',
    })).rejects.toThrow(`${'x'.repeat(600)}...`)

    fetchMock.mockResolvedValueOnce(jsonResponse({ status: 'done' }))
    await expect(submitVideoGeneration('sk-video', {
      model: GROK_VIDEO_MODEL,
      prompt: 'test',
      duration: 8,
      resolution: '480p',
    })).rejects.toThrow('未返回 request_id')
  })
})
