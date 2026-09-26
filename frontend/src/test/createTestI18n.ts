import { createI18n } from 'vue-i18n'

import en from '@/i18n/locales/en/native'
import zh from '@/i18n/locales/zh/native'

function compileTestMessages(value: unknown): unknown {
  if (typeof value === 'string') {
    return (context: { named: (key: string) => unknown }) => value.replace(
      /\{(\w+)\}/g,
      (_match, key: string) => String(context.named(key) ?? '')
    )
  }
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .map(([key, child]) => [key, compileTestMessages(child)])
  )
}

export function createTestI18n(locale: 'en' | 'zh' = 'zh') {
  return createI18n<Record<string, string>, 'en' | 'zh', false>({
    legacy: false,
    locale,
    fallbackLocale: 'en',
    messages: {
      en: compileTestMessages(en) as Record<string, string>,
      zh: compileTestMessages(zh) as Record<string, string>,
    },
  })
}
