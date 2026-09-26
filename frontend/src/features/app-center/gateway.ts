import { buildGatewayUrl } from '@/api/client'
import {
  imageGenerationAPI,
  isAsyncImageGatewayUnavailableError,
  isAsyncImageTaskError,
  type AsyncImageSubmissionPath,
  type ImageSubmissionPath,
} from '@/api/imageGeneration'
import {
  videoGenerationAPI,
  type VideoSubmissionPath,
  type VideoGenerationStatus,
} from '@/api/videoGeneration'

import type {
  ApplicationNextAction,
  LocalMediaResult,
} from './types'
import {
  isFailedVideoStatus,
  isTerminalVideoStatus,
  normalizeVideoStatus,
  type VideoTaskStatus,
} from './videoStatus'

const VIDEO_POLL_INTERVAL_MS = 5000
const VIDEO_POLL_TIMEOUT_MS = 20 * 60 * 1000

export interface GatewayStepResult {
  output: unknown
  gatewayRequestId?: string
  localResult?: LocalMediaResult
}

export class VideoContinuationError extends Error {
  constructor(
    message: string,
    readonly requestId: string,
    readonly status: VideoTaskStatus,
    readonly checkpointed: boolean,
  ) {
    super(message)
    this.name = 'VideoContinuationError'
  }
}

export function isVideoContinuationError(error: unknown): error is VideoContinuationError {
  return error instanceof VideoContinuationError
}

export class ImageContinuationError extends Error {
  constructor(
    message: string,
    readonly taskId: string,
    readonly status: 'processing' | 'failed',
    readonly checkpointed: boolean,
  ) {
    super(message)
    this.name = 'ImageContinuationError'
  }
}

export function isImageContinuationError(error: unknown): error is ImageContinuationError {
  return error instanceof ImageContinuationError
}

interface ChatCompletionPayload {
  choices?: Array<{ message?: { content?: unknown } }>
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function boundedText(value: unknown, max = 8000): string {
  const text = typeof value === 'string' ? value : ''
  return text.length > max ? `${text.slice(0, max)}...` : text
}

function boundedValue(value: unknown, depth = 0): unknown {
  if (depth >= 6) return '[depth limited]'
  if (typeof value === 'string') return boundedText(value)
  if (Array.isArray(value)) return value.slice(0, 50).map((item) => boundedValue(item, depth + 1))
  if (!isRecord(value)) return value
  return Object.fromEntries(
    Object.entries(value)
      .slice(0, 80)
      .map(([key, item]) => [key, boundedValue(item, depth + 1)]),
  )
}

function isAbortError(error: unknown): boolean {
  return (error instanceof DOMException && error.name === 'AbortError')
    || (isRecord(error) && (error.name === 'AbortError' || error.code === 'ERR_CANCELED'))
}

async function responsePayload(response: Response, fallback: string): Promise<unknown> {
  const text = await response.text()
  let payload: unknown
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = { message: text }
  }
  if (!response.ok) {
    const record = isRecord(payload) ? payload : {}
    const nested = isRecord(record.error) ? record.error : {}
    const message = boundedText(record.message || nested.message, 600)
    throw new Error(message || `${fallback}（HTTP ${response.status}）`)
  }
  return payload
}

function actionPath(action: ApplicationNextAction): string {
  const expected: Record<ApplicationNextAction['executor'], string> = {
    chat_completions: '/v1/chat/completions',
    image_generation: '/v1/images/generations',
    image_edit: '/v1/images/edits',
    video_generation: '/v1/videos/generations',
    video_edit: '/v1/videos/edits',
    video_extension: '/v1/videos/extensions',
  }
  const normalized = action.path.startsWith('/') ? action.path : `/${action.path}`
  if (normalized !== expected[action.executor]) {
    throw new Error(`运行步骤 ${action.step_key} 的网关路径与执行器不匹配。`)
  }
  return normalized
}

