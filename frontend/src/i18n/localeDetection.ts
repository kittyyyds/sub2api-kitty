export type LocaleCode = 'en' | 'zh'

export const LOCALE_KEY = 'sub2api_locale'
export const LOCALE_SOURCE_KEY = 'sub2api_locale_source'
export const MANUAL_LOCALE_SOURCE = 'manual'
export const DEFAULT_LOCALE: LocaleCode = 'en'

const CHINESE_TIMEZONES = new Set([
  'Asia/Shanghai',
  'Asia/Chongqing',
  'Asia/Harbin',
  'Asia/Urumqi',
  'Asia/Hong_Kong',
  'Asia/Macau',
  'Asia/Taipei',
])

export function isLocaleCode(value: unknown): value is LocaleCode {
  return value === 'en' || value === 'zh'
}

export function normalizeLocaleCode(value: unknown): LocaleCode | null {
  if (typeof value !== 'string') return null

  const normalized = value.trim().toLowerCase().replace(/_/g, '-')
  if (normalized === 'zh' || normalized.startsWith('zh-')) return 'zh'
  if (normalized === 'en' || normalized.startsWith('en-')) return 'en'
  return null
}

export function getSavedManualLocale(storage: Pick<Storage, 'getItem'>): LocaleCode | null {
  const saved = storage.getItem(LOCALE_KEY)
  if (!isLocaleCode(saved)) return null

  // Locale values saved by older releases only came from the language switcher.
  const source = storage.getItem(LOCALE_SOURCE_KEY)
  return !source || source === MANUAL_LOCALE_SOURCE ? saved : null
}

export function localeFromTrace(trace: string): LocaleCode | null {
  const locationLine = trace
    .split(/\r?\n/)
    .find((line) => line.trim().toLowerCase().startsWith('loc='))
  if (!locationLine) return null

  const countryCode = locationLine.slice(locationLine.indexOf('=') + 1).trim().toUpperCase()
  if (!/^[A-Z]{2}$/.test(countryCode)) return null
  return countryCode === 'CN' ? 'zh' : 'en'
}

export function localeFromTimezone(timezone: string | undefined): LocaleCode | null {
  if (!timezone) return null
  return CHINESE_TIMEZONES.has(timezone) ? 'zh' : 'en'
}

export function localeFromBrowserLanguage(language: string | undefined): LocaleCode {
  return language?.toLowerCase().startsWith('zh') ? 'zh' : DEFAULT_LOCALE
}

interface DetectLocaleOptions {
  storage: Pick<Storage, 'getItem'>
  fetchTrace?: () => Promise<string | null>
  timezone?: string
  browserLanguage?: string
}

export async function detectPreferredLocale({
  storage,
  fetchTrace,
  timezone,
  browserLanguage,
}: DetectLocaleOptions): Promise<LocaleCode> {
  const saved = getSavedManualLocale(storage)
  if (saved) return saved

  if (fetchTrace) {
    try {
      const trace = await fetchTrace()
      const networkLocale = trace ? localeFromTrace(trace) : null
      if (networkLocale) return networkLocale
    } catch {
      // Region detection is optional; local signals below keep startup reliable.
    }
  }

  return localeFromTimezone(timezone) ?? localeFromBrowserLanguage(browserLanguage)
}

export async function fetchCloudflareTrace(timeoutMs = 1500): Promise<string | null> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), timeoutMs)

  try {
    const response = await fetch('/cdn-cgi/trace', {
      cache: 'no-store',
      credentials: 'omit',
      signal: controller.signal,
    })
    if (!response.ok) return null
    return await response.text()
  } catch {
    return null
  } finally {
    window.clearTimeout(timeout)
  }
}

export function getSystemTimezone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return undefined
  }
}
