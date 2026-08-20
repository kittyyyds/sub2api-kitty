import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApplicationRun } from '../../types'

const apiMocks = vi.hoisted(() => ({
  cancelRun: vi.fn(),
  checkpointStep: vi.fn(),
  completeStep: vi.fn(),
  failStep: vi.fn(),
  getRun: vi.fn(),
  listRuns: vi.fn(),
  retryRun: vi.fn(),
  startRun: vi.fn(),
}))

const keyMocks = vi.hoisted(() => ({
  getById: vi.fn(),
  list: vi.fn(),
}))

const gatewayMocks = vi.hoisted(() => ({
  executeMediaAction: vi.fn(),
  executePlannerAction: vi.fn(),
}))

const videoMocks = vi.hoisted(() => ({
  createContentURL: vi.fn(),
}))

vi.mock('../../api', () => ({
  appCenterAPI: {
    cancelRun: apiMocks.cancelRun,
    checkpointStep: apiMocks.checkpointStep,
    completeStep: apiMocks.completeStep,
    failStep: apiMocks.failStep,
    getRun: apiMocks.getRun,
    listRuns: apiMocks.listRuns,
    retryRun: apiMocks.retryRun,
    startRun: apiMocks.startRun,
  },
}))

vi.mock('@/api/keys', () => ({ keysAPI: { getById: keyMocks.getById, list: keyMocks.list } }))

vi.mock('@/api/videoGeneration', () => ({
  videoGenerationAPI: { createContentURL: videoMocks.createContentURL },
}))

vi.mock('../../gateway', () => ({
  executeMediaAction: gatewayMocks.executeMediaAction,
  executePlannerAction: gatewayMocks.executePlannerAction,
  isImageContinuationError: (error: unknown) => error instanceof Error && error.name === 'ImageContinuationError',
  isVideoContinuationError: (error: unknown) => error instanceof Error && error.name === 'VideoContinuationError',
}))

import { useAppCenter } from '../useAppCenter'

function createRun(overrides: Partial<ApplicationRun> = {}): ApplicationRun {
  return {
    id: 'run-1',
    app_slug: 'free-media-studio',
    status: 'running',
    ...overrides,
  }
}

function videoRun(status: 'processing' | 'failed' | 'expired', overrides: Partial<ApplicationRun> = {}): ApplicationRun {
  return createRun({
    status: status === 'processing' ? 'running' : 'failed',
    media_api_key_id: 9,
    steps: [{
      step_key: 'media',
      status: status === 'processing' ? 'running' : 'failed',
      executor: 'video_generation',
      gateway_request_id: 'video-123',
      output: { request_id: 'video-123', status },
    }],
    ...overrides,
  })
}

function videoAction(requestId = '') {
  return {
    step_key: 'media',
    executor: 'video_generation' as const,
    credential_role: 'media' as const,
    api_key_id: 9,
    session_id: 'app:run-1',
    path: '/v1/videos/generations',
    method: 'POST',
    content_type: 'application/json',
    request: { model: 'grok-imagine-video-1.5', prompt: 'animate' },
    body: {},
    ...(requestId ? { gateway_request_id: requestId } : {}),
  }
}

function imageRun(status: 'processing' | 'failed', overrides: Partial<ApplicationRun> = {}): ApplicationRun {
  return createRun({
    status: status === 'processing' ? 'running' : 'failed',
    media_api_key_id: 9,
    steps: [{
      step_key: 'media',
      status: status === 'processing' ? 'running' : 'failed',
      executor: 'image_generation',
      gateway_request_id: 'image-123',
      output: { task_id: 'image-123', status },
    }],
    ...overrides,
  })
}

function imageAction(taskId = '') {
  return {
    step_key: 'media',
    executor: 'image_generation' as const,
    credential_role: 'media' as const,
    api_key_id: 9,
    session_id: 'app:run-1',
    path: '/v1/images/generations',
    method: 'POST',
    content_type: 'application/json',
    request: { model: 'grok-imagine-image', prompt: 'draw it' },
    body: {},
    ...(taskId ? { gateway_request_id: taskId } : {}),
  }
}

