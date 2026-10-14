import { describe, expect, it } from 'vitest'
import { isVideoUsage, videoUnitPrice } from '../videoUsage'

describe('video usage receipts', () => {
  it('recognizes per-video billing and legacy per-second rows without guessing model names', () => {
    expect(isVideoUsage({ video_count: 1, billing_mode: 'per_request' })).toBe(true)
    expect(isVideoUsage({ billing_mode: 'video' })).toBe(true)
    expect(isVideoUsage({ billing_mode: 'per_request' })).toBe(false)
    expect(isVideoUsage({ video_count: 0, billing_mode: 'image' })).toBe(false)
  })

  it('divides the receipt amount by its exact billing units', () => {
    expect(videoUnitPrice({ video_count: 1, video_duration_seconds: 5, billing_mode: 'video', total_cost: 3.75 })).toBe(.75)
    expect(videoUnitPrice({ video_count: 2, video_duration_seconds: 30, billing_mode: 'per_request', total_cost: 2.6 })).toBe(1.3)
    expect(videoUnitPrice({ video_count: 2, video_duration_seconds: 5, billing_mode: 'video', total_cost: 7.5 })).toBe(.75)
    expect(videoUnitPrice({ video_count: 1, billing_mode: 'per_request', total_cost: 0 })).toBe(0)
  })

  it('keeps missing historical units and unsupported modes unknown', () => {
    expect(videoUnitPrice({ billing_mode: 'video', total_cost: 3.75 })).toBeNull()
    expect(videoUnitPrice({ video_count: 1, billing_mode: 'video', total_cost: 3.75 })).toBeNull()
    expect(videoUnitPrice({ video_count: 1, billing_mode: 'token', total_cost: 3.75 })).toBeNull()
    expect(videoUnitPrice({ video_count: 1, billing_mode: 'per_request' })).toBeNull()
    expect(videoUnitPrice({ video_count: 1, billing_mode: 'per_request', total_cost: NaN })).toBeNull()
  })
})
