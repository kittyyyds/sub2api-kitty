import {
  imageGenerationAPI,
  isAsyncImageGatewayUnavailableError,
  type GeneratedImage,
} from '@/api/imageGeneration'
import {
  GROK_VIDEO_15_MODEL,
  videoGenerationAPI,
  type GrokVideoModel,
  type GrokVideoResolution,
} from '@/api/videoGeneration'
import { buildGatewayUrl } from '@/api/url'
import { consumeSSE } from '@/features/ai-workbench/sse'
import {
  fileToDataURL,
  gatewayAuthorization,
  readGatewayError,
} from '@/features/ai-workbench/gateway'
import { sanitizeSvg } from '@/utils/sanitize'

import { streamChatCompletion } from '@/features/kitty-chat/chatGateway'
import { DEFAULT_CHAT_SETTINGS } from '@/features/kitty-chat/storage'
import type { ChatAPIMessage } from '@/features/kitty-chat/types'
import type {
  StudioForm,
  StudioModelOption,
  StudioResultType,
} from './types'

const VIDEO_POLL_INTERVAL_MS = 4_000
const VIDEO_POLL_TIMEOUT_MS = 30 * 60 * 1_000

export interface GeneratedStudioAsset {
  type: StudioResultType
  source: string
  mimeType: string
  requestId?: string
  blob?: Blob
}

interface GenerateStudioOptions {
  apiKey: string
  model: StudioModelOption
  form: StudioForm
  references: File[]
  signal: AbortSignal
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function mimeForFormat(format: StudioForm['format']): string {
  return format === 'jpeg' ? 'image/jpeg' : format === 'webp' ? 'image/webp' : 'image/png'
}

function dataSource(value: string, mimeType: string): string {
  const text = value.trim()
  if (!text) return ''
  if (/^(?:data:|blob:|https?:\/\/)/i.test(text)) return text
  return `data:${mimeType};base64,${text}`
}

export function imageDimensions(form: Pick<StudioForm, 'ratio' | 'resolution'>): string {
  const scale = form.resolution === '4k' ? 3840 : form.resolution === '2k' ? 2048 : 1024
  switch (form.ratio) {
    case '16:9': return `${scale}x${Math.round(scale * 9 / 16)}`
    case '9:16': return `${Math.round(scale * 9 / 16)}x${scale}`
    case '3:2': return `${scale}x${Math.round(scale * 2 / 3)}`
    case '2:3': return `${Math.round(scale * 2 / 3)}x${scale}`
    default: return `${scale}x${scale}`
  }
}

function generatedImagesToAssets(images: GeneratedImage[], mimeType: string): GeneratedStudioAsset[] {
  return images.map((image) => ({
    type: 'image',
    source: dataSource(image.url, mimeType),
    mimeType,
  }))
}

async function generateOpenAIImage(options: GenerateStudioOptions): Promise<GeneratedStudioAsset[]> {
  const path = options.form.mode === 'image-image' ? '/v1/images/edits' : '/v1/images/generations'
  const asyncPath = `${path}/async` as '/v1/images/generations/async' | '/v1/images/edits/async'
  const request = {
    model: options.model.id,
    prompt: options.form.prompt.trim(),
    n: 1,
    size: imageDimensions(options.form),
    resolution: options.form.resolution,
    aspect_ratio: options.form.ratio,
    output_format: options.form.format,
    background: options.form.background,
  }
  const requestOptions = {
    signal: options.signal,
    referenceImages: options.references,
  }
  try {
    const task = await imageGenerationAPI.submitAsync(options.apiKey, asyncPath, request, requestOptions)
    const images = await imageGenerationAPI.pollAsync(options.apiKey, task.taskId, { signal: options.signal })
    return generatedImagesToAssets(images, mimeForFormat(options.form.format))
  } catch (error) {
    if (!isAsyncImageGatewayUnavailableError(error)) throw error
    const images = await imageGenerationAPI.submitGateway(options.apiKey, path, request, requestOptions)
    return generatedImagesToAssets(images, mimeForFormat(options.form.format))
  }
}

function collectInlineImages(value: unknown, output: GeneratedStudioAsset[]): void {
  if (Array.isArray(value)) {
    value.forEach((item) => collectInlineImages(item, output))
    return
  }
  if (!isRecord(value)) return

  const inline = isRecord(value.inlineData) ? value.inlineData : isRecord(value.inline_data) ? value.inline_data : null
  if (inline) {
    const data = asText(inline.data)
    const mimeType = asText(inline.mimeType ?? inline.mime_type) || 'image/png'
    if (data && mimeType.startsWith('image/')) {
      output.push({ type: 'image', source: dataSource(data, mimeType), mimeType })
    }
  }
  Object.values(value).forEach((item) => collectInlineImages(item, output))
}

export function extractGeminiImages(payload: unknown): GeneratedStudioAsset[] {
  const images: GeneratedStudioAsset[] = []
  collectInlineImages(payload, images)
  const seen = new Set<string>()
  return images.filter((image) => {
    if (seen.has(image.source)) return false
    seen.add(image.source)
    return true
  })
}

async function generateGeminiImage(options: GenerateStudioOptions): Promise<GeneratedStudioAsset[]> {
  const parts: Array<Record<string, unknown>> = [{ text: options.form.prompt.trim() }]
  for (const reference of options.references) {
    const source = await fileToDataURL(reference)
    parts.push({
      inlineData: {
        mimeType: reference.type || 'image/png',
        data: source.split(',', 2)[1] || '',
      },
    })
  }
  const response = await fetch(buildGatewayUrl(`/v1beta/models/${encodeURIComponent(options.model.id)}:streamGenerateContent?alt=sse`), {
    method: 'POST',
    headers: {
      ...gatewayAuthorization(options.apiKey),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      contents: [{ role: 'user', parts }],
      generationConfig: {
        responseModalities: ['TEXT', 'IMAGE'],
        imageConfig: {
          aspectRatio: options.form.ratio,
          imageSize: options.form.resolution.toUpperCase(),
        },
      },
    }),
    signal: options.signal,
  })
  if (!response.ok) throw await readGatewayError(response, 'Gemini 图片生成失败')