async function requestChatCompletion(
  apiKey: string,
  action: ApplicationNextAction,
  request: Record<string, unknown>,
  signal: AbortSignal,
): Promise<string> {
  const response = await fetch(buildGatewayUrl(actionPath(action)), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
      ...(action.session_id.trim() ? { 'X-Session-Id': action.session_id.trim() } : {}),
    },
    body: JSON.stringify({ ...request, stream: false }),
    signal,
  })
  const payload = await responsePayload(response, '策划模型调用失败') as ChatCompletionPayload
  const content = payload.choices?.[0]?.message?.content
  if (typeof content !== 'string' || !content.trim()) {
    throw new Error('策划模型没有返回可解析的内容。')
  }
  return content.trim()
}

export function parsePlannerJSON(content: string): unknown {
  const trimmed = content.trim()
  const fenced = trimmed.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/i)?.[1]?.trim() || trimmed
  try {
    return JSON.parse(fenced)
  } catch {
    const objectStart = fenced.indexOf('{')
    const objectEnd = fenced.lastIndexOf('}')
    const arrayStart = fenced.indexOf('[')
    const arrayEnd = fenced.lastIndexOf(']')
    const candidates = [
      objectStart >= 0 && objectEnd > objectStart ? fenced.slice(objectStart, objectEnd + 1) : '',
      arrayStart >= 0 && arrayEnd > arrayStart ? fenced.slice(arrayStart, arrayEnd + 1) : '',
    ].filter(Boolean)
    for (const candidate of candidates) {
      try {
        return JSON.parse(candidate)
      } catch {
        // Try the next bounded JSON candidate.
      }
    }
    throw new Error('策划模型返回的内容不是有效 JSON。')
  }
}

function validatePlannerPlan(value: unknown): Record<string, unknown> {
  if (!isRecord(value)) throw new Error('策划模型必须返回一个 JSON 对象。')
  const allowedKeys = new Set(['final_prompt', 'negative_prompt', 'rationale'])
  if (Object.keys(value).some((key) => !allowedKeys.has(key))) {
    throw new Error('策划模型返回的 JSON 包含未定义字段。')
  }

  if (typeof value.final_prompt !== 'string') {
    throw new Error('策划模型返回的 JSON 缺少 final_prompt。')
  }
  const finalPrompt = value.final_prompt.trim()
  if (!finalPrompt) throw new Error('策划模型返回的 JSON 缺少 final_prompt。')
  if (new TextEncoder().encode(finalPrompt).length > 8000) {
    throw new Error('策划模型返回的 final_prompt 超过 8000 字节。')
  }

  if (!Object.prototype.hasOwnProperty.call(value, 'negative_prompt')) {
    throw new Error('策划模型返回的 JSON 缺少 negative_prompt。')
  }
  if (value.negative_prompt !== null && typeof value.negative_prompt !== 'string') {
    throw new Error('策划模型返回的 negative_prompt 必须是字符串或 null。')
  }
  const negativePrompt = typeof value.negative_prompt === 'string' ? value.negative_prompt.trim() : null
  if (negativePrompt !== null && negativePrompt.length > 2000) {
    throw new Error('策划模型返回的 negative_prompt 超过 2000 个字符。')
  }

  if (typeof value.rationale !== 'string') {
    throw new Error('策划模型返回的 JSON 缺少 rationale。')
  }
  const rationale = value.rationale.trim()
  if (rationale.length > 2000) {
    throw new Error('策划模型返回的 rationale 超过 2000 个字符。')
  }

  return {
    final_prompt: finalPrompt,
    negative_prompt: negativePrompt,
    rationale,
  }
}

export async function executePlannerAction(
  apiKey: string,
  action: ApplicationNextAction,
  signal: AbortSignal,
): Promise<GatewayStepResult> {
  const firstContent = await requestChatCompletion(apiKey, action, action.request, signal)
  let parsed: Record<string, unknown>
  try {
    parsed = validatePlannerPlan(parsePlannerJSON(firstContent))
  } catch (error) {
    const instruction = action.repair_instruction?.trim()
    if (!instruction) throw error
    const messages = Array.isArray(action.request.messages) ? [...action.request.messages] : []
    const repairedContent = await requestChatCompletion(apiKey, action, {
      ...action.request,
      messages: [
        ...messages,
        { role: 'assistant', content: firstContent },
        { role: 'user', content: instruction },
      ],
    }, signal)
    parsed = validatePlannerPlan(parsePlannerJSON(repairedContent))
  }

  const output = boundedValue(parsed)
  return {
    output,
    localResult: { type: 'planner', plannerOutput: output },
  }
}

