import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  LOCALE_KEY,
  LOCALE_SOURCE_KEY,
  detectPreferredLocale,
  localeFromTimezone,
  normalizeLocaleCode,
} from '../localeDetection'

function storage(values: Record<string, string> = {}): Pick<Storage, 'getItem'> {
  return {
    getItem(key: string) {
      return values[key] ?? null
    },
  }
}

describe('locale detection', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('normalizes browser-style locale aliases for manual switching', () => {
    expect(normalizeLocaleCode('zh-CN')).toBe('zh')
    expect(normalizeLocaleCode('zh_TW')).toBe('zh')
    expect(normalizeLocaleCode('en-US')).toBe('en')
    expect(normalizeLocaleCode('ja-JP')).toBeNull()
  })

  it('keeps a legacy or explicitly manual selection ahead of local signals', () => {
    expect(detectPreferredLocale({
      storage: storage({ [LOCALE_KEY]: 'zh' }),
      timezone: 'America/New_York',
      browserLanguage: 'en-US',
    })).toBe('zh')

    expect(detectPreferredLocale({
      storage: storage({ [LOCALE_KEY]: 'en', [LOCALE_SOURCE_KEY]: 'manual' }),
      timezone: 'Asia/Shanghai',
      browserLanguage: 'zh-CN',
    })).toBe('en')
  })

  it('uses the timezone, then the browser language', () => {
    expect(detectPreferredLocale({
      storage: storage(),
      timezone: 'Asia/Shanghai',
      browserLanguage: 'en-US',
    })).toBe('zh')

    expect(detectPreferredLocale({
      storage: storage(),
      timezone: 'America/Los_Angeles',
      browserLanguage: 'zh-CN',
    })).toBe('en')

    expect(detectPreferredLocale({
      storage: storage(),
      timezone: undefined,
      browserLanguage: 'zh-TW',
    })).toBe('zh')

    expect(detectPreferredLocale({
      storage: storage(),
      timezone: undefined,
      browserLanguage: undefined,
    })).toBe('en')
  })

  it('decides without any network request, so the first screen never waits', () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)

    const locale = detectPreferredLocale({ storage: storage(), timezone: 'Asia/Shanghai', browserLanguage: 'zh-CN' })

    expect(locale).toBe('zh')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('maps Chinese-region timezones', () => {
    expect(localeFromTimezone('Asia/Hong_Kong')).toBe('zh')
    expect(localeFromTimezone('Asia/Taipei')).toBe('zh')
    expect(localeFromTimezone('Europe/London')).toBe('en')
    expect(localeFromTimezone(undefined)).toBeNull()
  })
})
