import { describe, expect, it } from 'vitest'
import {
  buildMonitorTimelineSlots,
  isMonitorCoverageFresh,
  isLatestMonitorBucketCurrent,
  latestMonitorWindowBucketStart,
} from '../timeline'

describe('channel monitor V3 timeline', () => {
  it('builds real time slots and preserves an internal missing bucket', () => {
    const buckets = [
      { bucket_start: '2026-09-01T10:00:00.000Z', value: 'first' },
      { bucket_start: '2026-09-01T10:10:00.000Z', value: 'latest' },
    ]
    const slots = buildMonitorTimelineSlots(
      buckets,
      '2026-09-01T10:15:00.000Z',
      '2026-09-01T10:15:00.000Z',
      300,
      3,
    )

    expect(slots.map(slot => slot.key)).toEqual([
      '2026-09-01T10:00:00.000Z',
      '2026-09-01T10:05:00.000Z',
      '2026-09-01T10:10:00.000Z',
    ])
    expect(slots.map(slot => slot.bucket?.value ?? null)).toEqual(['first', null, 'latest'])
  })

  it('treats the interval ending at data-through as the latest completed bucket', () => {
    expect(latestMonitorWindowBucketStart(undefined, '2026-09-01T10:15:00.000Z', 300))
      .toBe(Date.parse('2026-09-01T10:10:00.000Z'))
    expect(isLatestMonitorBucketCurrent(
      '2026-09-01T10:10:00.000Z',
      undefined,
      '2026-09-01T10:15:00.000Z',
      300,
    )).toBe(true)
  })

  it('rejects a stale or invalid bucket as the current health signal', () => {
    expect(isLatestMonitorBucketCurrent('2026-09-01T09:55:00.000Z', undefined, '2026-09-01T10:15:00.000Z', 300)).toBe(false)
    expect(isLatestMonitorBucketCurrent('invalid', undefined, '2026-09-01T10:15:00.000Z', 300)).toBe(false)
    expect(isLatestMonitorBucketCurrent('2026-09-01T10:10:00.000Z', undefined, '', 300)).toBe(false)
  })

  it('anchors the timeline to requested-end and leaves trailing gaps unknown', () => {
    const slots = buildMonitorTimelineSlots(
      [{ bucket_start: '2026-09-01T10:05:00.000Z' }],
      '2026-09-01T10:15:00.000Z',
      '2026-09-01T10:10:00.000Z',
      300,
      2,
    )

    expect(slots.map(slot => slot.key)).toEqual([
      '2026-09-01T10:05:00.000Z',
      '2026-09-01T10:10:00.000Z',
    ])
    expect(slots.map(slot => Boolean(slot.bucket))).toEqual([true, false])
  })

  it('expires a watermark after two refresh cycles, including local elapsed time', () => {
    const base = {
      dataThrough: '2026-09-01T10:10:00.000Z',
      computedAt: '2026-09-01T10:10:01.000Z',
      aggregationLagSeconds: 50,
      refreshIntervalSeconds: 60,
      observedAtMs: 1_000_000,
    }
    expect(isMonitorCoverageFresh({ ...base, nowMs: 1_060_000 })).toBe(true)
    expect(isMonitorCoverageFresh({ ...base, nowMs: 1_071_000 })).toBe(false)
    expect(isMonitorCoverageFresh({ ...base, computedAt: '0001-01-01T00:00:00Z', nowMs: 1_000_000 })).toBe(false)
  })
})