function attachLocalFiles(
  action: ApplicationNextAction,
  files: File[],
  mode = '',
): Record<string, unknown> {
  const request = action.request
  const imageFiles = files.filter((file) => file.type.startsWith('image/'))
  const videoFile = files.find((file) => file.type.startsWith('video/'))
  const usesReferenceImages = mode === 'reference_to_video'
    || (!mode && action.executor === 'video_generation' && request.model === 'grok-imagine-video')
  const usesSourceImage = mode === 'image_to_video'
    || (!mode && action.executor === 'video_generation' && !usesReferenceImages)
  const usesSourceVideo = mode === 'video_edit'
    || mode === 'video_extend'
    || action.executor === 'video_edit'
    || action.executor === 'video_extension'
  return {
    ...request,
    ...(usesSourceImage && imageFiles[0] ? { image: imageFiles[0] } : {}),
    ...(usesReferenceImages && imageFiles.length ? { reference_images: imageFiles.slice(0, 8) } : {}),
    ...(usesSourceVideo && videoFile ? { video: videoFile } : {}),
  }
}

export function persistentMediaURL(raw: string): string {
  const value = raw.trim()
  if (/^https?:\/\//i.test(value)) return value
  return value.startsWith('/') ? buildGatewayUrl(value) : ''
}

function imageGatewayResult(
  images: Array<{ url: string; revisedPrompt: string }>,
  taskId = '',
): GatewayStepResult {
  const outputImages = images.slice(0, 8).map((image) => ({
    url: image.url.startsWith('data:') ? '' : image.url,
    revised_prompt: boundedText(image.revisedPrompt, 600),
  }))
  return {
    output: { count: images.length, images: outputImages },
    ...(taskId ? { gatewayRequestId: taskId } : {}),
    localResult: { type: 'images', images },
  }
}

async function waitForVideo(
  apiKey: string,
  requestId: string,
  sessionId: string,
  signal: AbortSignal,
): Promise<VideoGenerationStatus> {
  const startedAt = Date.now()
  while (true) {
    const status = await videoGenerationAPI.status(apiKey, requestId, signal, { sessionId })
    if (isTerminalVideoStatus(status.status)) return status
    if (Date.now() - startedAt >= VIDEO_POLL_TIMEOUT_MS) {
      throw new Error('视频仍在处理中，可稍后从运行历史继续查看。')
    }
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(resolve, VIDEO_POLL_INTERVAL_MS)
      signal.addEventListener('abort', () => {
        clearTimeout(timer)
        reject(new DOMException('The operation was aborted.', 'AbortError'))
      }, { once: true })
    })
  }
}

