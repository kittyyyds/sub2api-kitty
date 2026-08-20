import { buildGatewayUrl } from './url'

export const GROK_VIDEO_MODEL = 'grok-imagine-video' as const
export const GROK_VIDEO_15_MODEL = 'grok-imagine-video-1.5' as const
export const GROK_VIDEO_MODELS = [GROK_VIDEO_MODEL, GROK_VIDEO_15_MODEL] as const
export const GROK_VIDEO_MAX_DURATION_SECONDS = 15
export const GROK_VIDEO_ASPECT_RATIOS = ['16:9', '1:1', '9:16', '3:2', '2:3'] as const

export type GrokVideoModel = (typeof GROK_VIDEO_MODELS)[number]
export type GrokVideoResolution = '480p' | '720p' | '1080p'
export type GrokVideoAspectRatio = (typeof GROK_VIDEO_ASPECT_RATIOS)[number]
export type GrokVideoStatus = 'pending' | 'processing' | 'done' | 'failed' | 'expired' | (string & {})

export interface VideoGenerationRequest {
  model: GrokVideoModel
  prompt: string
  duration: number
  resolution: GrokVideoResolution
  aspectRatio?: string
  image?: File
  images?: File[]
}

export interface VideoGatewayOptions {
  signal?: AbortSignal
  sessionId?: string
}

export type VideoSubmissionPath =
  | '/v1/videos/generations'
  | '/v1/videos/edits'
  | '/v1/videos/extensions'

export interface VideoGenerationSubmission {
  requestId: string
}

export interface VideoGenerationStatus {
  requestId: string
  status: GrokVideoStatus
  videoUrl: string
  errorMessage: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function responseErrorMessage(payload: unknown, fallback: string): string {
  if (!isRecord(payload)) return fallback
  const nestedError = isRecord(payload.error) ? payload.error : null
  const nestedData = isRecord(payload.data) ? payload.data : null
  const dataError = isRecord(nestedData?.error) ? nestedData.error : null
  const message = asText(payload.message)
    || asText(payload.detail)
    || asText(payload.error)
    || asText(nestedError?.message)
    || asText(nestedData?.message)
    || asText(nestedData?.error)
    || asText(dataError?.message)
    || fallback
  return message.length > 600 ? `${message.slice(0, 600)}...` : message
}

async function parseJSONResponse(response: Response, fallback: string): Promise<unknown> {
  const text = await response.text()
  let payload: unknown = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = { message: text }
  }

  if (!response.ok) {
    const contentType = response.headers.get('content-type') || ''
    const returnedHTML = /text\/html/i.test(contentType)
      || /^\s*<!doctype\s+html/i.test(text)
      || /^\s*<html[\s>]/i.test(text)
    if (returnedHTML) {
      throw new Error(`视频接口暂时不可用（HTTP ${response.status}），请稍后重试。`)
    }
    throw new Error(responseErrorMessage(payload, `${fallback}（HTTP ${response.status}）。`))
  }

  return payload
}

function authorizationHeaders(apiKey: string, sessionId = ''): HeadersInit {
  if (!apiKey.trim()) throw new Error('未选择可用的调用 Key。')
  return {
    Authorization: `Bearer ${apiKey}`,
    ...(sessionId.trim() ? { 'X-Session-Id': sessionId.trim() } : {}),
  }
}

function normalizedRequestID(requestId: string): string {
  const value = requestId.trim()
  if (!value) throw new Error('视频任务缺少 request_id。')
  return value
}

function videoRequestPath(requestId: string, suffix = ''): string {
  return `/v1/videos/${encodeURIComponent(normalizedRequestID(requestId))}${suffix}`
}

function extractRequestID(payload: unknown): string {
  if (!isRecord(payload)) return ''
  const data = isRecord(payload.data) ? payload.data : null
  const video = isRecord(payload.video) ? payload.video : null
  return asText(payload.request_id)
    || asText(payload.id)
    || asText(data?.request_id)
    || asText(data?.id)
    || asText(video?.request_id)
    || asText(video?.id)
}

