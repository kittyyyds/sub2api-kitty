import { buildGatewayUrl } from './url'

export const IMAGE_REFERENCE_LIMIT = 5

export const KITTY_IMAGE_MODEL = 'gpt-image-2' as const

export const GROK_IMAGE_MODEL = 'grok-imagine-image' as const
export const GROK_IMAGE_20_MODEL = 'grok-imagine-image-2.0' as const
export const GROK_IMAGE_QUALITY_MODEL = 'grok-imagine-image-quality' as const

export const GROK_IMAGE_MODELS = [
  GROK_IMAGE_MODEL,
  GROK_IMAGE_20_MODEL,
  GROK_IMAGE_QUALITY_MODEL,
] as const
export const GROK_IMAGE_RESOLUTIONS = ['1k', '2k'] as const
export const GROK_IMAGE_ASPECT_RATIOS = [
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
] as const

export const GROK_IMAGE_COUNT_OPTIONS = [1, 2, 3, 4] as const
export const OPENAI_IMAGE_COUNT_OPTIONS = [1, 2, 3, 4, 5, 6, 7, 8] as const

export function imageGenerationCountOptions(platform: string): readonly number[] {
  return platform.trim().toLowerCase() === 'grok'
    ? GROK_IMAGE_COUNT_OPTIONS
    : OPENAI_IMAGE_COUNT_OPTIONS
}

export function normalizeImageGenerationCount(value: unknown, platform: string): number {
  const options = imageGenerationCountOptions(platform)
  const count = Math.round(Number(value)) || options[0]
  return Math.max(options[0], Math.min(options[options.length - 1], count))
}

export const KITTY_IMAGE_MODELS = [
  {
    value: '1k',
    model: KITTY_IMAGE_MODEL,
    label: '1K 标准',
    description: '适合日常配图、封面草稿和快速预览。',
  },
  {
    value: '2k',
    model: KITTY_IMAGE_MODEL,
    label: '2K 标准',
    description: '适合产品图、海报和需要更多细节的画面。',
  },
  {
    value: '4k',
    model: KITTY_IMAGE_MODEL,
    label: '4K 标准',
    description: '适合大尺寸物料、高清展示和精修预览。',
  },
] as const

export type KittyImagePreset = (typeof KITTY_IMAGE_MODELS)[number]['value']
export type GrokImageModel = (typeof GROK_IMAGE_MODELS)[number]
export type GrokImageResolution = (typeof GROK_IMAGE_RESOLUTIONS)[number]
export type GrokImageAspectRatio = (typeof GROK_IMAGE_ASPECT_RATIOS)[number]
export type KittyImageModel = (typeof KITTY_IMAGE_MODELS)[number]['model'] | GrokImageModel
export type KittyImageQuality = 'auto' | 'low' | 'medium' | 'high'

export interface ImageGenerationRequest {
  model: KittyImageModel
  prompt: string
  n: number
  size: string
  quality?: KittyImageQuality
  aspect_ratio?: string
  resolution?: GrokImageResolution
}

export interface ImageGatewayOptions {
  signal?: AbortSignal
  sessionId?: string
  referenceImages?: File[]
}

export type ImageSubmissionPath = '/v1/images/generations' | '/v1/images/edits'
export type AsyncImageSubmissionPath = '/v1/images/generations/async' | '/v1/images/edits/async'

export interface AsyncImageTaskSubmission {
  taskId: string
  status: string
  pollUrl: string
}

export interface AsyncImagePollOptions extends Pick<ImageGatewayOptions, 'signal' | 'sessionId'> {
  pollIntervalMs?: number
}

export class AsyncImageGatewayUnavailableError extends Error {
  readonly code = 'ASYNC_IMAGE_GATEWAY_UNAVAILABLE'
  readonly status = 404

  constructor(message = '异步图片任务未启用。') {
    super(message)
    this.name = 'AsyncImageGatewayUnavailableError'
  }
}