  const assets: GeneratedStudioAsset[] = []
  await consumeSSE(response, ({ data }) => {
    if (!data || data === '[DONE]') return
    try {
      assets.push(...extractGeminiImages(JSON.parse(data) as unknown))
    } catch {
      // Some compatible providers send a non-JSON heartbeat; it has no image payload.
    }
  })
  if (!assets.length) throw new Error('Gemini 已完成响应，但没有返回图片数据。')
  return assets
}

export function extractSanitizedSvg(content: string): string {
  const match = content.match(/<svg\b[\s\S]*?<\/svg>/i)
  return match ? sanitizeSvg(match[0]) : ''
}

async function generateClaudeSvg(options: GenerateStudioOptions): Promise<GeneratedStudioAsset[]> {
  const referenceNotes = options.references.length
    ? `\n用户还上传了 ${options.references.length} 张参考图。请依据描述重建画面构图。`
    : ''
  const messages: ChatAPIMessage[] = [{
    role: 'system',
    content: 'You are an expert SVG illustrator. Return one complete standalone SVG only. Do not use scripts, foreignObject, external URLs, embedded HTML, or markdown fences.',
  }, {
    role: 'user',
    content: `请创作一幅精细的 SVG 矢量插画：${options.form.prompt.trim()}${referenceNotes}\n画布比例 ${options.form.ratio}，确保文字可读、元素不重叠。`,
  }]
  let content = ''
  await streamChatCompletion({
    apiKey: options.apiKey,
    model: options.model.id,
    messages,
    settings: { ...DEFAULT_CHAT_SETTINGS, temperature: 0.5, maxTokens: 16_000 },
    signal: options.signal,
  }, { onText: (chunk) => { content += chunk } })
  const svg = extractSanitizedSvg(content)
  if (!svg) throw new Error('Claude 已返回内容，但没有解析到完整 SVG。')
  return [{
    type: 'image',
    source: `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`,
    mimeType: 'image/svg+xml',
  }]
}

function wait(delayMs: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.reject(signal.reason)
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(resolve, delayMs)
    signal.addEventListener('abort', () => {
      window.clearTimeout(timer)
      reject(signal.reason || new DOMException('The operation was aborted.', 'AbortError'))
    }, { once: true })
  })
}

async function generateGrokVideo(options: GenerateStudioOptions): Promise<GeneratedStudioAsset[]> {
  const image = options.form.mode === 'image-video' ? options.references[0] : undefined
  const submission = await videoGenerationAPI.submit(options.apiKey, {
    model: options.model.id as GrokVideoModel,
    prompt: options.form.prompt.trim(),
    duration: Math.max(1, Math.min(15, Math.round(options.form.videoDuration))),
    resolution: options.form.videoResolution as GrokVideoResolution,
    aspectRatio: options.form.ratio,
    image,
  }, options.signal)

  const startedAt = Date.now()
  while (true) {
    const status = await videoGenerationAPI.status(options.apiKey, submission.requestId, options.signal)
    const normalized = status.status.toLowerCase()
    if (['done', 'completed', 'succeeded', 'success'].includes(normalized)) {
      const blob = await videoGenerationAPI.content(options.apiKey, submission.requestId, options.signal)
      return [{
        type: 'video',
        source: '',
        mimeType: blob.type || 'video/mp4',
        requestId: submission.requestId,
        blob,
      }]
    }
    if (['failed', 'expired', 'cancelled', 'canceled'].includes(normalized)) {
      throw new Error(status.errorMessage || `视频任务${normalized}。`)
    }
    if (Date.now() - startedAt >= VIDEO_POLL_TIMEOUT_MS) {
      throw new Error(`视频仍在上游处理中，任务 ID：${submission.requestId}`)
    }
    await wait(VIDEO_POLL_INTERVAL_MS, options.signal)
  }
}

export async function generateStudioAsset(options: GenerateStudioOptions): Promise<GeneratedStudioAsset[]> {
  switch (options.model.strategy) {
    case 'openai-image': return generateOpenAIImage(options)
    case 'gemini-image': return generateGeminiImage(options)
    case 'claude-svg': return generateClaudeSvg(options)
    case 'grok-video': return generateGrokVideo(options)
  }
}

export function videoResolutionAllowed(model: string, resolution: StudioForm['videoResolution']): boolean {
  return resolution !== '1080p' || model === GROK_VIDEO_15_MODEL
}
