import { buildGatewayUrl } from '@/api/url'

export type SuanliVideoFamily = 'seedance' | 'sd25' | 'h3'
export interface SuanliVideoModel {
  id: string
  family: SuanliVideoFamily
  resolution: '480p' | '720p' | '1080p' | '2K'
  maxImages: number
  maxAudio: number
}

export const SUANLI_VIDEO_MODELS: readonly SuanliVideoModel[] = [
  { id: '官转稳定sd2.0(933原生过真人480P)', family: 'seedance', resolution: '480p', maxImages: 1, maxAudio: 0 },
  { id: '官转稳定sd2.0(933原生过真人720P)', family: 'seedance', resolution: '720p', maxImages: 1, maxAudio: 0 },
  { id: '官转稳定sd2.5(满血30-10-10不卡脸1080P)', family: 'seedance', resolution: '1080p', maxImages: 1, maxAudio: 0 },
  { id: 'sd2.0(满血9-3-3不卡脸720P)', family: 'seedance', resolution: '720p', maxImages: 1, maxAudio: 0 },
  { id: 'sd2.5(满血30-10-10不卡脸720P)', family: 'seedance', resolution: '720p', maxImages: 1, maxAudio: 0 },
  { id: '特价/sd-2.5', family: 'sd25', resolution: '720p', maxImages: 10, maxAudio: 0 },
  { id: '特价/官方h3-2k', family: 'h3', resolution: '2K', maxImages: 9, maxAudio: 3 },
  { id: '特价/官方h3-720p', family: 'h3', resolution: '720p', maxImages: 9, maxAudio: 3 },
  { id: '特价/官方h3-1080p', family: 'h3', resolution: '1080p', maxImages: 9, maxAudio: 3 },
]

export const SUANLI_VIDEO_RATIOS = ['16:9', '9:16', '1:1'] as const
export type SuanliVideoRatio = (typeof SUANLI_VIDEO_RATIOS)[number]

export function isExclusiveSuanliModelCatalog(ids: string[]): boolean {
  const allowed = new Set(SUANLI_VIDEO_MODELS.map((model) => model.id))
  return ids.length > 0 && ids.every((id) => allowed.has(id))
}

export async function loadVideoModelIDs(apiKey: string, signal?: AbortSignal): Promise<string[]> {
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: { Authorization: `Bearer ${apiKey}` },
    signal,
  })
  if (!response.ok) throw new Error('读取当前密钥的视频型号失败，请重新选择密钥。')
  const payload = await response.json() as { data?: Array<{ id?: unknown }> }
  if (!Array.isArray(payload.data)) throw new Error('当前密钥的模型列表格式无效。')
  return payload.data.flatMap((model) => typeof model.id === 'string' ? [model.id] : [])
}

export function splitReferenceURLs(text: string): string[] {
  return text.split(/\r?\n/).map((value) => value.trim()).filter(Boolean)
}

function validatePublicURLs(urls: string[], label: string): void {
  for (const raw of urls) {
    let url: URL
    try { url = new URL(raw) } catch { throw new Error(`${label}需要填写完整的公开链接。`) }
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) {
      throw new Error(`${label}仅支持无需登录的 HTTP 或 HTTPS 链接。`)
    }
  }
}

export function suanliPixelSize(resolution: SuanliVideoModel['resolution'], ratio: SuanliVideoRatio): string {
  const short = { '480p': 480, '720p': 720, '1080p': 1080, '2K': 1440 }[resolution]
  const long = Math.round(short * 16 / 9 / 2) * 2
  return ratio === '1:1' ? `${short}x${short}` : ratio === '9:16' ? `${short}x${long}` : `${long}x${short}`
}

export function buildSuanliVideoRequest(input: {
  modelID: string
  prompt: string
  duration: number
  ratio: SuanliVideoRatio
  imageURLs: string[]
  audioURLs: string[]
}): Record<string, unknown> {
  const model = SUANLI_VIDEO_MODELS.find((item) => item.id === input.modelID)
  if (!model) throw new Error('请选择当前分组支持的视频型号。')
  if (!input.prompt.trim()) throw new Error('请输入视频提示词。')
  if (!SUANLI_VIDEO_RATIOS.includes(input.ratio)) throw new Error('请选择支持的画面比例。')
  if (!input.imageURLs.length || input.imageURLs.length > model.maxImages) {
    throw new Error(`当前型号需要 1–${model.maxImages} 张参考图。`)
  }
  if (input.audioURLs.length > model.maxAudio) throw new Error(`当前型号最多支持 ${model.maxAudio} 条参考音频。`)
  validatePublicURLs(input.imageURLs, '参考图片')
  validatePublicURLs(input.audioURLs, '参考音频')
  if (model.family === 'sd25') {
    return {
      model: model.id, prompt: input.prompt.trim(), resolution: '720p',
      aspect_ratio: input.ratio, images: input.imageURLs,
    }
  }
  if (!Number.isInteger(input.duration) || input.duration < 5 || input.duration > 15) {
    throw new Error('本次测试的时长范围为 5–15 秒。')
  }
  return {
    model: model.id,
    prompt: input.prompt.trim(),
    seconds: input.duration,
    size: suanliPixelSize(model.resolution, input.ratio),
    reference_images: input.imageURLs.map((url) => ({ url, role: 'reference_image' })),
    ...(input.audioURLs.length ? { reference_audios: input.audioURLs.map((url) => ({ url })) } : {}),
  }
}
