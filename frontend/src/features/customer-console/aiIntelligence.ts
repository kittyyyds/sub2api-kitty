import type { AIIntelligencePoint } from '@/api/aiIntelligence'

export type AIRecommendationSortKey =
  | 'model'
  | 'effort'
  | 'iq'
  | 'average_minutes'
  | 'average_price_usd'
  | 'pass_rate'
  | 'recommendation_score'

export type SortDirection = 'asc' | 'desc'

export interface AIIntelligenceView {
  recommended: AIIntelligencePoint | null
  smartest: AIIntelligencePoint | null
  bestValue: AIIntelligencePoint | null
  fastest: AIIntelligencePoint | null
  recommendations: AIIntelligencePoint[]
  intelligenceRanking: AIIntelligencePoint[]
  speedRanking: AIIntelligencePoint[]
}

function stablePointKey(point: AIIntelligencePoint): string {
  return `${point.model}:${point.effort}`
}

function descendingMetric(
  points: readonly AIIntelligencePoint[],
  metric: (point: AIIntelligencePoint) => number,
): AIIntelligencePoint[] {
  return [...points].sort((left, right) => (
    metric(right) - metric(left) || stablePointKey(left).localeCompare(stablePointKey(right))
  ))
}

function ascendingMetric(
  points: readonly AIIntelligencePoint[],
  metric: (point: AIIntelligencePoint) => number,
): AIIntelligencePoint[] {
  return [...points].sort((left, right) => (
    metric(left) - metric(right) || stablePointKey(left).localeCompare(stablePointKey(right))
  ))
}

export function passRate(point: AIIntelligencePoint): number {
  if (!Number.isFinite(point.valid_tasks) || point.valid_tasks <= 0) return 0
  return (point.passed / point.valid_tasks) * 100
}

export function valueEfficiency(point: AIIntelligencePoint): number {
  if (!Number.isFinite(point.average_price_usd) || point.average_price_usd <= 0) return 0
  return point.iq / point.average_price_usd
}

export function buildAIIntelligenceView(points: readonly AIIntelligencePoint[]): AIIntelligenceView {
  const valid = points.filter((point) => (
    point.model.startsWith('gpt-')
    && point.model !== 'gpt-5.6-luna'
    && point.effort !== 'ultra'
    && Number.isFinite(point.iq)
    && Number.isFinite(point.average_minutes)
    && Number.isFinite(point.recommendation_score)
  ))
  const byRecommendation = descendingMetric(valid, (point) => point.recommendation_score)
  const candidatePool = byRecommendation.slice(0, 5)

  return {
    recommended: byRecommendation[0] ?? null,
    smartest: descendingMetric(candidatePool, (point) => point.iq)[0] ?? null,
    bestValue: descendingMetric(candidatePool, valueEfficiency)[0] ?? null,
    fastest: ascendingMetric(candidatePool, (point) => point.average_minutes)[0] ?? null,
    recommendations: byRecommendation.slice(0, 10),
    intelligenceRanking: descendingMetric(valid, (point) => point.iq).slice(0, 12),
    speedRanking: ascendingMetric(valid, (point) => point.average_minutes).slice(0, 12),
  }
}

export function sortRecommendationRows(
  points: readonly AIIntelligencePoint[],
  key: AIRecommendationSortKey,
  direction: SortDirection,
): AIIntelligencePoint[] {
  const multiplier = direction === 'asc' ? 1 : -1
  return [...points].sort((left, right) => {
    let compared = 0
    if (key === 'model' || key === 'effort') {
      compared = left[key].localeCompare(right[key])
    } else if (key === 'pass_rate') {
      compared = passRate(left) - passRate(right)
    } else {
      compared = left[key] - right[key]
    }
    return compared * multiplier || stablePointKey(left).localeCompare(stablePointKey(right))
  })
}

export function rankingProgress(value: number, values: number[], inverse = false): number {
  const finiteValues = values.filter(Number.isFinite)
  if (!finiteValues.length || !Number.isFinite(value)) return 12
  const minimum = Math.min(...finiteValues)
  const maximum = Math.max(...finiteValues)
  if (maximum === minimum) return 100
  const normalized = (value - minimum) / (maximum - minimum)
  return 12 + 88 * (inverse ? 1 - normalized : normalized)
}
