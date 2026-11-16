import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import en from '../locales/en/native'
import zh from '../locales/zh/native'

function leafPaths(value: unknown, prefix = ''): string[] {
  if (typeof value === 'string') return [prefix]
  if (!value || typeof value !== 'object') return []
  return Object.entries(value as Record<string, unknown>)
    .flatMap(([key, child]) => leafPaths(child, prefix ? `${prefix}.${key}` : key))
    .sort()
}

describe('native customer-console locales', () => {
  it('keeps the English and Chinese message trees in sync', () => {
    expect(leafPaths(en)).toEqual(leafPaths(zh))
  })

  it('does not leave Chinese UI copy in the custom Vue panels', () => {
    const panels = [
      '../../components/customer-console/billing/BillingEstimator.vue',
      '../../views/user/native/BillingRulesPanel.vue',
      '../../views/user/native/DocumentationPanel.vue',
    ]

    for (const panel of panels) {
      const source = readFileSync(resolve(__dirname, panel), 'utf8')
      expect(source, panel).not.toMatch(/[\u4e00-\u9fff]/)
    }
  })
})
