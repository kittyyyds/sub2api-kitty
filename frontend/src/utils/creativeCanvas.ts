export function getCreativeCanvasUrl(siteVersion?: string): string {
  return `/canvas/canvas?kitty_version=${encodeURIComponent(siteVersion || 'v0.8.0-kitty.2')}`
}
