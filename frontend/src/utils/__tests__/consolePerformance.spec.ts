import { describe, expect, it } from 'vitest'

import { resolveConsolePerformanceMode } from '../consolePerformance'

describe('resolveConsolePerformanceMode', () => {
  it('selects lite mode for explicit low-resource or user preference signals', () => {
    expect(resolveConsolePerformanceMode({ hardwareConcurrency: 4 })).toBe('lite')
    expect(resolveConsolePerformanceMode({ deviceMemory: 4 })).toBe('lite')
    expect(resolveConsolePerformanceMode({ saveData: true })).toBe('lite')
    expect(resolveConsolePerformanceMode({ reducedMotion: true })).toBe('lite')
  })

  it('keeps full styling for capable devices and unknown capability values', () => {
    expect(resolveConsolePerformanceMode({ hardwareConcurrency: 8, deviceMemory: 8 })).toBe('full')
    expect(resolveConsolePerformanceMode({})).toBe('full')
  })
})
