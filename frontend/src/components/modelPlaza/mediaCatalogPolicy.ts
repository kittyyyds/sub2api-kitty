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
  /** The source table has conflicting units; the effective backend price remains authoritative. */
  sourceUnitConflict?: boolean
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
    if (!documented.sourceUnitConflict && documented.billingUnits.length && !documented.billingUnits.includes(entry.unit)) {
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
    // A reviewed source-unit discrepancy must not hide a verified final selling
    // price. It still blocks estimation; backend conflicts and other warnings do not qualify.
    const catalogPriceVerified = documented.sourceUnitConflict === true && warnings.length === 0
      && entry.status === 'priced' && !entry.pricingConflict && entry.model.pricing?.final_unit_price === true
      && tiers.length > 0
    if (documented.sourceUnitConflict) warnings.push('来源价表的计费单位存在冲突，当前显示本站分组的实际售价与单位；费用试算暂停。')
    return [{ ...entry, tags, hasReferenceVideo, tiers,
      status: entry.status === 'priced' && !tiers.length ? 'pending' as const : entry.status,
      referencePrice: hasReferenceVideo && !documented.conditionalReferencePricing ? entry.referencePrice : null,
      model: { ...entry.model, description: documented.description },
      document: documented, catalogWarnings: [...new Set(warnings)], catalogPriceVerified,
      estimateBlocked: documented.conditionalReferencePricing || documented.durationVariesByResolution === true || warnings.length > 0,
      ...(warnings.length && !catalogPriceVerified ? { status: 'pending' as const, referencePrice: null, tiers: tiers.map(tier => ({ ...tier, price: null })) } : {}),
    }]
  })
}
