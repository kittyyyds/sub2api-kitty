import rawCatalog from './suanliMediaCatalog.json'
import type { MediaKind, MediaPriceEntry } from './mediaPricing'

export interface MediaCatalogModel {
  id: string
  kind: MediaKind
  categoryId?: string
  sourceRows: number[]
  description: string
  /** Editorial source labels; never live health or authorization signals. */
  sourceBadges?: string[]
  sourceAdvantages?: string
  sourceDescription?: string
  sourceRecommendation?: string
  annotationRows?: number[]
  billingUnits: string[]
  pricingNotes: string[]
  warnings: string[]
  conditionalReferencePricing: boolean
  referenceRule: 'free' | 'charged' | 'unspecified'
  seconds?: { min: number; max: number }
  secondOptions?: number[]
  referenceVideoMentioned?: boolean
  durationVariesByResolution?: boolean
  pricedResolutions?: string[]
}
export interface MediaCatalogPolicy {
  sourceUrl: string
  title: string
  capturedOn: string
  categories?: { id: string; label: string; description?: string }[]
  models: MediaCatalogModel[]
}
// A dated editorial whitelist, not a live upstream scanner or a pricing authority.
// This public file deliberately contains no supplier monetary amounts.
export const suanliMediaCatalog = rawCatalog as MediaCatalogPolicy

export function applyMediaCatalogPolicy(entries: MediaPriceEntry[], policy: MediaCatalogPolicy): MediaPriceEntry[] {
  const catalog = new Map(policy.models.map(model => [model.id, model]))
  return entries.flatMap(entry => {
    const documented = catalog.get(entry.id)
    if (!documented) return []
    const warnings = [...documented.warnings]
    if (documented.kind !== entry.kind) warnings.push('价表分类与本站接口分类不一致，需核对。')
    if (documented.billingUnits.length && !documented.billingUnits.includes(entry.unit)) {
      warnings.push('价表与本站的计费单位不一致，暂不展示确定报价。')
    }
    if (documented.referenceRule === 'free' && entry.referencePrice !== null && entry.referencePrice > 0) {
      warnings.push('价表注明参考视频不计费，但本站返回了参考费用，需核对。')
    }
    const tags: string[] = []
    if (/文生视频/.test(documented.description)) tags.push('文生视频')
    if (/图生视频/.test(documented.description)) tags.push('图生视频')
    if (/首帧/.test(documented.description)) tags.push('首帧参考')
    if (/尾帧/.test(documented.description)) tags.push('尾帧参考')
    // Do not publish the gateway's generic 1–120s / all-resolution fallback as a model promise.
    const hasReferenceVideo = entry.hasReferenceVideo && documented.referenceVideoMentioned === true
    const tiers = entry.tiers.filter(tier => tier.price !== null
      && (!documented.pricedResolutions?.length || documented.pricedResolutions.includes(tier.name.toLowerCase())))
    return [{ ...entry, tags, hasReferenceVideo, tiers,
      status: entry.status === 'priced' && !tiers.length ? 'pending' as const : entry.status,
      referencePrice: hasReferenceVideo && !documented.conditionalReferencePricing ? entry.referencePrice : null,
      model: { ...entry.model, description: documented.description },
      document: documented, catalogWarnings: [...new Set(warnings)],
      estimateBlocked: documented.conditionalReferencePricing || documented.durationVariesByResolution === true || warnings.length > 0,
      ...(warnings.length ? { status: 'pending' as const, referencePrice: null, tiers: tiers.map(tier => ({ ...tier, price: null })) } : {}),
    }]
  })
}
