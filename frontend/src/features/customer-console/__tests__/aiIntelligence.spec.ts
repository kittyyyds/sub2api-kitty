import { describe, expect, it } from 'vitest'

import type { AIIntelligencePoint } from '@/api/aiIntelligence'
import {
  buildAIIntelligenceView,
  passRate,
  rankingProgress,
  sortRecommendationRows,
} from '../aiIntelligence'

function point(
  model: string,
  effort: string,
  iq: number,
  minutes: number,
  price: number,
): AIIntelligencePoint {
  return {
    model,
    effort,
    iq,
    passed: 63,
    valid_tasks: 112,
    average_price_usd: price,
    average_minutes: minutes,
    latest_graded_at: '2026-08-15T10:00:00Z',
    recommendation_score: iq - 1.5 * minutes,
  }
}

describe('GPT intelligence test recommendations', () => {
  const points = [
    point('gpt-balanced', 'low', 90, 10, 2),
    point('gpt-smart', 'xhigh', 110, 25, 7),
    point('gpt-value', 'medium', 86, 9, 0.5),
    point('gpt-fast', 'low', 80, 4, 1.5),
    point('gpt-fifth', 'high', 95, 18, 3),
    point('gpt-outside-pool', 'max', 130, 45, 1),
    point('gpt-5.6-luna', 'low', 150, 3, 1),
    point('gpt-ultra', 'ultra', 140, 5, 1),
    point('claude-test', 'high', 120, 8, 1),
  ]

  it('uses recommendation score for the top-five candidate pool', () => {
    const view = buildAIIntelligenceView(points)

    expect(view.recommended?.model).toBe('gpt-balanced')
    expect(view.smartest?.model).toBe('gpt-smart')
    expect(view.bestValue?.model).toBe('gpt-value')
    expect(view.fastest?.model).toBe('gpt-fast')
    expect(view.recommendations.map((item) => item.model)).not.toContain('gpt-5.6-luna')
    expect(view.recommendations.map((item) => item.model)).not.toContain('gpt-ultra')
    expect(view.recommendations.map((item) => item.model)).not.toContain('claude-test')
  })

  it('builds independent intelligence and speed rankings', () => {
    const view = buildAIIntelligenceView(points)

    expect(view.intelligenceRanking[0].model).toBe('gpt-outside-pool')
    expect(view.speedRanking[0].model).toBe('gpt-fast')
    expect(view.speedRanking.at(-1)?.model).toBe('gpt-outside-pool')
  })

  it('sorts table rows and computes pass rates deterministically', () => {
    const rows = buildAIIntelligenceView(points).recommendations
    expect(sortRecommendationRows(rows, 'average_price_usd', 'asc')[0].model).toBe('gpt-value')
    expect(sortRecommendationRows(rows, 'iq', 'desc')[0].model).toBe('gpt-outside-pool')
    expect(passRate(rows[0])).toBeCloseTo(56.25)
  })

  it('keeps ranking bars within the intended visual range', () => {
    expect(rankingProgress(10, [10, 20, 30])).toBe(12)
    expect(rankingProgress(30, [10, 20, 30])).toBe(100)
    expect(rankingProgress(10, [10, 20, 30], true)).toBe(100)
    expect(rankingProgress(30, [10, 20, 30], true)).toBe(12)
  })
})
