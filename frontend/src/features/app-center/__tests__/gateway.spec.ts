import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { buildGatewayUrl, imageGenerationAPI, videoGenerationAPI } = vi.hoisted(() => ({
  buildGatewayUrl: vi.fn((path: string) => `https://gateway.test${path}`),
  imageGenerationAPI: {
    submitGateway: vi.fn(),
    submitAsync: vi.fn(),
    pollAsync: vi.fn(),
  },
  videoGenerationAPI: {
    submitGateway: vi.fn(),
    status: vi.fn(),
    createContentURL: vi.fn(),
  },
}))

vi.mock('@/api/client', () => ({ buildGatewayUrl }))
vi.mock('@/api/imageGeneration', () => ({
  imageGenerationAPI,
  isAsyncImageGatewayUnavailableError: (error: unknown) => (
    typeof error === 'object' && error !== null
    && 'code' in error && error.code === 'ASYNC_IMAGE_GATEWAY_UNAVAILABLE'
  ),
  isAsyncImageTaskError: (error: unknown) => (
    typeof error === 'object' && error !== null
    && 'name' in error && error.name === 'AsyncImageTaskError'
  ),
}))
vi.mock('@/api/videoGeneration', () => ({ videoGenerationAPI }))

import { executeMediaAction, executePlannerAction, parsePlannerJSON, persistentMediaURL } from '../gateway'
import type { ApplicationNextAction } from '../types'

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    text: vi.fn().mockResolvedValue(JSON.stringify(body)),
  } as unknown as Response
}

function plannerAction(overrides: Partial<ApplicationNextAction> = {}): ApplicationNextAction {
  return {
    step_key: 'plan',
    executor: 'chat_completions',
    credential_role: 'planner',
    api_key_id: 1,
    session_id: 'workflow-session',
    path: '/v1/chat/completions',
    method: 'POST',
    content_type: 'application/json',
    request: {
      model: 'gpt-5.5',
      messages: [{ role: 'user', content: 'plan it' }],
      stream: true,
    },
    body: {},
    repair_instruction: 'Return only valid JSON.',
    ...overrides,
  }
}