export async function executeMediaAction(
  apiKey: string,
  action: ApplicationNextAction,
  files: File[],
  signal: AbortSignal,
  mode = '',
  onVideoSubmitted?: (requestId: string) => Promise<void>,
  onImageTaskSubmitted?: (taskId: string) => Promise<void>,
): Promise<GatewayStepResult> {
  const path = actionPath(action)
  if (action.executor === 'image_generation' || action.executor === 'image_edit') {
    const references = action.executor === 'image_edit'
      ? files.filter((file) => file.type.startsWith('image/'))
      : []
    const requestOptions = {
      signal,
      sessionId: action.session_id,
      referenceImages: references,
    }
    let taskId = action.gateway_request_id?.trim() || ''
    let checkpointed = !!taskId

    if (!taskId) {
      let submission: Awaited<ReturnType<typeof imageGenerationAPI.submitAsync>>
      try {
        submission = await imageGenerationAPI.submitAsync(
          apiKey,
          `${path}/async` as AsyncImageSubmissionPath,
          action.request,
          requestOptions,
        )
      } catch (submitError) {
        if (!isAsyncImageGatewayUnavailableError(submitError)) throw submitError
        const images = await imageGenerationAPI.submitGateway(
          apiKey,
          path as ImageSubmissionPath,
          action.request,
          requestOptions,
        )
        return imageGatewayResult(images)
      }

      taskId = submission.taskId
      if (onImageTaskSubmitted) {
        try {
          await onImageTaskSubmitted(taskId)
          checkpointed = true
        } catch (checkpointError) {
          if (isAbortError(checkpointError)) throw checkpointError
          const message = checkpointError instanceof Error && checkpointError.message.trim()
            ? checkpointError.message
            : '图片任务已提交，但保存任务号失败。'
          throw new ImageContinuationError(message, taskId, 'processing', false)
        }
      }
    }

    let images: Awaited<ReturnType<typeof imageGenerationAPI.pollAsync>>
    try {
      images = await imageGenerationAPI.pollAsync(apiKey, taskId, {
        signal,
        sessionId: action.session_id,
      })
    } catch (pollError) {
      if (isAbortError(pollError)) throw pollError
      const status = isAsyncImageTaskError(pollError) && pollError.status === 'failed'
        ? 'failed'
        : 'processing'
      const message = pollError instanceof Error && pollError.message.trim()
        ? pollError.message
        : status === 'failed' ? '图片任务执行失败。' : '图片仍在处理中，可稍后从运行历史继续查看。'
      throw new ImageContinuationError(message, taskId, status, checkpointed)
    }
    return imageGatewayResult(images, taskId)
  }

  let requestId = action.gateway_request_id?.trim() || ''
  let checkpointed = !!requestId
  if (!requestId) {
    const submission = await videoGenerationAPI.submitGateway(
      apiKey,
      path as VideoSubmissionPath,
      attachLocalFiles(action, files, mode),
      { signal, sessionId: action.session_id },
    )
    requestId = submission.requestId
    if (onVideoSubmitted) {
      try {
        await onVideoSubmitted(requestId)
        checkpointed = true
      } catch (checkpointError) {
        if (isAbortError(checkpointError)) throw checkpointError
        const message = checkpointError instanceof Error && checkpointError.message.trim()
          ? checkpointError.message
          : '视频任务已提交，但保存任务号失败。'
        throw new VideoContinuationError(message, requestId, 'submitted', false)
      }
    }
  }

  let status: VideoGenerationStatus
  try {
    status = await waitForVideo(apiKey, requestId, action.session_id, signal)
  } catch (pollError) {
    if (isAbortError(pollError)) throw pollError
    const message = pollError instanceof Error && pollError.message.trim()
      ? pollError.message
      : '视频仍在上游处理中，可从运行历史继续查询。'
    throw new VideoContinuationError(message, requestId, 'processing', checkpointed)
  }
  const normalizedStatus = normalizeVideoStatus(status.status) || 'processing'
  if (isFailedVideoStatus(normalizedStatus)) {
    const fallback = normalizedStatus === 'expired'
      ? '视频任务已过期。'
      : normalizedStatus === 'cancelled'
        ? '视频任务已取消。'
        : '视频任务执行失败。'
    throw new VideoContinuationError(
      status.errorMessage || fallback,
      requestId,
      normalizedStatus,
      checkpointed,
    )
  }

  const persistedVideoURL = persistentMediaURL(status.videoUrl)
  let localVideoUrl = ''
  try {
    localVideoUrl = await videoGenerationAPI.createContentURL(
      apiKey,
      requestId,
      signal,
      { sessionId: action.session_id },
    )
  } catch {
    if (/^https?:\/\//i.test(status.videoUrl.trim())) localVideoUrl = status.videoUrl.trim()
  }
  if (!localVideoUrl) {
    throw new VideoContinuationError(
      '视频已生成，但内容暂不可读取，可稍后继续查询。',
      requestId,
      'processing',
      checkpointed,
    )
  }

  return {
    output: {
      request_id: requestId,
      status: normalizedStatus,
      ...(persistedVideoURL ? { video_url: persistedVideoURL } : {}),
    },
    gatewayRequestId: requestId,
    localResult: {
      type: 'video',
      videoUrl: localVideoUrl,
      requestId,
      videoStatus: normalizedStatus,
    },
  }
}
