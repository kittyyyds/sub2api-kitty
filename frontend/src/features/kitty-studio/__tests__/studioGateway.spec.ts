import { describe, expect, it } from 'vitest'

import { extractGeminiImages, extractSanitizedSvg, imageDimensions, videoResolutionAllowed } from '../studioGateway'
import { GROK_VIDEO_15_MODEL, GROK_VIDEO_MODEL } from '../modelCapabilities'

describe('Kitty Studio gateway helpers', () => {
  it('maps native resolution and ratio without browser upscaling', () => {
    expect(imageDimensions({ ratio: '1:1', resolution: '1k' })).toBe('1024x1024')
    expect(imageDimensions({ ratio: '16:9', resolution: '4k' })).toBe('3840x2160')
    expect(imageDimensions({ ratio: '9:16', resolution: '2k' })).toBe('1152x2048')
  })

  it('extracts Gemini inline image parts', () => {
    expect(extractGeminiImages({ candidates: [{ content: { parts: [{ inlineData: { mimeType: 'image/png', data: 'YWJj' } }] } }] }))
      .toEqual([{ type: 'image', source: 'data:image/png;base64,YWJj', mimeType: 'image/png' }])
  })

  it('sanitizes Claude SVG output', () => {
    const svg = extractSanitizedSvg('```svg\n<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><rect width="10" height="10"/></svg>\n```')
    expect(svg).toContain('<rect')
    expect(svg).not.toContain('<script')
  })

  it('only enables 1080p for Grok Imagine Video 1.5', () => {
    expect(videoResolutionAllowed(GROK_VIDEO_MODEL, '1080p')).toBe(false)
    expect(videoResolutionAllowed(GROK_VIDEO_15_MODEL, '1080p')).toBe(true)
  })
})
