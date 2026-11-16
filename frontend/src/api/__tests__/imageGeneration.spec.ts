import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { buildGatewayUrl } = vi.hoisted(() => ({
  buildGatewayUrl: vi.fn((path: string) => `https://gateway.test${path}`),
}))

vi.mock('@/api/url', () => ({ buildGatewayUrl }))

import {
  AsyncImageGatewayUnavailableError,
  AsyncImageTaskError,
  GROK_IMAGE_ASPECT_RATIOS,
  GROK_IMAGE_MODELS,
  KITTY_IMAGE_MODELS,
  editImage,
  extractGeneratedImages,
  generateImage,
  isAsyncImageGatewayUnavailableError,
  isAsyncImageTaskError,
  pollAsyncImageGatewayTask,
  submitAsyncImageGatewayRequest,
  submitImageGatewayRequest,
} from '@/api/imageGeneration'

function response(body: unknown, status = 200, contentType = 'application/json'): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (name: string) => name.toLowerCase() === 'content-type' ? contentType : null },
    text: vi.fn().mockResolvedValue(typeof body === 'string' ? body : JSON.stringify(body)),
  } as unknown as Response
}

describe('imageGeneration API', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    buildGatewayUrl.mockClear()
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps three resolution presets on the canonical image model', () => {
    expect(KITTY_IMAGE_MODELS.map(({ label, model }) => `${label} · ${model}`)).toEqual([
      '1K 标准 · gpt-image-2',
      '2K 标准 · gpt-image-2',
      '4K 标准 · gpt-image-2',
    ])
    expect(new Set(KITTY_IMAGE_MODELS.map(({ model }) => model))).toEqual(new Set(['gpt-image-2']))
    expect(GROK_IMAGE_MODELS).toEqual([
      'grok-imagine-image',
      'grok-imagine-image-2.0',
      'grok-imagine-image-quality',
    ])
    expect(GROK_IMAGE_ASPECT_RATIOS).toEqual([
      '1:1',
      '16:9',
      '9:16',
      '4:3',
      '3:4',
      '3:2',
      '2:3',
      '2:1',
      '1:2',
      '19.5:9',
      '9:19.5',
      '20:9',
      '9:20',
      'auto',
    ])
  })

  it('accepts the approved Grok image models for generation and editing', async () => {
    fetchMock
      .mockResolvedValueOnce(response({ data: [{ url: 'https://cdn.test/grok.png' }] }))
      .mockResolvedValueOnce(response({ data: [{ url: 'https://cdn.test/grok-edit.png' }] }))

    await generateImage('sk-grok', {
      model: 'grok-imagine-image-quality',
      prompt: 'high quality landscape',
      n: 1,
      size: '1024x1024',
      quality: 'high',
    })
    await editImage('sk-grok', {
      model: 'grok-imagine-image',
      prompt: 'replace the background',
      n: 1,
      size: '1024x1024',
      quality: 'auto',
    }, [new File(['image'], 'source.png', { type: 'image/png' })])

    expect(JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))).toEqual({
      model: 'grok-imagine-image-quality',
      prompt: 'high quality landscape',
      n: 1,
      size: '1024x1024',
    })
    expect(Array.from(((fetchMock.mock.calls[1][1] as RequestInit).body as FormData).entries()))
      .toContainEqual(['model', 'grok-imagine-image'])
  })

  it('posts text generation to the gateway with the selected user key', async () => {
    fetchMock.mockResolvedValue(response({
      data: [{ url: 'https://cdn.test/image.png', revised_prompt: 'refined prompt' }],
    }))

    const images = await generateImage('sk-private-value', {
      model: 'gpt-image-2',
      prompt: 'metal product on a clean set',
      n: 2,
      size: '1280x1024',
      quality: 'high',
    })

    expect(buildGatewayUrl).toHaveBeenCalledWith('/v1/images/generations')
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('https://gateway.test/v1/images/generations')
    expect(init.method).toBe('POST')
    expect(init.headers).toEqual({
      Authorization: 'Bearer sk-private-value',
      'Content-Type': 'application/json',
    })
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'gpt-image-2',
      prompt: 'metal product on a clean set',
      n: 2,
      size: '1280x1024',
      quality: 'high',
    })
    expect(images).toEqual([{ url: 'https://cdn.test/image.png', revisedPrompt: 'refined prompt' }])
  })

  it('uses multipart edits with at most five image fields and no manual content type', async () => {
    fetchMock.mockResolvedValue(response({ data: [{ b64_json: 'YWJj' }] }))
    const files = Array.from({ length: 7 }, (_, index) => (
      new File([`file-${index}`], `reference-${index}.png`, { type: 'image/png' })
    ))

    const images = await editImage('sk-edit-key', {
      model: 'gpt-image-2',
      prompt: 'keep the silhouette',
      n: 1,
      size: '1024x1024',
      quality: 'auto',
    }, files)

    expect(buildGatewayUrl).toHaveBeenCalledWith('/v1/images/edits')
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.headers).toEqual({ Authorization: 'Bearer sk-edit-key' })
    const entries = Array.from((init.body as FormData).entries())
    expect(entries.filter(([name]) => name === 'image' || name === 'image[]')).toHaveLength(5)
    expect(entries.filter(([name]) => name === 'image')).toHaveLength(1)
    expect(entries.filter(([name]) => name === 'image[]')).toHaveLength(4)
    expect(images[0].url).toBe('data:image/png;base64,YWJj')
  })

  it('normalizes verified response variants and drops empty items', () => {
    expect(extractGeneratedImages({
      images: [
        { image_url: 'https://cdn.test/one.webp', prompt: 'one' },
        null,
        { download_url: 'https://cdn.test/two.webp' },
      ],
    })).toEqual([
      { url: 'https://cdn.test/one.webp', revisedPrompt: 'one' },
      { url: 'https://cdn.test/two.webp', revisedPrompt: '' },
    ])
  })

  it('keeps synchronous gateway submission compatible', async () => {
    fetchMock.mockResolvedValue(response({
      data: [{ url: 'https://cdn.test/synchronous.png' }],
    }))

    await expect(submitImageGatewayRequest('sk-sync', '/v1/images/generations', {
      model: 'gpt-image-2',
      prompt: 'synchronous request',
    }, { sessionId: 'run-sync' })).resolves.toEqual([
      { url: 'https://cdn.test/synchronous.png', revisedPrompt: '' },
    ])

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.headers).toEqual({
      Authorization: 'Bearer sk-sync',
      'Content-Type': 'application/json',
      'X-Session-Id': 'run-sync',
    })
  })

  it('submits asynchronous image generation and parses its task contract', async () => {
    fetchMock.mockResolvedValue(response({
      task_id: 'imgtask_generation',
      status: 'processing',
      poll_url: '/v1/images/tasks/imgtask_generation',
    }, 202))
    const controller = new AbortController()

    const task = await submitAsyncImageGatewayRequest(
      'sk-async',
      '/v1/images/generations/async',
      { model: 'gpt-image-2', prompt: 'a lighthouse' },
      { sessionId: 'run-generation', signal: controller.signal },
    )

    expect(task).toEqual({
      taskId: 'imgtask_generation',
      status: 'processing',
      pollUrl: '/v1/images/tasks/imgtask_generation',
    })
    expect(buildGatewayUrl).toHaveBeenCalledWith('/v1/images/generations/async')
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init).toMatchObject({
      method: 'POST',
      headers: {
        Authorization: 'Bearer sk-async',
        'Content-Type': 'application/json',
        'X-Session-Id': 'run-generation',
      },
      signal: controller.signal,
    })
    expect(JSON.parse(String(init.body))).toEqual({ model: 'gpt-image-2', prompt: 'a lighthouse' })
  })

  it('submits asynchronous image edits as multipart with bounded references', async () => {
    fetchMock.mockResolvedValue(response({
      task_id: 'imgtask_edit',
      status: 'processing',
      poll_url: '/v1/images/tasks/imgtask_edit',
    }, 202))
    const files = Array.from({ length: 7 }, (_, index) => (
      new File([`file-${index}`], `reference-${index}.png`, { type: 'image/png' })
    ))

    await submitAsyncImageGatewayRequest('sk-edit', '/v1/images/edits/async', {
      model: 'grok-imagine-image',
      prompt: 'keep the subject',
      metadata: { source: 'application-center' },
    }, { referenceImages: files, sessionId: 'run-edit' })

    expect(buildGatewayUrl).toHaveBeenCalledWith('/v1/images/edits/async')
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.headers).toEqual({
      Authorization: 'Bearer sk-edit',
      'X-Session-Id': 'run-edit',
    })
    const entries = Array.from((init.body as FormData).entries())
    expect(entries).toContainEqual(['metadata', JSON.stringify({ source: 'application-center' })])
    expect(entries.filter(([name]) => name === 'image' || name === 'image[]')).toHaveLength(5)
  })

  it('continues processing polls and parses completed task results', async () => {
    fetchMock
      .mockResolvedValueOnce(response({ task_id: 'imgtask_poll', status: 'processing' }))
      .mockResolvedValueOnce(response({
        task_id: 'imgtask_poll',
        status: 'completed',
        result: {
          data: [{ url: 'https://cdn.test/persisted.png', revised_prompt: 'persisted prompt' }],
        },
      }))

    const images = await pollAsyncImageGatewayTask('sk-poll', 'imgtask_poll', {
      pollIntervalMs: 0,
      sessionId: 'run-poll',
    })

    expect(images).toEqual([{
      url: 'https://cdn.test/persisted.png',
      revisedPrompt: 'persisted prompt',
    }])
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(buildGatewayUrl).toHaveBeenNthCalledWith(1, '/v1/images/tasks/imgtask_poll')
    expect(buildGatewayUrl).toHaveBeenNthCalledWith(2, '/v1/images/tasks/imgtask_poll')
    expect((fetchMock.mock.calls[0][1] as RequestInit).headers).toEqual({
      Authorization: 'Bearer sk-poll',
      'X-Session-Id': 'run-poll',
    })
  })

  it('returns a bounded error for failed asynchronous tasks', async () => {
    fetchMock.mockResolvedValue(response({
      task_id: 'imgtask_failed',
      status: 'failed',
      error: { type: 'api_error', message: 'x'.repeat(800) },
    }))

    const taskError = await pollAsyncImageGatewayTask('sk-poll', 'imgtask_failed', {
      pollIntervalMs: 0,
    }).catch((error: unknown) => error)

    expect(taskError).toBeInstanceOf(AsyncImageTaskError)
    expect(isAsyncImageTaskError(taskError)).toBe(true)
    expect(taskError).toMatchObject({
      taskId: 'imgtask_failed',
      status: 'failed',
      message: `${'x'.repeat(600)}...`,
    })
  })

  it('marks only submission-stage 404 responses as safe synchronous fallbacks', async () => {
    fetchMock.mockResolvedValueOnce(response({
      error: { message: 'async image tasks are not enabled' },
    }, 404))
    const unavailable = await submitAsyncImageGatewayRequest(
      'sk-async',
      '/v1/images/generations/async',
      { model: 'gpt-image-2', prompt: 'fallback' },
    ).catch((error: unknown) => error)

    expect(unavailable).toBeInstanceOf(AsyncImageGatewayUnavailableError)
    expect(isAsyncImageGatewayUnavailableError(unavailable)).toBe(true)

    fetchMock.mockResolvedValueOnce(response({ error: { message: 'gateway failed' } }, 500))
    const submitFailure = await submitAsyncImageGatewayRequest(
      'sk-async',
      '/v1/images/generations/async',
      { model: 'gpt-image-2', prompt: 'do not retry' },
    ).catch((error: unknown) => error)
    expect(isAsyncImageGatewayUnavailableError(submitFailure)).toBe(false)

    fetchMock.mockResolvedValueOnce(response({ error: { message: 'image task not found' } }, 404))
    const pollFailure = await pollAsyncImageGatewayTask('sk-async', 'imgtask_missing', {
      pollIntervalMs: 0,
    }).catch((error: unknown) => error)
    expect(isAsyncImageGatewayUnavailableError(pollFailure)).toBe(false)
  })

  it('rejects malformed and non-202 asynchronous submission responses', async () => {
    fetchMock.mockResolvedValueOnce(response({
      task_id: 'imgtask_incomplete',
      status: 'processing',
    }, 202))
    await expect(submitAsyncImageGatewayRequest(
      'sk-async',
      '/v1/images/generations/async',
      { model: 'gpt-image-2', prompt: 'missing poll URL' },
    )).rejects.toThrow('没有返回完整的 task_id、status 和 poll_url')

    fetchMock.mockResolvedValueOnce(response({
      task_id: 'imgtask_wrong_status',
      status: 'processing',
      poll_url: '/v1/images/tasks/imgtask_wrong_status',
    }, 200))
    await expect(submitAsyncImageGatewayRequest(
      'sk-async',
      '/v1/images/generations/async',
      { model: 'gpt-image-2', prompt: 'wrong status code' },
    )).rejects.toThrow('无效的提交响应（HTTP 200）')
  })

  it('aborts while waiting for the next asynchronous task poll', async () => {
    fetchMock.mockResolvedValue(response({ task_id: 'imgtask_abort', status: 'processing' }))
    const controller = new AbortController()
    const polling = pollAsyncImageGatewayTask('sk-poll', 'imgtask_abort', {
      pollIntervalMs: 60_000,
      signal: controller.signal,
    })

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledOnce())
    controller.abort()

    await expect(polling).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it('returns a bounded backend error and handles HTML gateway failures', async () => {
    fetchMock.mockResolvedValueOnce(response({ error: { message: 'x'.repeat(800) } }, 400))
    await expect(generateImage('sk-key', {
      model: 'gpt-image-2',
      prompt: 'test',
      n: 1,
      size: '1024x1024',
      quality: 'auto',
    })).rejects.toThrow(`${'x'.repeat(600)}...`)

    fetchMock.mockResolvedValueOnce(response('<html>timeout</html>', 504, 'text/html'))
    await expect(generateImage('sk-key', {
      model: 'gpt-image-2',
      prompt: 'test',
      n: 1,
      size: '1024x1024',
      quality: 'auto',
    })).rejects.toThrow('图片生成等待超时')
  })
})