const mediaKey = {
  id: 9,
  user_id: 1,
  key: 'sk-media',
  name: 'Media key',
  group_id: 2,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '',
  updated_at: '',
  current_concurrency: 0,
  group: { name: 'Grok', platform: 'grok', allow_image_generation: true },
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
}

function mountComposable(): {
  center: ReturnType<typeof useAppCenter>
  unmount: () => void
} {
  let center!: ReturnType<typeof useAppCenter>
  const wrapper = mount(defineComponent({
    setup() {
      center = useAppCenter()
      return () => h('div')
    },
  }))
  return { center, unmount: () => wrapper.unmount() }
}

describe('useAppCenter history video preview', () => {
  const revokeObjectURL = vi.fn()

  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    Object.values(gatewayMocks).forEach((mock) => mock.mockReset())
    videoMocks.createContentURL.mockReset().mockResolvedValue('blob:history-video')
    keyMocks.getById.mockReset().mockResolvedValue(mediaKey)
    keyMocks.list.mockReset().mockResolvedValue({ items: [mediaKey] })
    apiMocks.listRuns.mockResolvedValue({ items: [] })
    revokeObjectURL.mockReset()
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
  })

  it('loads a completed history video with the run media key', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const run = createRun({
      status: 'completed',
      media_api_key_id: 9,
      session_id: 'app:run-1',
      output: {
        request_id: 'video-history',
        status: 'completed',
        video_url: 'https://gateway.test/v1/videos/video-history/content',
      },
      steps: [{
        step_key: 'media',
        status: 'completed',
        executor: 'video_generation',
        gateway_request_id: 'video-history',
        output: {
          request_id: 'video-history',
          status: 'completed',
          video_url: 'https://gateway.test/v1/videos/video-history/content',
        },
      }],
    })
    apiMocks.getRun.mockResolvedValue(run)

    await center.openHistoryRun(run)

    expect(videoMocks.createContentURL).toHaveBeenCalledWith(
      'sk-media',
      'video-history',
      expect.any(AbortSignal),
      { sessionId: 'app:run-1' },
    )
    expect(center.historyVideoPreviewUrl.value).toBe('blob:history-video')
    expect(center.historyVideoPreviewLoading.value).toBe(false)
    expect(center.historyVideoPreviewError.value).toBe('')
    unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:history-video')
  })

  it('reports an unavailable original media key without exposing the protected URL', async () => {
    keyMocks.list.mockResolvedValue({ items: [] })
    keyMocks.getById.mockRejectedValue(new Error('not found'))
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const run = createRun({
      status: 'completed',
      media_api_key_id: 9,
      output: { request_id: 'video-history', status: 'completed' },
      steps: [{
        step_key: 'media',
        status: 'completed',
        executor: 'video_generation',
        gateway_request_id: 'video-history',
        output: { request_id: 'video-history', status: 'completed' },
      }],
    })
    apiMocks.getRun.mockResolvedValue(run)

    await center.openHistoryRun(run)

    expect(videoMocks.createContentURL).not.toHaveBeenCalled()
    expect(center.historyVideoPreviewUrl.value).toBe('')
    expect(center.historyVideoPreviewError.value).toContain('原媒体密钥')
    unmount()
  })

  it('loads the exact owned media key when it is outside the first list page', async () => {
    keyMocks.list.mockResolvedValue({ items: [] })
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const run = createRun({
      status: 'completed',
      media_api_key_id: 9,
      output: { request_id: 'video-history', status: 'completed' },
      steps: [{
        step_key: 'media',
        status: 'completed',
        executor: 'video_generation',
        gateway_request_id: 'video-history',
        output: { request_id: 'video-history', status: 'completed' },
      }],
    })
    apiMocks.getRun.mockResolvedValue(run)

    await center.openHistoryRun(run)

    expect(keyMocks.getById).toHaveBeenCalledWith(9, { signal: expect.any(AbortSignal) })
    expect(videoMocks.createContentURL).toHaveBeenCalledWith(
      'sk-media',
      'video-history',
      expect.any(AbortSignal),
      { sessionId: undefined },
    )
    expect(center.historyVideoPreviewUrl.value).toBe('blob:history-video')
    unmount()
  })

  it.each(['quota_exhausted', 'expired'] as const)(
    'keeps completed videos readable with an original %s media key',
    async (status) => {
      keyMocks.list.mockResolvedValue({ items: [] })
      keyMocks.getById.mockResolvedValue({ ...mediaKey, status })
      const { center, unmount } = mountComposable()
      await center.loadKeys()
      const run = createRun({
        status: 'completed',
        media_api_key_id: mediaKey.id,
        session_id: 'app:run-1',
        output: { request_id: 'video-history', status: 'completed' },
        steps: [{
          step_key: 'media',
          status: 'completed',
          executor: 'video_generation',
          gateway_request_id: 'video-history',
          output: { request_id: 'video-history', status: 'completed' },
        }],
      })
      apiMocks.getRun.mockResolvedValue(run)

      await center.openHistoryRun(run)

      expect(videoMocks.createContentURL).toHaveBeenCalledWith(
        'sk-media',
        'video-history',
        expect.any(AbortSignal),
        { sessionId: 'app:run-1' },
      )
      expect(center.historyVideoPreviewUrl.value).toBe('blob:history-video')
      unmount()
    },
  )

  it('keeps the latest selection when history detail responses arrive out of order', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const runA = createRun({ id: 'run-a', status: 'completed' })
    const runB = createRun({ id: 'run-b', status: 'completed' })
    let resolveA!: (run: ApplicationRun) => void
    let resolveB!: (run: ApplicationRun) => void
    apiMocks.getRun.mockImplementation((id: string) => new Promise<ApplicationRun>((resolve) => {
      if (id === 'run-a') resolveA = resolve
      else resolveB = resolve
    }))

    const openingA = center.openHistoryRun(runA)
    await vi.waitFor(() => expect(apiMocks.getRun).toHaveBeenCalledTimes(1))
    const openingB = center.openHistoryRun(runB)
    await vi.waitFor(() => expect(apiMocks.getRun).toHaveBeenCalledTimes(2))
    resolveB(runB)
    await openingB
    resolveA(runA)
    await openingA

    expect(center.selectedHistoryRun.value?.id).toBe('run-b')
    expect(videoMocks.createContentURL).not.toHaveBeenCalled()
    unmount()
  })
})