export class AsyncImageTaskError extends Error {
  constructor(
    message: string,
    readonly taskId: string,
    readonly status: string,
  ) {
    super(message)
    this.name = 'AsyncImageTaskError'
  }
}

export function isAsyncImageTaskError(error: unknown): error is AsyncImageTaskError {
  return error instanceof AsyncImageTaskError
    || (isRecord(error)
      && error.name === 'AsyncImageTaskError'
      && typeof error.taskId === 'string'
      && typeof error.status === 'string')
}

export interface GeneratedImage {
  url: string
  revisedPrompt: string
}

interface ImageResponseItem {
  url?: unknown
  image_url?: unknown
  downloadUrl?: unknown
  download_url?: unknown
  b64_json?: unknown
  revised_prompt?: unknown
  revisedPrompt?: unknown
  prompt?: unknown
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function imageItems(payload: unknown): unknown[] {
  if (Array.isArray(payload)) return payload
  if (!isRecord(payload)) return []

  let value: unknown = payload.data ?? payload.images ?? payload.output ?? payload.results ?? payload
  if (isRecord(value) && Array.isArray(value.data)) value = value.data
  return Array.isArray(value) ? value : [value]
}

export function extractGeneratedImages(payload: unknown): GeneratedImage[] {
  return imageItems(payload).flatMap((rawItem) => {
    const item: ImageResponseItem = typeof rawItem === 'string'
      ? { url: rawItem }
      : isRecord(rawItem)
        ? rawItem
        : {}
    const base64 = asText(item.b64_json)
    const source = asText(item.url)
      || asText(item.image_url)
      || asText(item.downloadUrl)
      || asText(item.download_url)
      || base64

    if (!source) return []

    return [{
      url: base64 && source === base64 && !base64.startsWith('data:')
        ? `data:image/png;base64,${base64}`
        : source,
      revisedPrompt: asText(item.revised_prompt)
        || asText(item.revisedPrompt)
        || asText(item.prompt),
    }]
  })
}

function responseErrorMessage(payload: unknown, fallback: string): string {
  if (!isRecord(payload)) return fallback
  const nestedError = isRecord(payload.error) ? payload.error : null
  const message = asText(payload.message)
    || asText(payload.detail)
    || asText(nestedError?.message)
    || fallback
  return message.length > 600 ? `${message.slice(0, 600)}...` : message
}

async function readResponsePayload(response: Response): Promise<{ payload: unknown; text: string }> {
  const text = await response.text()
  let payload: unknown = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = { message: text }
  }
  return { payload, text }
}

function imageResponseError(response: Response, payload: unknown, text: string): Error {
  const contentType = response.headers.get('content-type') || ''
  const returnedHTML = /text\/html/i.test(contentType)
    || /^\s*<!doctype\s+html/i.test(text)
    || /^\s*<html[\s>]/i.test(text)
  if (returnedHTML) {
    return new Error(response.status === 504 || response.status === 524
      ? '图片生成等待超时，请稍后重试；当前请求可能仍在上游处理。'
      : `图片接口暂时不可用（HTTP ${response.status}），请稍后重试。`)
  }
  return new Error(responseErrorMessage(payload, `图片接口返回 HTTP ${response.status}。`))
}

async function parseImageResponse(response: Response): Promise<GeneratedImage[]> {
  const { payload, text } = await readResponsePayload(response)

  if (!response.ok) throw imageResponseError(response, payload, text)

  const images = extractGeneratedImages(payload)
  if (!images.length) {
    throw new Error('接口已返回，但没有解析到图片 URL 或 b64_json。')
  }
  return images
}

function abortReason(signal: AbortSignal): unknown {
  if (signal.reason !== undefined) return signal.reason
  return new DOMException('The operation was aborted.', 'AbortError')
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw abortReason(signal)
}

