import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import { resolveIntervalPrices } from '@/utils/pricing'
import { buildCatalogSections, catalogKind, groupModelRate, normalizeModelId, type CatalogMetric } from './modelCatalog'

export interface PriceTableRow {
  key: string
  modelKey: string
  contextBand: 'standard' | 'extended' | null
  model: PlazaModel
  modelId: string
  kind: ReturnType<typeof catalogKind>
  tier: string
  period: string
  metrics: CatalogMetric[]
}

export function productFor(model: PlazaModel): string {
  const kind = catalogKind(model)
  if (kind === 'image') return 'image'
  if (kind === 'video' || model.platform === 'video') return 'video'
  return model.platform
}

// Use the same USD units, interval resolver and effective rates as Kitty's catalog.
// Media final-price tiers continue through modelCatalog to avoid multiplying twice.
export function priceTableRows(group: ModelPlazaGroup, product: string): PriceTableRow[] {
  return group.models.filter(model => productFor(model) === product).flatMap<PriceTableRow>(model => {
    const kind = catalogKind(model)
    const modelId = model.platform === 'video' ? model.name : normalizeModelId(model.name)
    // A product can contain identically named models from several platforms.
    // Keep their identities separate while leaving the displayed/copyable ID intact.
    const modelKey = JSON.stringify([group.id, model.platform || group.platform, kind, model.name])
    if (kind !== 'token') {
      const card = buildCatalogSections([{ ...group, models: [model] }], group.id)[0]?.cards[0]
      const finalTiers = new Set((kind === 'image' ? model.final_image_price_tiers : model.final_video_price_tiers)?.map(tier => tier.trim().toLowerCase()) ?? [])
      const periods = group.platform === 'video' || model.platform === 'video' ? [] : model.time_pricing?.periods ?? []
      return card?.metrics.flatMap(metric => {
        const standard = { key: `${modelKey}:${metric.key}`, modelKey, contextBand: null, model, modelId, kind, tier: metric.tier, period: '', metrics: [metric] }
        return [standard, ...periods.map((period, index) => {
          const baseRate = groupModelRate(group, kind)
          const timeRate = Math.round(baseRate * period.multiplier * 1000) / 1000
          const timePrice = metric.current == null ? null : finalTiers.has(metric.tier.trim().toLowerCase()) || baseRate === 0 ? metric.current : metric.current / baseRate * timeRate
          return { ...standard, key: `${standard.key}:time:${index}`, period: `${period.start_time}–${period.end_time} · ${model.time_pricing?.timezone} · ×${period.multiplier}${model.time_pricing?.weekdays_only ? ' (Mon–Fri)' : ''}`, metrics: [{ ...metric, current: timePrice }] }
        })]
      }) ?? []
    }
    const pricing = model.pricing
    const intervals = pricing?.intervals?.length
      ? [...pricing.intervals].sort((a, b) => a.min_tokens - b.min_tokens).map(interval => resolveIntervalPrices(interval, pricing))
      : [null]
    // The backend has already resolved group/global rules and the legacy switch.
    // Do not hide an effective global tier merely because that switch is off.
    const activeIntervals = intervals
    const periods = [null, ...(model.time_pricing?.periods ?? [])]
    return activeIntervals.flatMap((interval, index) => periods.map((period, periodIndex) => {
      const official = interval && model.official_pricing?.intervals?.find(candidate =>
        candidate.min_tokens === interval.min_tokens && candidate.max_tokens === interval.max_tokens)
      const officialPrices = official && model.official_pricing ? resolveIntervalPrices(official, model.official_pricing) : model.official_pricing
      const baseRate = groupModelRate(group, kind)
      const rate = period ? Math.round(baseRate * period.multiplier * 1000) / 1000 : baseRate
      const fields = ['input', 'output', 'cache_write', 'cache_read', 'cache_write_1h'] as const
      const metrics = fields.map(label => {
        const field = `${label}_price` as const
        const reference = officialPrices?.[field] ?? model.official_pricing?.[field] ?? null
        const base = interval?.[field] ?? pricing?.[field] ?? reference
        const current = base == null ? null : base * rate
        return { key: label, label: label === 'cache_write_1h' ? 'cache_write' : label, tier: '', current, official: reference,
          saving: current != null && reference != null && reference > 0 && current < reference ? Math.round((1 - current / reference) * 1000) / 10 : null } satisfies CatalogMetric
      })
      const tier = interval ? interval.tier_label || (interval.max_tokens == null ? `>${interval.min_tokens.toLocaleString()} tokens` : `≤${interval.max_tokens.toLocaleString()} tokens`) : ''
      return { key: `${modelKey}:${index}:${periodIndex}`, modelKey,
        contextBand: interval ? (interval.min_tokens > 0 ? 'extended' : 'standard') : null,
        model, modelId, kind, tier,
        period: period ? `${period.start_time}–${period.end_time} · ${model.time_pricing?.timezone} · ×${period.multiplier}${model.time_pricing?.weekdays_only ? ' (Mon–Fri)' : ''}` : '', metrics }
    }))
  })
}

/** Merge token model identities, not prices; media rows retain their own specs. */
export function groupPriceTableRows(rows: PriceTableRow[]): { key: string; rows: PriceTableRow[] }[] {
  const groups: { key: string; rows: PriceTableRow[] }[] = []
  for (const row of rows) {
    const key = row.kind === 'token' ? row.modelKey : row.key
    const previous = groups[groups.length - 1]
    if (previous?.key === key) previous.rows.push(row)
    else groups.push({ key, rows: [row] })
  }
  return groups
}