describe('useAppCenter stopRun', () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    Object.values(gatewayMocks).forEach((mock) => mock.mockReset())
    keyMocks.getById.mockReset().mockResolvedValue(mediaKey)
    keyMocks.list.mockReset().mockResolvedValue({ items: [mediaKey] })
    apiMocks.listRuns.mockResolvedValue({ items: [] })
  })

  it('keeps a checkpointed video run resumable when stopping local polling', async () => {
    const { center, unmount } = mountComposable()
    center.activeRun.value = createRun({
      next_action: {
        step_key: 'media',
        executor: 'video_generation',
        credential_role: 'media',
        api_key_id: 9,
        session_id: 'app:run-1',
        path: '/v1/videos/video-123',
        method: 'GET',
        content_type: 'application/json',
        request: {},
        body: {},
        gateway_request_id: 'video-123',
      },
    })
    center.isRunning.value = true

    await center.stopRun()

    expect(apiMocks.cancelRun).not.toHaveBeenCalled()
    expect(center.activeRun.value?.status).toBe('running')
    expect(center.isRunning.value).toBe(false)
    expect(center.successMessage.value).toContain('稍后继续查询')
    expect(apiMocks.listRuns).toHaveBeenCalledOnce()
    unmount()
  })

  it('cancels a non-checkpointed run instead of leaving it running', async () => {
    const { center, unmount } = mountComposable()
    const cancelled = createRun({ status: 'cancelled' })
    apiMocks.cancelRun.mockResolvedValue(cancelled)
    center.activeRun.value = createRun()
    center.isRunning.value = true

    await center.stopRun()

    expect(apiMocks.cancelRun).toHaveBeenCalledWith('run-1')
    expect(center.activeRun.value?.status).toBe('cancelled')
    expect(center.isRunning.value).toBe(false)
    expect(center.successMessage.value).toContain('取消')
    expect(apiMocks.listRuns).toHaveBeenCalledOnce()
    unmount()
  })

  it('treats the top-level cancel command as stop-waiting after video submission', async () => {
    const { center, unmount } = mountComposable()
    center.activeRun.value = videoRun('processing')
    center.isRunning.value = true

    await center.cancelRun()

    expect(apiMocks.cancelRun).not.toHaveBeenCalled()
    expect(center.activeRun.value?.status).toBe('running')
    expect(center.isRunning.value).toBe(false)
    expect(center.successMessage.value).toContain('稍后继续查询')
    unmount()
  })

  it('keeps a checkpointed image run resumable when stopping local polling', async () => {
    const { center, unmount } = mountComposable()
    center.activeRun.value = imageRun('processing', { next_action: imageAction('image-123') })
    center.isRunning.value = true

    await center.stopRun()

    expect(apiMocks.cancelRun).not.toHaveBeenCalled()
    expect(center.activeRun.value?.status).toBe('running')
    expect(center.successMessage.value).toContain('图片任务已提交')
    unmount()
  })
})

