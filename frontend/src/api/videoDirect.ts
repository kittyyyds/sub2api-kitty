import { buildGatewayUrl } from './url'

export interface VideoCapability {
  media_type: string; supported: boolean; family?: string; provider?: string
  seconds?: { min: number; max: number; fixed?: number }; second_options?: number[]
  resolutions?: string[]; aspect_ratios?: string[]; reference_images?: { min: number; max: number }
  reference_videos?: { min: number; max: number }; reference_audios?: { min: number; max: number }
  max_reference_seconds?: number; face?: boolean; generate_audio?: boolean
}
export interface VideoModel { id: string; capabilities: VideoCapability }
export interface VideoQuote { estimated_cost: number; unit_price: number; unit: string; reference_seconds: number; reference_per_second: number; output_cost: number; reference_cost: number; reference_videos?: string[]; note: string }

export async function videoModels(key: string, signal?: AbortSignal): Promise<VideoModel[]> {
  const r = await fetch(buildGatewayUrl('/v1/models'), { headers: { Authorization: `Bearer ${key}` }, signal, cache: 'no-store' })
  const data = await r.json()
  if (!r.ok) throw new Error('视频模型目录读取失败')
  return (data.data || []).filter((m: VideoModel) => m.capabilities?.supported && m.capabilities.media_type === 'video')
}
export async function videoQuote(key: string, body: Record<string, unknown>, signal?: AbortSignal): Promise<VideoQuote> {
  const r = await fetch(buildGatewayUrl('/v1/videos/quote'), { method: 'POST', headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body), signal, cache: 'no-store' })
  const data = await r.json()
  if (!r.ok || data.code !== 0) throw new Error(data.message || '视频报价暂时不可用')
  return data.data
}
export async function videoDelivery(key: string, id: string, signal?: AbortSignal): Promise<{ url: string; direct: boolean }> {
  const path = `/v1/videos/${encodeURIComponent(id)}`
  const r = await fetch(buildGatewayUrl(path), { headers: { Authorization: `Bearer ${key}` }, signal, cache: 'no-store' })
  const data = await r.json()
  if (!r.ok) throw new Error(data.error?.message || '视频交付状态暂时不可用')
  if (data.delivery_status === 'expired' || data.status === 'expired') throw new Error('上游视频已过期，无法继续下载')
  if (data.delivery_mode !== 'upstream_direct') return { url: buildGatewayUrl(path + '/content'), direct: false }
  if (data.status !== 'completed' || typeof data.url !== 'string') throw new Error('视频尚未交付，请继续查询原任务')
  const url = new URL(data.url)
  if (url.protocol !== 'https:' || url.username || url.password) throw new Error('上游直链无效')
  return { url: url.href, direct: true }
}
export async function fetchVideoContent(key: string, id: string, headers: Record<string, string> = {}, signal?: AbortSignal): Promise<Response> {
  const delivery = await videoDelivery(key, id, signal)
  const safe: Record<string, string> = {}
  for (const [name, value] of Object.entries(headers)) if (/^(range|if-range)$/i.test(name)) safe[name] = value
  if (!delivery.direct) safe.Authorization = `Bearer ${key}`
  return fetch(delivery.url, { headers: safe, signal, cache: 'no-store', credentials: 'omit', referrerPolicy: 'no-referrer', redirect: delivery.direct ? 'follow' : 'error' })
}