function validateGenerationRequest(request: VideoGenerationRequest): void {
  if (!request.prompt.trim()) throw new Error('请输入视频生成提示词。')
  if (!Number.isInteger(request.duration) || request.duration < 1 || request.duration > GROK_VIDEO_MAX_DURATION_SECONDS) {
    throw new Error(`视频时长必须是 1-${GROK_VIDEO_MAX_DURATION_SECONDS} 秒的整数。`)
  }

  if (!GROK_VIDEO_MODELS.includes(request.model)) {
    throw new Error('请选择受支持的 Grok 视频模型。')
  }
  if (request.aspectRatio?.trim() && !GROK_VIDEO_ASPECT_RATIOS.includes(request.aspectRatio.trim() as GrokVideoAspectRatio)) {
    throw new Error('请选择受支持的 Grok 视频画面比例。')
  }
  if (request.resolution === '1080p' && request.model !== GROK_VIDEO_15_MODEL) {
    throw new Error(`1080p 仅支持使用 ${GROK_VIDEO_15_MODEL}。`)
  }
}

export function fileToDataURL(file: File, signal?: AbortSignal): Promise<string> {
  if (signal?.aborted) return Promise.reject(new DOMException('The operation was aborted.', 'AbortError'))

  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    const cleanup = () => signal?.removeEventListener('abort', abort)
    const abort = () => reader.abort()

    reader.onload = () => {
      cleanup()
      if (typeof reader.result === 'string' && reader.result.startsWith('data:')) {
        resolve(reader.result)
        return
      }
      reject(new Error('无法读取视频参考图。'))
    }
    reader.onerror = () => {
      cleanup()
      reject(reader.error || new Error('无法读取视频参考图。'))
    }
    reader.onabort = () => {
      cleanup()
      reject(new DOMException('The operation was aborted.', 'AbortError'))
    }
    signal?.addEventListener('abort', abort, { once: true })
    reader.readAsDataURL(file)
  })
}

async function serializeVideoPayload(value: unknown, signal?: AbortSignal): Promise<unknown> {
  if (typeof File !== 'undefined' && value instanceof File) {
    return { url: await fileToDataURL(value, signal) }
  }
  if (Array.isArray(value)) {
    return Promise.all(value.map((item) => serializeVideoPayload(item, signal)))
  }
  if (!isRecord(value)) return value

  const entries = await Promise.all(Object.entries(value).map(async ([key, item]) => (
    [key, await serializeVideoPayload(item, signal)] as const
  )))
  return Object.fromEntries(entries)
}

