import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import { priceTableRows, productFor } from './catalogTable'
import { applyMediaCatalogPolicy, type MediaCatalogPolicy, type MediaCatalogModel } from './mediaCatalogPolicy'

export type MediaKind = 'image' | 'video'
export interface MediaTier { key: string; name: string; price: number | null; period: string }
export interface MediaPriceEntry {
  key: string
  id: string
  kind: MediaKind
  group: ModelPlazaGroup
  model: PlazaModel
  unit: '张' | '条' | '秒'
  tiers: MediaTier[]
  tags: string[]
  referencePrice: number | null
  hasReferenceVideo: boolean
  variableTimePrice: boolean
  pricingConflict: boolean
  status: 'priced' | 'pending' | 'unsupported'
  document?: MediaCatalogModel
  catalogWarnings?: string[]
  estimateBlocked?: boolean
}
const valid = (n: unknown): n is number => typeof n === 'number' && Number.isFinite(n) && n >= 0
export function mediaMoney(value: number | null): string {
  if (valid(value) && value > 0 && value < 0.00000001) return '<$0.00000001'
  return valid(value) ? `$${value.toFixed(8).replace(/\.?0+$/, '') || '0'}` : '价格待确认'
}
export function mediaEntries(data: ModelPlazaResponse | null, policy?: MediaCatalogPolicy): MediaPriceEntry[] {
  const raw = (data?.groups ?? []).flatMap(group => group.models.flatMap(model => {
    const kind = productFor(model)
    if (kind !== 'image' && kind !== 'video') return []
    const rows = priceTableRows({ ...group, models: [model] }, kind)
    const tiers = rows.map(row => ({
      key: row.key, name: row.tier || '标准规格', period: row.period,
      price: valid(row.metrics[0]?.current) ? row.metrics[0].current : null,
    }))
    const caps = model.capabilities
    const operationLabels: Record<string, string> = { text_to_image: '文生图', image_to_image: '图生图',
      text_to_video: '文生视频', image_to_video: '图生视频', reference_to_video: '参考生视频', video_to_video: '视频编辑' }
    const tags = (caps?.operations ?? []).map(op => operationLabels[op]).filter(Boolean)
    const hasReferenceVideo = (caps?.reference_videos?.max ?? 0) > 0
    return [{ key: JSON.stringify([group.id, model.platform || group.platform, kind, model.name]), id: model.name, kind, group, model,
      unit: kind === 'image' ? '张' : model.pricing?.billing_mode === 'per_request' ? '条' : '秒',
      tiers, tags: [...new Set(tags)], hasReferenceVideo,
      referencePrice: hasReferenceVideo && valid(model.pricing?.reference_per_second) ? model.pricing.reference_per_second : null,
      variableTimePrice: model.platform !== 'video' && group.platform !== 'video'
        && (group.peak_rate_enabled || (model.time_pricing?.periods?.length ?? 0) > 0),
      pricingConflict: false,
      status: caps?.supported === false ? 'unsupported' : tiers.some(tier => valid(tier.price)) ? 'priced' : 'pending',
    } satisfies MediaPriceEntry]
  }))
  // The public DTO can contain both a final-price record and its legacy duplicate.
  // Never merge across groups or normalize a customer's exact model ID.
  const byIdentity = new Map<string, MediaPriceEntry[]>()
  for (const entry of raw) byIdentity.set(entry.key, [...(byIdentity.get(entry.key) ?? []), entry])
  const entries = [...byIdentity.values()].map(candidates => {
    const isFinal = (entry: MediaPriceEntry) => {
      if (entry.model.pricing?.final_unit_price) return true
      const marked = new Set((entry.kind === 'image' ? entry.model.final_image_price_tiers : entry.model.final_video_price_tiers)?.map(t => t.trim().toLowerCase()) ?? [])
      const standard = entry.tiers.filter(t => !t.period)
      return standard.length > 0 && standard.every(t => valid(t.price) && marked.has(t.name.trim().toLowerCase()))
    }
    const authoritative = candidates.filter(isFinal)
    const eligible = authoritative.length ? authoritative : candidates
    const chosen = eligible[0]!
    const signature = (entry: MediaPriceEntry) => JSON.stringify([entry.unit, entry.referencePrice, entry.hasReferenceVideo, entry.variableTimePrice,
      entry.tiers.map(t => [t.name, t.period, t.price]).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)))])
    const pricingConflict = eligible.some(entry => signature(entry) !== signature(chosen))
    if (!pricingConflict) return chosen
    // Conflicting quotes are not a license to pick the cheapest value.
    return { ...chosen, pricingConflict: true, status: 'pending' as const, referencePrice: null, tiers: chosen.tiers.map(t => ({ ...t, price: null })) }
  })
  return policy ? applyMediaCatalogPolicy(entries, policy) : entries
}
export function mediaPriceSummary(entry: MediaPriceEntry): string {
  if (entry.catalogWarnings?.length) return '价格／规格待核对'
  if (entry.pricingConflict) return '价格冲突 · 待确认'
  if (entry.status === 'unsupported') return '暂不支持调用'
  const standard = entry.tiers.filter(t => !t.period && valid(t.price))
  if (!standard.length) return '价格待确认'
  const values = standard.map(t => t.price as number)
  return `${mediaMoney(Math.min(...values))}/${entry.unit}${new Set(values).size > 1 ? ' 起' : ''}`
}
export function mediaMatches(entry: MediaPriceEntry, search: string): boolean {
  const text = [entry.id, entry.group.name, entry.model.description ?? '', entry.document?.categoryId ?? '', ...entry.tags].join(' ').toLocaleLowerCase()
  return search.trim().toLocaleLowerCase().split(/\s+/).every(part => text.includes(part))
}
export function estimateMediaPrice(entry: MediaPriceEntry, tierKey: string, quantity: number, seconds: number, referenceSeconds: number) {
  const tier = entry.tiers.find(t => t.key === tierKey)
  if (entry.status !== 'priced' || entry.estimateBlocked || !tier || !valid(tier.price) || entry.variableTimePrice
    || !Number.isInteger(quantity) || quantity < 1 || quantity > 100
    || !valid(referenceSeconds) || !valid(seconds)) return null
  const caps = entry.model.capabilities
  if (entry.kind === 'video') {
    if (entry.document?.secondOptions?.length && !entry.document.secondOptions.includes(seconds)) return null
    if (entry.document?.seconds && (seconds < entry.document.seconds.min || seconds > entry.document.seconds.max)) return null
    if (caps?.second_options?.length && !caps.second_options.includes(seconds)) return null
    if (caps?.seconds && (seconds < caps.seconds.min || seconds > caps.seconds.max)) return null
    if (entry.unit === '秒' && seconds <= 0) return null
  }
  if (referenceSeconds > 0 && (!entry.hasReferenceVideo || entry.referencePrice === null
    || (caps?.max_reference_seconds != null && referenceSeconds > caps.max_reference_seconds))) return null
  const generation = tier.price * (entry.unit === '秒' ? seconds : quantity)
  const reference = referenceSeconds * (entry.referencePrice ?? 0)
  return { generation, reference, total: generation + reference }
}