describe('useAppCenter resumeRun', () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    Object.values(gatewayMocks).forEach((mock) => mock.mockReset())
    keyMocks.getById.mockReset().mockResolvedValue(mediaKey)
    keyMocks.list.mockReset().mockResolvedValue({ items: [mediaKey] })
    apiMocks.listRuns.mockResolvedValue({ items: [] })
  })

  it.each(['failed', 'expired'] as const)('does not retry a terminal %s video task', async (status) => {
    const { center, unmount } = mountComposable()
    const run = videoRun(status)
    apiMocks.getRun.mockResolvedValue(run)

    await center.resumeRun(run)

    expect(apiMocks.retryRun).not.toHaveBeenCalled()
    expect(gatewayMocks.executeMediaAction).not.toHaveBeenCalled()
    expect(center.error.value).toContain('没有可继续查询')
    unmount()
  })

  it('retries a resumable failed run and reuses its checkpointed request ID', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const failedRun = videoRun('processing', { status: 'failed' })
    const action = videoAction('video-123')
    const resumedRun = videoRun('processing', { status: 'running', next_action: action })
    const completedRun = videoRun('processing', {
      status: 'completed',
      output: { request_id: 'video-123', status: 'completed', video_url: 'https://cdn.test/video.mp4' },
      next_action: null,
    })
    apiMocks.getRun.mockResolvedValue(failedRun)
    apiMocks.retryRun.mockResolvedValue(resumedRun)
    gatewayMocks.executeMediaAction.mockResolvedValue({
      output: completedRun.output,
      gatewayRequestId: 'video-123',
      localResult: {
        type: 'video',
        requestId: 'video-123',
        videoUrl: 'https://cdn.test/video.mp4',
        videoStatus: 'completed',
      },
    })
    apiMocks.completeStep.mockResolvedValue(completedRun)

    await center.resumeRun(failedRun)

    expect(apiMocks.retryRun).toHaveBeenCalledWith('run-1', expect.any(AbortSignal))
    expect(gatewayMocks.executeMediaAction).toHaveBeenCalledOnce()
    expect(gatewayMocks.executeMediaAction.mock.calls[0]?.[1]).toEqual(expect.objectContaining({
      gateway_request_id: 'video-123',
    }))
    expect(center.successMessage.value).toContain('已完成')
    unmount()
  })

  it('resumes a checkpointed image task from history without creating another task', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const pendingRun = imageRun('processing', { next_action: imageAction('image-123') })
    const completedRun = imageRun('processing', {
      status: 'completed',
      output: { count: 1, images: [{ url: 'https://cdn.test/image.png' }] },
      next_action: null,
    })
    apiMocks.getRun.mockResolvedValue(pendingRun)
    gatewayMocks.executeMediaAction.mockResolvedValue({
      output: completedRun.output,
      gatewayRequestId: 'image-123',
      localResult: {
        type: 'images',
        images: [{ url: 'https://cdn.test/image.png', revisedPrompt: '' }],
      },
    })
    apiMocks.completeStep.mockResolvedValue(completedRun)

    await center.resumeRun(pendingRun)

    expect(apiMocks.retryRun).not.toHaveBeenCalled()
    expect(gatewayMocks.executeMediaAction).toHaveBeenCalledOnce()
    expect(gatewayMocks.executeMediaAction.mock.calls[0]?.[1]).toEqual(expect.objectContaining({
      gateway_request_id: 'image-123',
    }))
    expect(apiMocks.completeStep).toHaveBeenCalledWith(
      'run-1',
      'media',
      expect.objectContaining({ gateway_request_id: 'image-123' }),
      expect.any(AbortSignal),
    )
    expect(center.localResult.value?.images?.[0]?.url).toBe('https://cdn.test/image.png')
    expect(center.successMessage.value).toContain('已完成')
    unmount()
  })
})