export async function submitVideoGatewayRequest(
  apiKey: string,
  path: VideoSubmissionPath,
  request: Record<string, unknown>,
  options: VideoGatewayOptions = {},
): Promise<VideoGenerationSubmission> {
  const body = await serializeVideoPayload(request, options.signal)
  const response = await fetch(buildGatewayUrl(path), {
    method: 'POST',
    headers: {
      ...authorizationHeaders(apiKey, options.sessionId),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
    signal: options.signal,
  })
  const payload = await parseJSONResponse(response, '视频任务提交失败')
  const requestId = extractRequestID(payload)
  if (!requestId) throw new Error('视频接口未返回 request_id。')
  return { requestId }
}

export async function submitVideoGeneration(
  apiKey: string,
  request: VideoGenerationRequest,
  signal?: AbortSignal,
  options: Omit<VideoGatewayOptions, 'signal'> = {},
): Promise<VideoGenerationSubmission> {
  validateGenerationRequest(request)

  const references = [request.image, ...(request.images || [])].filter((file): file is File => !!file)
  const body = {
    model: request.model,
    prompt: request.prompt.trim(),
    duration: request.duration,
    resolution: request.resolution,
    ...(request.aspectRatio?.trim() ? { aspect_ratio: request.aspectRatio.trim() } : {}),
    ...(references[0] ? { image: references[0] } : {}),
    ...(references.length > 1 ? { images: references } : {}),
  }
  return submitVideoGatewayRequest(apiKey, '/v1/videos/generations', body, {
    signal,
    sessionId: options.sessionId,
  })
}

export function submitVideoEdit(
  apiKey: string,
  request: Record<string, unknown>,
  options: VideoGatewayOptions = {},
): Promise<VideoGenerationSubmission> {
  return submitVideoGatewayRequest(apiKey, '/v1/videos/edits', request, options)
}

export function submitVideoExtension(
  apiKey: string,
  request: Record<string, unknown>,
  options: VideoGatewayOptions = {},
): Promise<VideoGenerationSubmission> {
  return submitVideoGatewayRequest(apiKey, '/v1/videos/extensions', request, options)
}

export function parseVideoGenerationStatus(
  payload: unknown,
  fallbackRequestId = ''
): VideoGenerationStatus {
  if (!isRecord(payload)) throw new Error('视频任务状态响应格式无效。')
  const data = isRecord(payload.data) ? payload.data : null
  const source = data && asText(data.status) ? data : payload
  const video = isRecord(source.video) ? source.video : null
  const requestId = extractRequestID(payload) || fallbackRequestId.trim()
  const status = asText(source.status).toLowerCase()
  const videoUrl = asText(video?.url)
    || asText(source.video_url)
    || asText(source.download_url)
    || asText(source.url)
    || asText(payload.video_url)
    || asText(payload.download_url)
    || asText(payload.url)

  if (!requestId) throw new Error('视频任务状态缺少 request_id。')
  if (!status) throw new Error('视频任务状态响应缺少 status。')

  return {
    requestId,
    status,
    videoUrl,
    errorMessage: responseErrorMessage(source, ''),
  }
}

export async function getVideoGenerationStatus(
  apiKey: string,
  requestId: string,
  signal?: AbortSignal,
  options: Omit<VideoGatewayOptions, 'signal'> = {},
): Promise<VideoGenerationStatus> {
  const normalizedID = normalizedRequestID(requestId)
  const response = await fetch(buildGatewayUrl(videoRequestPath(normalizedID)), {
    method: 'GET',
    headers: authorizationHeaders(apiKey, options.sessionId),
    signal,
  })
  const payload = await parseJSONResponse(response, '视频任务状态查询失败')
  return parseVideoGenerationStatus(payload, normalizedID)
}

export function buildVideoContentURL(requestId: string): string {
  return buildGatewayUrl(videoRequestPath(requestId, '/content'))
}

export async function fetchVideoContent(
  apiKey: string,
  requestId: string,
  signal?: AbortSignal,
  options: Omit<VideoGatewayOptions, 'signal'> = {},
): Promise<Blob> {
  const response = await fetch(buildVideoContentURL(requestId), {
    method: 'GET',
    headers: authorizationHeaders(apiKey, options.sessionId),
    signal,
  })
  if (!response.ok) {
    await parseJSONResponse(response, '视频内容下载失败')
  }
  const blob = await response.blob()
  const mediaType = blob.type.split(';', 1)[0]?.trim().toLowerCase() || ''
  if (mediaType && mediaType !== 'application/octet-stream') return blob
  return blob.slice(0, blob.size, 'video/mp4')
}

export async function createVideoContentURL(
  apiKey: string,
  requestId: string,
  signal?: AbortSignal,
  options: Omit<VideoGatewayOptions, 'signal'> = {},
): Promise<string> {
  const blob = await fetchVideoContent(apiKey, requestId, signal, options)
  return URL.createObjectURL(blob)
}

export const videoGenerationAPI = {
  submit: submitVideoGeneration,
  edit: submitVideoEdit,
  extend: submitVideoExtension,
  submitGateway: submitVideoGatewayRequest,
  status: getVideoGenerationStatus,
  content: fetchVideoContent,
  createContentURL: createVideoContentURL,
}
