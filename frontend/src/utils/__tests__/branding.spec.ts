import { beforeEach, describe, expect, it } from 'vitest'
import {
  KITTY_BRAND_APPLE_TOUCH_ICON_URL,
  KITTY_BRAND_FAVICON_URL,
  KITTY_BRAND_LOGO_URL,
  resolveBrandLogoUrl,
  resolveFaviconUrl,
  updateFavicon,
} from '@/utils/branding'

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = [
      '<link rel="icon" href="/favicon.svg">',
      '<link rel="icon" href="/legacy-favicon.svg">',
      '<link rel="apple-touch-icon" href="/apple-touch-icon.svg">',
      '<link rel="apple-touch-icon" href="/legacy-touch-icon.svg">',
    ].join('')
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const links = document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]')
    const appleTouchIcons = document.querySelectorAll<HTMLLinkElement>('link[rel="apple-touch-icon"]')
    expect(Array.from(links, (link) => link.href)).toEqual([
      'https://example.com/custom-logo.png',
      'https://example.com/custom-logo.png',
    ])
    expect(Array.from(appleTouchIcons, (link) => link.href)).toEqual([
      'https://example.com/custom-logo.png',
      'https://example.com/custom-logo.png',
    ])
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    const appleTouchIcon = document.querySelector<HTMLLinkElement>('link[rel="apple-touch-icon"]')
    expect(link?.getAttribute('href')).toBe(KITTY_BRAND_FAVICON_URL)
    expect(link?.type).toBe('image/svg+xml')
    expect(appleTouchIcon?.getAttribute('href')).toBe(KITTY_BRAND_APPLE_TOUCH_ICON_URL)
  })

  it('sets the MIME type from a URL without being confused by its query string', () => {
    updateFavicon('/uploads/brand.png?v=2')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.type).toBe('image/png')
  })
})

describe('resolveBrandLogoUrl', () => {
  it('uses the configured site logo across the application', () => {
    expect(resolveBrandLogoUrl('/uploads/custom-logo.svg')).toBe('/uploads/custom-logo.svg')
  })

  it.each(['', 'javascript:alert(1)', '//example.com/logo.svg'])(
    'falls back to the bundled logo for %j',
    (configuredLogoUrl) => {
      expect(resolveBrandLogoUrl(configuredLogoUrl)).toBe(KITTY_BRAND_LOGO_URL)
    },
  )
})

describe('resolveFaviconUrl', () => {
  it('uses the configured site logo on every route', () => {
    expect(resolveFaviconUrl('https://example.com/custom-logo.png'))
      .toBe('https://example.com/custom-logo.png')
  })

  it('falls back to the kitty favicon when no configured logo exists', () => {
    expect(resolveFaviconUrl('')).toBe(KITTY_BRAND_FAVICON_URL)
  })
})