describe('application center gateway planner', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    buildGatewayUrl.mockClear()
    imageGenerationAPI.submitGateway.mockReset()
    imageGenerationAPI.submitAsync.mockReset()
    imageGenerationAPI.pollAsync.mockReset()
    videoGenerationAPI.submitGateway.mockReset()
    videoGenerationAPI.status.mockReset()
    videoGenerationAPI.createContentURL.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => vi.unstubAllGlobals())

  it('parses fenced JSON and bounded JSON embedded in prose', () => {
    expect(parsePlannerJSON('```json\n{"prompt":"clean"}\n```')).toEqual({ prompt: 'clean' })
    expect(parsePlannerJSON('Here is the plan: {"prompt":"clean"} done.')).toEqual({ prompt: 'clean' })
  })

  it('forces non-streaming chat, keeps session affinity, and repairs invalid JSON once', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ choices: [{ message: { content: 'not-json' } }] }))
      .mockResolvedValueOnce(jsonResponse({ choices: [{ message: { content: '{"final_prompt":"repaired","negative_prompt":null,"rationale":"keeps the subject"}' } }] }))

    const result = await executePlannerAction('sk-planner', plannerAction(), new AbortController().signal)

    expect(result.output).toEqual({
      final_prompt: 'repaired',
      negative_prompt: null,
      rationale: 'keeps the subject',
    })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    const firstInit = fetchMock.mock.calls[0][1] as RequestInit
    expect(firstInit.headers).toEqual({
      Authorization: 'Bearer sk-planner',
      'Content-Type': 'application/json',
      'X-Session-Id': 'workflow-session',
    })
    expect(JSON.parse(String(firstInit.body))).toEqual(expect.objectContaining({ stream: false }))
    const repairedBody = JSON.parse(String((fetchMock.mock.calls[1][1] as RequestInit).body))
    expect(repairedBody.messages.slice(-2)).toEqual([
      { role: 'assistant', content: 'not-json' },
      { role: 'user', content: 'Return only valid JSON.' },
    ])
  })

  it('repairs schema-invalid JSON instead of completing a run with unusable planner output', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ choices: [{ message: { content: '{"prompt":"missing final prompt"}' } }] }))
      .mockResolvedValueOnce(jsonResponse({ choices: [{ message: { content: '{"final_prompt":"usable","negative_prompt":"blur","rationale":"clearer"}' } }] }))

    await expect(executePlannerAction('sk-planner', plannerAction(), new AbortController().signal))
      .resolves.toEqual(expect.objectContaining({
        output: { final_prompt: 'usable', negative_prompt: 'blur', rationale: 'clearer' },
      }))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('repairs overlong or additional planner fields instead of silently truncating them', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ choices: [{ message: { content: JSON.stringify({
        final_prompt: 'x'.repeat(8001),
        negative_prompt: null,
        rationale: '',
        extra: true,
      }) } }] }))
      .mockResolvedValueOnce(jsonResponse({ choices: [{ message: { content: '{"final_prompt":"bounded","negative_prompt":"","rationale":""}' } }] }))

    await expect(executePlannerAction('sk-planner', plannerAction(), new AbortController().signal))
      .resolves.toEqual(expect.objectContaining({
        output: { final_prompt: 'bounded', negative_prompt: '', rationale: '' },
      }))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('rejects a path that does not match the declared executor', async () => {
    await expect(executePlannerAction('sk-planner', plannerAction({ path: '/v1/images/generations' }), new AbortController().signal))
      .rejects.toThrow('路径与执行器不匹配')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('application center media gateway', () => {
  beforeEach(() => {
    buildGatewayUrl.mockClear()
    imageGenerationAPI.submitGateway.mockReset().mockResolvedValue([
      { url: 'https://cdn.test/sync-image.png', revisedPrompt: '' },
    ])
    imageGenerationAPI.submitAsync.mockReset().mockResolvedValue({
      taskId: 'image-1',
      status: 'processing',
      pollUrl: '/v1/images/tasks/image-1',
    })
    imageGenerationAPI.pollAsync.mockReset().mockResolvedValue([
      { url: 'https://cdn.test/image.png', revisedPrompt: 'refined image' },
    ])
    videoGenerationAPI.submitGateway.mockReset().mockResolvedValue({ requestId: 'video-1' })
    videoGenerationAPI.status.mockReset().mockResolvedValue({
      requestId: 'video-1',
      status: 'completed',
      videoUrl: '/v1/videos/video-1/content',
      errorMessage: '',
    })
    videoGenerationAPI.createContentURL.mockReset().mockResolvedValue('blob:video-1')
  })

  it('persists relative video URLs while using authenticated content locally', async () => {
    expect(persistentMediaURL('/v1/videos/video-1/content')).toBe(
      'https://gateway.test/v1/videos/video-1/content',
    )

    const result = await executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'video_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:run-1',
      path: '/v1/videos/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
      body: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
    }, [], new AbortController().signal, 'text_to_video')

    expect(result.output).toEqual({
      request_id: 'video-1',
      status: 'completed',
      video_url: 'https://gateway.test/v1/videos/video-1/content',
    })
    expect(result.localResult?.videoUrl).toBe('blob:video-1')
  })

  it('maps local multi-image input to reference_images only for reference-to-video', async () => {
    const images = [
      new File(['one'], 'one.png', { type: 'image/png' }),
      new File(['two'], 'two.png', { type: 'image/png' }),
    ]
    await executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'video_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:run-2',
      path: '/v1/videos/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-video', prompt: 'use references' },
      body: { model: 'grok-imagine-video', prompt: 'use references' },
    }, images, new AbortController().signal, 'reference_to_video')

    expect(videoGenerationAPI.submitGateway).toHaveBeenCalledWith(
      'sk-media',
      '/v1/videos/generations',
      expect.objectContaining({ reference_images: images }),
      expect.objectContaining({ sessionId: 'app:run-2' }),
    )
    expect(videoGenerationAPI.submitGateway.mock.calls[0]?.[2]).not.toHaveProperty('image')
  })

  it('checkpoints an asynchronous image task and resumes it without submitting again', async () => {
    const checkpoint = vi.fn().mockResolvedValue(undefined)
    const action = {
      step_key: 'media',
      executor: 'image_generation' as const,
      credential_role: 'media' as const,
      api_key_id: 2,
      session_id: 'app:image-run',
      path: '/v1/images/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-image', prompt: 'draw it' },
      body: {},
    }

    const result = await executeMediaAction(
      'sk-media',
      action,
      [],
      new AbortController().signal,
      'text_to_image',
      undefined,
      checkpoint,
    )

    expect(imageGenerationAPI.submitAsync).toHaveBeenCalledWith(
      'sk-media',
      '/v1/images/generations/async',
      action.request,
      expect.objectContaining({ sessionId: 'app:image-run', referenceImages: [] }),
    )
    expect(checkpoint).toHaveBeenCalledWith('image-1')
    expect(imageGenerationAPI.pollAsync).toHaveBeenCalledWith(
      'sk-media',
      'image-1',
      expect.objectContaining({ sessionId: 'app:image-run' }),
    )
    expect(result).toEqual({
      output: {
        count: 1,
        images: [{ url: 'https://cdn.test/image.png', revised_prompt: 'refined image' }],
      },
      gatewayRequestId: 'image-1',
      localResult: {
        type: 'images',
        images: [{ url: 'https://cdn.test/image.png', revisedPrompt: 'refined image' }],
      },
    })

    imageGenerationAPI.submitAsync.mockClear()
    await executeMediaAction(
      'sk-media',
      { ...action, gateway_request_id: 'image-1' },
      [],
      new AbortController().signal,
      'text_to_image',
      undefined,
      checkpoint,
    )
    expect(imageGenerationAPI.submitAsync).not.toHaveBeenCalled()
    expect(checkpoint).toHaveBeenCalledOnce()
  })

  it('uses asynchronous multipart image editing with the local references', async () => {
    const references = [
      new File(['one'], 'one.png', { type: 'image/png' }),
      new File(['two'], 'two.png', { type: 'image/png' }),
    ]
    await executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'image_edit',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:image-edit',
      path: '/v1/images/edits',
      method: 'POST',
      content_type: 'multipart/form-data',
      request: { model: 'grok-imagine-image', prompt: 'edit it' },
      body: {},
    }, references, new AbortController().signal, 'image_edit')

    expect(imageGenerationAPI.submitAsync).toHaveBeenCalledWith(
      'sk-media',
      '/v1/images/edits/async',
      expect.objectContaining({ prompt: 'edit it' }),
      expect.objectContaining({ referenceImages: references }),
    )
  })

  it('falls back to synchronous images only when asynchronous submission returns 404', async () => {
    imageGenerationAPI.submitAsync.mockRejectedValueOnce(Object.assign(new Error('not enabled'), {
      code: 'ASYNC_IMAGE_GATEWAY_UNAVAILABLE',
      status: 404,
    }))
    const result = await executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'image_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:fallback',
      path: '/v1/images/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'gpt-image-2', prompt: 'fallback safely' },
      body: {},
    }, [], new AbortController().signal, 'text_to_image')

    expect(imageGenerationAPI.submitGateway).toHaveBeenCalledOnce()
    expect(imageGenerationAPI.pollAsync).not.toHaveBeenCalled()
    expect(result).not.toHaveProperty('gatewayRequestId')

    imageGenerationAPI.submitAsync.mockRejectedValueOnce(new Error('upstream unavailable'))
    await expect(executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'image_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:no-fallback',
      path: '/v1/images/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'gpt-image-2', prompt: 'do not retry' },
      body: {},
    }, [], new AbortController().signal, 'text_to_image')).rejects.toThrow('upstream unavailable')
    expect(imageGenerationAPI.submitGateway).toHaveBeenCalledOnce()
  })

  it('keeps checkpointed image tasks resumable on polling errors and marks terminal failures', async () => {
    const action = {
      step_key: 'media',
      executor: 'image_generation' as const,
      credential_role: 'media' as const,
      api_key_id: 2,
      session_id: 'app:image-error',
      path: '/v1/images/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-image', prompt: 'draw it' },
      body: {},
    }
    const checkpoint = vi.fn().mockResolvedValue(undefined)
    imageGenerationAPI.pollAsync.mockRejectedValueOnce(new Error('temporary polling failure'))

    await expect(executeMediaAction(
      'sk-media', action, [], new AbortController().signal, 'text_to_image', undefined, checkpoint,
    )).rejects.toMatchObject({
      name: 'ImageContinuationError',
      taskId: 'image-1',
      status: 'processing',
      checkpointed: true,
    })

    imageGenerationAPI.pollAsync.mockRejectedValueOnce(Object.assign(new Error('image rejected'), {
      name: 'AsyncImageTaskError',
      taskId: 'image-1',
      status: 'failed',
    }))
    await expect(executeMediaAction(
      'sk-media', action, [], new AbortController().signal, 'text_to_image', undefined, checkpoint,
    )).rejects.toMatchObject({
      name: 'ImageContinuationError',
      taskId: 'image-1',
      status: 'failed',
      checkpointed: true,
      message: 'image rejected',
    })
  })

  it('checkpoints a submitted video and resumes polling without submitting it again', async () => {
    const checkpoint = vi.fn().mockResolvedValue(undefined)
    const action = {
      step_key: 'media',
      executor: 'video_generation' as const,
      credential_role: 'media' as const,
      api_key_id: 2,
      session_id: 'app:run-3',
      path: '/v1/videos/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
      body: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
    }

    await executeMediaAction('sk-media', action, [], new AbortController().signal, 'text_to_video', checkpoint)
    expect(checkpoint).toHaveBeenCalledWith('video-1')
    expect(videoGenerationAPI.submitGateway).toHaveBeenCalledTimes(1)

    videoGenerationAPI.submitGateway.mockClear()
    await executeMediaAction('sk-media', {
      ...action,
      gateway_request_id: 'video-1',
    }, [], new AbortController().signal, 'text_to_video', checkpoint)
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(videoGenerationAPI.status).toHaveBeenCalledWith(
      'sk-media',
      'video-1',
      expect.any(AbortSignal),
      { sessionId: 'app:run-3' },
    )
  })

  it('keeps checkpointed video runs resumable when polling is interrupted', async () => {
    videoGenerationAPI.status.mockRejectedValueOnce(new Error('temporary status failure'))
    await expect(executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'video_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:run-4',
      path: '/v1/videos/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
      body: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
    }, [], new AbortController().signal, 'text_to_video', vi.fn().mockResolvedValue(undefined)))
      .rejects.toMatchObject({
        name: 'VideoContinuationError',
        requestId: 'video-1',
        status: 'processing',
        checkpointed: true,
      })
  })

  it.each([
    ['failed', 'upstream rejected the video'],
    ['expired', 'upstream task expired'],
  ])('returns structured terminal errors for %s tasks', async (status, errorMessage) => {
    videoGenerationAPI.status.mockResolvedValueOnce({
      requestId: 'video-1',
      status,
      videoUrl: '',
      errorMessage,
    })

    await expect(executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'video_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:run-terminal',
      path: '/v1/videos/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
      body: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
    }, [], new AbortController().signal, 'text_to_video', vi.fn().mockResolvedValue(undefined)))
      .rejects.toMatchObject({
        name: 'VideoContinuationError',
        requestId: 'video-1',
        status,
        checkpointed: true,
        message: errorMessage,
      })
    expect(videoGenerationAPI.createContentURL).not.toHaveBeenCalled()
  })

  it('keeps a completed task resumable when its content is temporarily unreadable', async () => {
    videoGenerationAPI.status.mockResolvedValueOnce({
      requestId: 'video-1',
      status: 'done',
      videoUrl: '',
      errorMessage: '',
    })
    videoGenerationAPI.createContentURL.mockRejectedValueOnce(new Error('content not ready'))

    await expect(executeMediaAction('sk-media', {
      step_key: 'media',
      executor: 'video_generation',
      credential_role: 'media',
      api_key_id: 2,
      session_id: 'app:run-content',
      path: '/v1/videos/generations',
      method: 'POST',
      content_type: 'application/json',
      request: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
      body: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
    }, [], new AbortController().signal, 'text_to_video', vi.fn().mockResolvedValue(undefined)))
      .rejects.toMatchObject({
        name: 'VideoContinuationError',
        requestId: 'video-1',
        status: 'processing',
        checkpointed: true,
      })
  })
})