describe('useAppCenter terminal checkpoint persistence', () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    Object.values(gatewayMocks).forEach((mock) => mock.mockReset())
    keyMocks.getById.mockReset().mockResolvedValue(mediaKey)
    keyMocks.list.mockReset().mockResolvedValue({ items: [mediaKey] })
    apiMocks.listRuns.mockResolvedValue({ items: [] })
  })

  it.each(['failed', 'expired'] as const)('persists a structured %s result instead of leaving it resumable', async (status) => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const startedRun = createRun({
      media_api_key_id: 9,
      next_action: videoAction(),
      steps: [{ step_key: 'media', status: 'pending', executor: 'video_generation' }],
    })
    const terminalRun = videoRun(status)
    const terminalError = Object.assign(new Error(`video ${status}`), {
      name: 'VideoContinuationError',
      requestId: 'video-123',
      status,
      checkpointed: true,
    })
    apiMocks.startRun.mockResolvedValue(startedRun)
    gatewayMocks.executeMediaAction.mockRejectedValue(terminalError)
    apiMocks.failStep.mockResolvedValue(terminalRun)

    await center.runApplication({ values: { mode: 'text_to_video' }, files: [] })

    expect(apiMocks.failStep).toHaveBeenCalledWith(
      'run-1',
      'media',
      {
        error: `video ${status}`,
        output: { request_id: 'video-123', status },
        gateway_request_id: 'video-123',
      },
      expect.any(AbortSignal),
    )
    expect(center.activeRun.value).toEqual(terminalRun)
    expect(center.error.value).toContain(`video ${status}`)
    unmount()
  })

  it('checkpoints an image task before completing its application step', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const startedRun = createRun({
      media_api_key_id: 9,
      next_action: imageAction(),
      steps: [{ step_key: 'media', status: 'pending', executor: 'image_generation' }],
    })
    const checkpointedRun = imageRun('processing', { next_action: imageAction('image-123') })
    const completedRun = imageRun('processing', {
      status: 'completed',
      next_action: null,
      output: { count: 1, images: [{ url: 'https://cdn.test/image.png' }] },
    })
    apiMocks.startRun.mockResolvedValue(startedRun)
    apiMocks.checkpointStep.mockResolvedValue(checkpointedRun)
    gatewayMocks.executeMediaAction.mockImplementation(async (...args: unknown[]) => {
      const checkpoint = args[6] as ((taskId: string) => Promise<void>)
      await checkpoint('image-123')
      return {
        output: completedRun.output,
        gatewayRequestId: 'image-123',
        localResult: {
          type: 'images',
          images: [{ url: 'https://cdn.test/image.png', revisedPrompt: '' }],
        },
      }
    })
    apiMocks.completeStep.mockResolvedValue(completedRun)

    await center.runApplication({ values: { mode: 'text_to_image' }, files: [] })

    expect(apiMocks.checkpointStep).toHaveBeenCalledWith(
      'run-1',
      'media',
      {
        output: { task_id: 'image-123', status: 'processing' },
        gateway_request_id: 'image-123',
      },
    )
    expect(apiMocks.completeStep).toHaveBeenCalledWith(
      'run-1',
      'media',
      {
        output: completedRun.output,
        gateway_request_id: 'image-123',
      },
      expect.any(AbortSignal),
    )
    expect(center.activeRun.value).toEqual(completedRun)
    expect(center.successMessage.value).toContain('运行完成')
    unmount()
  })

  it('keeps a checkpointed image run processing when polling is interrupted', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const startedRun = createRun({
      media_api_key_id: 9,
      next_action: imageAction(),
      steps: [{ step_key: 'media', status: 'pending', executor: 'image_generation' }],
    })
    const checkpointedRun = imageRun('processing', { next_action: imageAction('image-123') })
    apiMocks.startRun.mockResolvedValue(startedRun)
    apiMocks.checkpointStep.mockResolvedValue(checkpointedRun)
    gatewayMocks.executeMediaAction.mockImplementation(async (...args: unknown[]) => {
      const checkpoint = args[6] as ((taskId: string) => Promise<void>)
      await checkpoint('image-123')
      throw Object.assign(new Error('temporary image polling failure'), {
        name: 'ImageContinuationError',
        taskId: 'image-123',
        status: 'processing',
        checkpointed: true,
      })
    })

    await center.runApplication({ values: { mode: 'text_to_image' }, files: [] })

    expect(apiMocks.failStep).not.toHaveBeenCalled()
    expect(center.activeRun.value).toEqual(checkpointedRun)
    expect(center.error.value).toContain('temporary image polling failure')
    unmount()
  })

  it('persists a terminal failed image task instead of leaving it resumable', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const startedRun = createRun({
      media_api_key_id: 9,
      next_action: imageAction(),
      steps: [{ step_key: 'media', status: 'pending', executor: 'image_generation' }],
    })
    const failedRun = imageRun('failed')
    apiMocks.startRun.mockResolvedValue(startedRun)
    gatewayMocks.executeMediaAction.mockRejectedValue(Object.assign(new Error('image rejected'), {
      name: 'ImageContinuationError',
      taskId: 'image-123',
      status: 'failed',
      checkpointed: true,
    }))
    apiMocks.failStep.mockResolvedValue(failedRun)

    await center.runApplication({ values: { mode: 'text_to_image' }, files: [] })

    expect(apiMocks.failStep).toHaveBeenCalledWith(
      'run-1',
      'media',
      {
        error: 'image rejected',
        output: { task_id: 'image-123', status: 'failed' },
        gateway_request_id: 'image-123',
      },
      expect.any(AbortSignal),
    )
    expect(center.activeRun.value).toEqual(failedRun)
    unmount()
  })

  it('never cancels the database run during the submit-to-checkpoint race', async () => {
    const { center, unmount } = mountComposable()
    await center.loadKeys()
    const startedRun = createRun({
      media_api_key_id: 9,
      next_action: videoAction(),
      steps: [{ step_key: 'media', status: 'pending', executor: 'video_generation' }],
    })
    let resolveCheckpoint!: (run: ApplicationRun) => void
    apiMocks.startRun.mockResolvedValue(startedRun)
    apiMocks.checkpointStep.mockImplementation(() => new Promise<ApplicationRun>((resolve) => {
      resolveCheckpoint = resolve
    }))
    gatewayMocks.executeMediaAction.mockImplementation(async (...args: unknown[]) => {
      const checkpoint = args[5] as ((requestId: string) => Promise<void>)
      await checkpoint('video-race')
      throw new DOMException('The operation was aborted.', 'AbortError')
    })

    const running = center.runApplication({ values: { mode: 'text_to_video' }, files: [] })
    await vi.waitFor(() => expect(apiMocks.checkpointStep).toHaveBeenCalledOnce())
    await center.cancelRun()

    expect(apiMocks.cancelRun).not.toHaveBeenCalled()
    expect(center.successMessage.value).toContain('稍后继续查询')

    resolveCheckpoint(videoRun('processing', {
      next_action: videoAction('video-race'),
      steps: [{
        step_key: 'media',
        status: 'running',
        executor: 'video_generation',
        gateway_request_id: 'video-race',
        output: { request_id: 'video-race', status: 'processing' },
      }],
    }))
    await running
    unmount()
  })
})
