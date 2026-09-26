export type ConsolePerformanceMode = 'full' | 'lite'

export interface ConsolePerformanceSignals {
  hardwareConcurrency?: number
  deviceMemory?: number
  saveData?: boolean
  reducedMotion?: boolean
}

export function resolveConsolePerformanceMode(
  signals: ConsolePerformanceSignals,
): ConsolePerformanceMode {
  if (signals.saveData || signals.reducedMotion) return 'lite'
  if (typeof signals.hardwareConcurrency === 'number' && signals.hardwareConcurrency <= 4) return 'lite'
  if (typeof signals.deviceMemory === 'number' && signals.deviceMemory <= 4) return 'lite'
  return 'full'
}

interface NavigatorWithPerformanceHints extends Navigator {
  deviceMemory?: number
  connection?: {
    saveData?: boolean
  }
}

export function detectConsolePerformanceMode(): ConsolePerformanceMode {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') return 'full'
  const nav = navigator as NavigatorWithPerformanceHints
  return resolveConsolePerformanceMode({
    hardwareConcurrency: nav.hardwareConcurrency,
    deviceMemory: nav.deviceMemory,
    saveData: nav.connection?.saveData,
    reducedMotion: window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true,
  })
}
