import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

export type CatalogKind = 'token' | 'image' | 'video' | 'per_request'
export type CatalogMetricKey = 'input' | 'cache_write' | 'cache_read' | 'output' | 'unit'

export interface CatalogMetric {
  key: string
  label: CatalogMetricKey
  tier: string
  current: number | null
  official: number | null
  saving: number | null
}

export interface CatalogCard {
  key: string
  modelId: string
  modelName: string
  platform: string
  kind: CatalogKind
  groupId: number
  groupName: string
  multiplier: number
  metrics: CatalogMetric[]
  hasLongContextPricing: boolean
  hasTimePricing: boolean
}

export interface CatalogSection {
  key: string
  platform: string
  kind: CatalogKind
  cards: CatalogCard[]
}

const SECTION_ORDER: Record<string, number> = {
  'openai:token': 10,
  'anthropic:token': 20,
  'gemini:token': 30,
  'antigravity:token': 40,
  'grok:token': 50,
  'openai:image': 100,
  'gemini:image': 110,
  'anthropic:image': 120,
  'antigravity:image': 130,
  'grok:image': 140,
  'grok:video': 200,
}

function finitePrice(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null
}

export function normalizeModelId(value: string): string {
  let result = value.trim()
  if (result.startsWith('models/')) result = result.slice('models/'.length)
  const marker = '/models/'
  const markerIndex = result.lastIndexOf(marker)
  if (markerIndex >= 0) result = result.slice(markerIndex + marker.length)
  return result
}

export function humanizeModelName(value: string): string {
  return normalizeModelId(value)
    .split(/[-_]/g)
    .filter(Boolean)
    .map((part) => {
      const lower = part.toLowerCase()
      if (['gpt', 'api', 'ai'].includes(lower)) return lower.toUpperCase()
      if (/^\d+k$/i.test(part)) return part.toUpperCase()
      return `${part.charAt(0).toUpperCase()}${part.slice(1)}`
    })
    .join(' ')
}

export function catalogKind(model: PlazaModel): CatalogKind {
  const mode = model.pricing?.billing_mode
  // Missing video pricing is not token pricing and must not inherit an official price.
  if (model.platform === 'video') return mode === 'per_request' ? 'per_request' : 'video'
  if (mode === 'image' || mode === 'video' || mode === 'per_request') return mode
  return 'token'
}

export function groupTextRate(group: ModelPlazaGroup): number {
  if (group.platform === 'video') return 1
  const value = group.user_rate_multiplier ?? group.rate_multiplier
  return Number.isFinite(value) && value >= 0 ? value : 1
}

export function groupModelRate(group: ModelPlazaGroup, kind: CatalogKind): number {
  if (group.platform === 'video') return 1
  if (kind === 'image' && group.image_rate_independent) {
    return finitePrice(group.image_rate_multiplier) ?? 1
  }
  if (kind === 'video' && group.video_rate_independent) {
    return finitePrice(group.video_rate_multiplier) ?? 1
  }
  return groupTextRate(group)
}

function saving(current: number | null, official: number | null): number | null {
  if (current == null || official == null || official <= 0 || current >= official) return null
  return Math.max(0, Math.min(99.9, Math.round((1 - current / official) * 1000) / 10))
}

function tokenMetric(
  model: PlazaModel,
  group: ModelPlazaGroup,
  label: Exclude<CatalogMetricKey, 'unit'>,
): CatalogMetric {
  const field = `${label}_price` as const
  const official = finitePrice(model.official_pricing?.[field])
  const base = finitePrice(model.pricing?.[field]) ?? official
  const current = base == null ? null : base * groupModelRate(group, 'token')
  return {
    key: label,
    label,
    tier: '',
    current,
    official,
    saving: saving(current, official),
  }
}

function tokenMetrics(model: PlazaModel, group: ModelPlazaGroup): CatalogMetric[] {
  const metrics = [
    tokenMetric(model, group, 'input'),
    tokenMetric(model, group, 'cache_write'),
    tokenMetric(model, group, 'cache_read'),
    tokenMetric(model, group, 'output'),
  ]
  return metrics.filter((metric) => (
    metric.label === 'input'
    || metric.label === 'output'
    || metric.current != null
    || metric.official != null
  ))
}

