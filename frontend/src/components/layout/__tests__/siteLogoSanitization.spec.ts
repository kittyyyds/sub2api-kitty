import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const sidebarSource = readFileSync(resolve(dir, '../AppSidebar.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')
const heyRouteSource = readFileSync(resolve(dir, '../../../views/HomeLandingView.vue'), 'utf8')
const authLayoutSource = readFileSync(resolve(dir, '../../auth/HeyRouteAuthLayout.vue'), 'utf8')

describe('site_logo centralization', () => {
  it('all branded surfaces use the sanitized store-level logo', () => {
    for (const source of [sidebarSource, keyUsageViewSource, heyRouteSource, authLayoutSource]) {
      expect(source).toContain('appStore.brandLogoUrl')
      expect(source).not.toContain('KITTY_BRAND_LOGO_URL')
    }
  })
})
