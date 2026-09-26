import { sanitizeUrl } from '@/utils/url'

export const KITTY_BRAND_LOGO_URL = '/logo.svg'
export const KITTY_BRAND_FAVICON_URL = '/favicon.svg'
export const KITTY_BRAND_APPLE_TOUCH_ICON_URL = '/apple-touch-icon.svg'

function sanitizeBrandImageUrl(value: string): string {
  return sanitizeUrl(value, {
    allowRelative: true,
    allowDataUrl: true,
  })
}

export function resolveBrandLogoUrl(configuredLogoUrl: string): string {
  return sanitizeBrandImageUrl(configuredLogoUrl) || KITTY_BRAND_LOGO_URL
}

export function resolveFaviconUrl(configuredLogoUrl: string, fallbackUrl = KITTY_BRAND_FAVICON_URL): string {
  return sanitizeBrandImageUrl(configuredLogoUrl) || fallbackUrl
}

function resolveImageMimeType(imageUrl: string): string {
  const dataType = imageUrl.match(/^data:(image\/[a-z0-9.+-]+)[;,]/i)?.[1]
  if (dataType) return dataType.toLowerCase()

  const pathname = imageUrl.split(/[?#]/, 1)[0].toLowerCase()
  if (pathname.endsWith('.svg')) return 'image/svg+xml'
  if (pathname.endsWith('.png')) return 'image/png'
  if (pathname.endsWith('.webp')) return 'image/webp'
  if (pathname.endsWith('.jpg') || pathname.endsWith('.jpeg')) return 'image/jpeg'
  if (pathname.endsWith('.gif')) return 'image/gif'
  return 'image/x-icon'
}

export function updateFavicon(logoUrl: string): void {
  const faviconUrl = resolveFaviconUrl(logoUrl)
  const appleTouchIconUrl = resolveFaviconUrl(logoUrl, KITTY_BRAND_APPLE_TOUCH_ICON_URL)

  const faviconLinks = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]'))
  if (faviconLinks.length === 0) {
    const link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
    faviconLinks.push(link)
  }

  faviconLinks.forEach((link) => {
    link.type = resolveImageMimeType(faviconUrl)
    link.href = faviconUrl
  })

  const appleTouchIcons = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="apple-touch-icon"]'))
  if (appleTouchIcons.length === 0) {
    const link = document.createElement('link')
    link.rel = 'apple-touch-icon'
    document.head.appendChild(link)
    appleTouchIcons.push(link)
  }
  appleTouchIcons.forEach((link) => {
    link.href = appleTouchIconUrl
  })
}