function unitMetrics(model: PlazaModel, group: ModelPlazaGroup, kind: CatalogKind): CatalogMetric[] {
  if (model.platform === 'video') {
    const pricing = model.pricing
    const validMode = pricing?.billing_mode === 'video' || pricing?.billing_mode === 'per_request'
    const resolutions = pricing?.resolution_prices
    // A configured resolution map is authoritative. Missing tiers do not inherit
    // the base price, and unsupported/unselected models are never added here.
    const rows = validMode && resolutions && Object.keys(resolutions).length > 0
      ? [...new Set([...(model.capabilities?.resolutions ?? []), ...Object.keys(resolutions)])]
        .map(tier => ({ tier, value: finitePrice(resolutions[tier]) }))
      : validMode && pricing.intervals?.length
        ? pricing.intervals.map(interval => ({ tier: interval.tier_label?.trim() || '', value: finitePrice(interval.per_request_price) }))
        : [{ tier: model.capabilities?.generic ? '以上游参数为准' : (model.capabilities?.resolutions ?? []).join(' / '), value: validMode ? finitePrice(pricing.per_request_price) : null }]
    return rows.map((row, index) => ({
      key: `${kind}:${row.tier || index}`, label: 'unit', tier: row.tier,
      current: row.value, official: null, saving: null,
    }))
  }
  const intervals = (model.pricing?.intervals ?? [])
    .filter((interval) => finitePrice(interval.per_request_price) != null)
  const finalTiers = new Set(
    (kind === 'image' ? model.final_image_price_tiers : model.final_video_price_tiers)
      ?.map((tier) => tier.trim().toLowerCase()) ?? []
  )
  const rows = intervals.length > 0
    ? intervals.map((interval) => ({
        tier: interval.tier_label?.trim() || '',
        value: finitePrice(interval.per_request_price),
      }))
    : [{ tier: '', value: finitePrice(model.pricing?.per_request_price) }]

  return rows.map((row, index) => {
    const isFinalPrice = model.platform === 'video' || finalTiers.has(row.tier.toLowerCase())
    const current = row.value == null
      ? null
      : row.value * (isFinalPrice ? 1 : groupModelRate(group, kind))
    return {
      key: `${kind}:${row.tier || index}`,
      label: 'unit',
      tier: row.tier,
      current,
      official: null,
      saving: null,
    }
  })
}

function cardFor(group: ModelPlazaGroup, model: PlazaModel): CatalogCard {
  const kind = catalogKind(model)
  const modelId = model.platform === 'video' ? model.name : normalizeModelId(model.name)
  return {
    key: `${model.platform}:${modelId}:${group.id}`,
    modelId,
    modelName: humanizeModelName(modelId),
    platform: model.platform || group.platform,
    kind,
    groupId: group.id,
    groupName: group.name,
    multiplier: groupModelRate(group, kind),
    metrics: kind === 'token' ? tokenMetrics(model, group) : unitMetrics(model, group, kind),
    hasLongContextPricing: (model.pricing?.intervals?.length ?? 0) > 1 && kind === 'token',
    hasTimePricing: group.platform !== 'video' && model.platform !== 'video' && (model.time_pricing?.periods?.length ?? 0) > 0,
  }
}

function unitPriceScore(card: CatalogCard): number {
  const prices = card.metrics
    .map((metric) => metric.current)
    .filter((value): value is number => value != null)
  if (prices.length === 0) return Number.POSITIVE_INFINITY
  return prices.reduce((total, value) => total + value, 0) / prices.length
}

function candidateIsCheaper(next: CatalogCard, current: CatalogCard): boolean {
  if (next.kind === 'token') {
    return next.multiplier < current.multiplier
      || (next.multiplier === current.multiplier && next.groupName.localeCompare(current.groupName) < 0)
  }
  const nextScore = unitPriceScore(next)
  const currentScore = unitPriceScore(current)
  return nextScore < currentScore
    || (nextScore === currentScore && next.multiplier < current.multiplier)
}

function modelKey(model: PlazaModel): string {
  const id = model.platform === 'video' ? model.name : normalizeModelId(model.name).toLowerCase()
  return `${model.platform}:${id}:${catalogKind(model)}`
}

export function visibleCatalogGroups(groups: ModelPlazaGroup[]): ModelPlazaGroup[] {
  return groups.filter((group) => group.name.trim().toLowerCase() !== 'default')
}

export function buildCatalogSections(
  groups: ModelPlazaGroup[],
  selectedGroupId: number | null,
): CatalogSection[] {
  const visibleGroups = visibleCatalogGroups(groups)
  let cards: CatalogCard[] = []

  if (selectedGroupId != null) {
    const selected = visibleGroups.find((group) => group.id === selectedGroupId)
    cards = selected?.models.map((model) => cardFor(selected, model)) ?? []
  } else {
    const bestByModel = new Map<string, CatalogCard>()
    for (const group of visibleGroups) {
      for (const model of group.models) {
        const key = modelKey(model)
        const next = cardFor(group, model)
        const current = bestByModel.get(key)
        if (!current || candidateIsCheaper(next, current)) bestByModel.set(key, next)
      }
    }
    cards = [...bestByModel.values()]
  }

  const sectionMap = new Map<string, CatalogSection>()
  for (const card of cards) {
    const key = `${card.platform}:${card.kind}`
    const section = sectionMap.get(key) ?? { key, platform: card.platform, kind: card.kind, cards: [] }
    section.cards.push(card)
    sectionMap.set(key, section)
  }

  return [...sectionMap.values()].sort((left, right) => {
    const leftRank = SECTION_ORDER[left.key] ?? 999
    const rightRank = SECTION_ORDER[right.key] ?? 999
    return leftRank - rightRank || left.key.localeCompare(right.key)
  })
}