function waitForImageTaskPoll(delayMs: number, signal?: AbortSignal): Promise<void> {
  throwIfAborted(signal)
  if (delayMs <= 0) return Promise.resolve()

  return new Promise((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, delayMs)
    const onAbort = () => {
      globalThis.clearTimeout(timer)
      signal?.removeEventListener('abort', onAbort)
      reject(signal ? abortReason(signal) : new DOMException('The operation was aborted.', 'AbortError'))
    }
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

export function isAsyncImageGatewayUnavailableError(error: unknown): error is AsyncImageGatewayUnavailableError {
  return error instanceof AsyncImageGatewayUnavailableError
    || (isRecord(error)
      && error.code === 'ASYNC_IMAGE_GATEWAY_UNAVAILABLE'
      && error.status === 404)
}

function authorizationHeaders(apiKey: string, sessionId = ''): HeadersInit {
  if (!apiKey.trim()) throw new Error('未选择可用的调用 Key。')
  return {
    Authorization: `Bearer ${apiKey}`,
    ...(sessionId.trim() ? { 'X-Session-Id': sessionId.trim() } : {}),
  }
}

function generationRequestBody(request: ImageGenerationRequest): Omit<ImageGenerationRequest, 'quality'> | ImageGenerationRequest {
  if (request.model === KITTY_IMAGE_MODEL) return request
  const { quality: _quality, ...grokRequest } = request
  return grokRequest
}

export async function submitImageGatewayRequest(
  apiKey: string,
  path: ImageSubmissionPath,
  request: Record<string, unknown>,
  options: ImageGatewayOptions = {},
): Promise<GeneratedImage[]> {
  const headers = authorizationHeaders(apiKey, options.sessionId)
  if (path === '/v1/images/edits') {
    const references = (options.referenceImages || []).slice(0, IMAGE_REFERENCE_LIMIT)
    if (!references.length) throw new Error('图生图模式需要至少一张参考图。')

    const body = new FormData()
    Object.entries(request).forEach(([key, value]) => {
      if (value === undefined || value === null || value === '') return
      body.append(key, typeof value === 'object' ? JSON.stringify(value) : String(value))
    })
    references.forEach((file, index) => {
      body.append(index === 0 ? 'image' : 'image[]', file, file.name || `reference-${index + 1}.png`)
    })
    const response = await fetch(buildGatewayUrl(path), {
      method: 'POST',
      headers,
      body,
      signal: options.signal,
    })
    return parseImageResponse(response)
  }

  const response = await fetch(buildGatewayUrl(path), {
    method: 'POST',
    headers: { ...headers, 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: options.signal,
  })
  return parseImageResponse(response)
}

export async function submitAsyncImageGatewayRequest(
  apiKey: string,
  path: AsyncImageSubmissionPath,
  request: Record<string, unknown>,
  options: ImageGatewayOptions = {},
): Promise<AsyncImageTaskSubmission> {
  throwIfAborted(options.signal)
  const headers = authorizationHeaders(apiKey, options.sessionId)
  let response: Response

  if (path === '/v1/images/edits/async') {
    const references = (options.referenceImages || []).slice(0, IMAGE_REFERENCE_LIMIT)
    if (!references.length) throw new Error('图生图模式需要至少一张参考图。')

    const body = new FormData()
    Object.entries(request).forEach(([key, value]) => {
      if (value === undefined || value === null || value === '') return
      body.append(key, typeof value === 'object' ? JSON.stringify(value) : String(value))
    })
    references.forEach((file, index) => {
      body.append(index === 0 ? 'image' : 'image[]', file, file.name || `reference-${index + 1}.png`)
    })
    response = await fetch(buildGatewayUrl(path), {
      method: 'POST',
      headers,
      body,
      signal: options.signal,
    })
  } else {
    response = await fetch(buildGatewayUrl(path), {
      method: 'POST',
      headers: { ...headers, 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
      signal: options.signal,
    })
  }

  const { payload, text } = await readResponsePayload(response)
  if (!response.ok) {
    const error = imageResponseError(response, payload, text)
    if (response.status === 404) throw new AsyncImageGatewayUnavailableError(error.message)
    throw error
  }
  if (response.status !== 202 || !isRecord(payload)) {
    throw new Error(`异步图片接口返回了无效的提交响应（HTTP ${response.status}）。`)
  }

  const taskId = asText(payload.task_id)
  const status = asText(payload.status).toLowerCase()
  const pollUrl = asText(payload.poll_url)
  if (!taskId || !status || !pollUrl) {
    throw new Error('异步图片接口已接受请求，但没有返回完整的 task_id、status 和 poll_url。')
  }
  return { taskId, status, pollUrl }
}

export async function pollAsyncImageGatewayTask(
  apiKey: string,
  taskId: string,
  options: AsyncImagePollOptions = {},
): Promise<GeneratedImage[]> {
  const normalizedTaskId = taskId.trim()
  if (!normalizedTaskId) throw new Error('缺少异步图片任务 ID。')
  const configuredInterval = Number(options.pollIntervalMs)
  const pollIntervalMs = Number.isFinite(configuredInterval)
    ? Math.max(0, configuredInterval)
    : 3000

  while (true) {
    throwIfAborted(options.signal)
    const response = await fetch(buildGatewayUrl(`/v1/images/tasks/${encodeURIComponent(normalizedTaskId)}`), {
      method: 'GET',
      headers: authorizationHeaders(apiKey, options.sessionId),
      signal: options.signal,
    })
    const { payload, text } = await readResponsePayload(response)
    if (!response.ok) throw imageResponseError(response, payload, text)
    if (!isRecord(payload)) throw new Error('异步图片任务返回了无效响应。')

    const status = asText(payload.status).toLowerCase()
    if (status === 'completed') {
      const images = extractGeneratedImages(payload.result)
      if (!images.length) {
        throw new AsyncImageTaskError('图片任务已完成，但没有解析到图片 URL。', normalizedTaskId, status)
      }
      return images
    }
    if (status === 'failed') {
      throw new AsyncImageTaskError(
        responseErrorMessage(payload.error, '图片生成失败，请稍后重试。'),
        normalizedTaskId,
        status,
      )
    }
    if (status !== 'processing') {
      throw new Error(`异步图片任务返回了未知状态：${status || 'empty'}。`)
    }
    await waitForImageTaskPoll(pollIntervalMs, options.signal)
  }
}

export async function generateImage(
  apiKey: string,
  request: ImageGenerationRequest,
  signal?: AbortSignal
): Promise<GeneratedImage[]> {
  const response = await fetch(buildGatewayUrl('/v1/images/generations'), {
    method: 'POST',
    headers: {
      ...authorizationHeaders(apiKey),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(generationRequestBody(request)),
    signal,
  })
  return parseImageResponse(response)
}

export async function editImage(
  apiKey: string,
  request: ImageGenerationRequest,
  referenceFiles: File[],
  signal?: AbortSignal
): Promise<GeneratedImage[]> {
  if (!referenceFiles.length) throw new Error('图生图模式需要至少一张参考图。')

  const body = new FormData()
  body.append('model', request.model)
  body.append('prompt', request.prompt)
  body.append('n', String(request.n))
  body.append('size', request.size)
  if (request.quality) body.append('quality', request.quality)
  if (request.aspect_ratio) body.append('aspect_ratio', request.aspect_ratio)
  if (request.resolution) body.append('resolution', request.resolution)
  referenceFiles.slice(0, IMAGE_REFERENCE_LIMIT).forEach((file, index) => {
    body.append(index === 0 ? 'image' : 'image[]', file, file.name || `reference-${index + 1}.png`)
  })

  const response = await fetch(buildGatewayUrl('/v1/images/edits'), {
    method: 'POST',
    headers: authorizationHeaders(apiKey),
    body,
    signal,
  })
  return parseImageResponse(response)
}

export const imageGenerationAPI = {
  generate: generateImage,
  edit: editImage,
  submitGateway: submitImageGatewayRequest,
  submitAsync: submitAsyncImageGatewayRequest,
  pollAsync: pollAsyncImageGatewayTask,
}
