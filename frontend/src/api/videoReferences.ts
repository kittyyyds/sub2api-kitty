import { buildGatewayUrl } from './url'

export interface TemporaryVideoReference { id: string; url: string; expires_at: string; bytes: number; mime_type: string }
const uploaded = new Map<string, TemporaryVideoReference>()
let cacheKey = ''

export function validateVideoReference(file: Blob, kind: 'image' | 'audio'): void {
  if (!file.size || file.size > 25 * 1024 * 1024) throw new Error('单个参考素材需大于 0 且不超过 25MB。')
  const accepted = kind === 'image' ? /^image\/(png|jpeg|gif|webp)$/ : /^(audio\/(mpeg|mp3|wav|wave|x-wav|ogg|flac|x-flac|mp4|x-m4a)|application\/ogg)$/
  if (!accepted.test(file.type)) throw new Error(kind === 'image' ? '请选择 PNG、JPEG、GIF 或 WebP 图片。' : '请选择 MP3、WAV、OGG、FLAC 或 M4A 音频。')
}

export async function uploadVideoReference(apiKey: string, file: File, kind: 'image' | 'audio', signal: AbortSignal): Promise<TemporaryVideoReference> {
  validateVideoReference(file, kind)
  if (cacheKey !== apiKey) { uploaded.clear(); cacheKey = apiKey }
  // File identity stays in this page only; no credential or signed URL is persisted.
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', await file.arrayBuffer()))
  const hash = Array.from(digest, value => value.toString(16).padStart(2, '0')).join('')
  if (signal.aborted) throw new DOMException('上传已取消', 'AbortError')
  const cached = cacheKey === apiKey ? uploaded.get(hash) : undefined
  if (cached && Date.parse(cached.expires_at) > Date.now() + 60_000) return cached
  const body = new FormData()
  body.append('file', file)
  const controller = new AbortController()
  const abort = () => controller.abort(signal.reason)
  signal.addEventListener('abort', abort, { once: true })
  if (signal.aborted) abort()
  const timer = setTimeout(() => controller.abort(new Error('素材上传超时，请重试。')), 90_000)
  try {
    const response = await fetch(buildGatewayUrl('/v1/video-references'), { method: 'POST', headers: { Authorization: `Bearer ${apiKey}` }, body, signal: controller.signal })
    const payload = await response.json().catch(() => null)
    if (!response.ok) throw new Error(payload?.error?.message || `素材上传失败（${response.status}）`)
    if (typeof payload?.url !== 'string' || !payload.url.startsWith('https://') || !payload.expires_at) throw new Error('素材上传结果无效。')
    if (cacheKey === apiKey && !signal.aborted) {
      if (uploaded.size >= 200) uploaded.delete(uploaded.keys().next().value!)
      uploaded.set(hash, payload)
    }
    return payload as TemporaryVideoReference
  } finally { clearTimeout(timer); signal.removeEventListener('abort', abort) }
}
