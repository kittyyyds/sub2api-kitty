import { describe, expect, it } from 'vitest'

import {
  LOCALE_KEY,
  LOCALE_SOURCE_KEY,
  detectPreferredLocale,
  localeFromTrace,
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
  it('normalizes browser-style locale aliases for manual switching', () => {
    expect(normalizeLocaleCode('zh-CN')).toBe('zh')
    expect(normalizeLocaleCode('zh_TW')).toBe('zh')
    expect(normalizeLocaleCode('en-US')).toBe('en')
    expect(normalizeLocaleCode('ja-JP')).toBeNull()
  })

  it('keeps a legacy or explicitly manual selection ahead of region signals', async () => {
    await expect(detectPreferredLocale({
      storage: storage({ [LOCALE_KEY]: 'zh' }),
      fetchTrace: async () => 'loc=US',
      timezone: 'America/New_York',
      browserLanguage: 'en-US',
    })).resolves.toBe('zh')

    await expect(detectPreferredLocale({
      storage: storage({ [LOCALE_KEY]: 'en', [LOCALE_SOURCE_KEY]: 'manual' }),
      fetchTrace: async () => 'loc=CN',
      timezone: 'Asia/Shanghai',
      browserLanguage: 'zh-CN',
    })).resolves.toBe('en')
  })

  it('uses Cloudflare country before timezone and browser language', async () => {
    await expect(detectPreferredLocale({
      storage: storage(),
      fetchTrace: async () => 'fl=1\nloc=CN\ntls=TLSv1.3',
      timezone: 'America/Los_Angeles',
      browserLanguage: 'en-US',
    })).resolves.toBe('zh')

    await expect(detectPreferredLocale({
      storage: storage(),
      fetchTrace: async () => 'loc=US',
      timezone: 'Asia/Shanghai',
      browserLanguage: 'zh-CN',
    })).resolves.toBe('en')
  })

  it('falls back from an unavailable trace to timezone and then browser language', async () => {
    await expect(detectPreferredLocale({
      storage: storage(),
      fetchTrace: async () => null,
      timezone: 'Asia/Shanghai',
      browserLanguage: 'en-US',
    })).resolves.toBe('zh')

    await expect(detectPreferredLocale({
      storage: storage(),
      fetchTrace: async () => null,
      timezone: undefined,
      browserLanguage: 'zh-TW',
    })).resolves.toBe('zh')
  })

  it('parses valid trace locations and Chinese-region timezones', () => {
    expect(localeFromTrace('ip=203.0.113.1\nloc=CN')).toBe('zh')
    expect(localeFromTrace('loc=DE')).toBe('en')
    expect(localeFromTrace('<!doctype html>')).toBeNull()
    expect(localeFromTimezone('Asia/Hong_Kong')).toBe('zh')
    expect(localeFromTimezone('Europe/London')).toBe('en')
  })
})
