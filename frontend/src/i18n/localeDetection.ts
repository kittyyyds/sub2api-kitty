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

export function localeFromTimezone(timezone: string | undefined): LocaleCode | null {
  if (!timezone) return null
  return CHINESE_TIMEZONES.has(timezone) ? 'zh' : 'en'
}

export function localeFromBrowserLanguage(language: string | undefined): LocaleCode {
  return language?.toLowerCase().startsWith('zh') ? 'zh' : DEFAULT_LOCALE
}

interface DetectLocaleOptions {
  storage: Pick<Storage, 'getItem'>
  timezone?: string
  browserLanguage?: string
}

// Decided from local signals only, so the first screen never waits on the network.
// The site is not behind Cloudflare, so /cdn-cgi/trace only returned the SPA page
// and delayed every visit by up to 1.5 seconds without ever yielding a country.
export function detectPreferredLocale({
  storage,
  timezone,
  browserLanguage,
}: DetectLocaleOptions): LocaleCode {
  const saved = getSavedManualLocale(storage)
  if (saved) return saved

  return localeFromTimezone(timezone) ?? localeFromBrowserLanguage(browserLanguage)
}

export function getSystemTimezone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return undefined
  }
}
