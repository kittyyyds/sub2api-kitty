import { describe, expect, it } from 'vitest'
import { buildSuanliVideoRequest, isExclusiveSuanliModelCatalog, SUANLI_VIDEO_MODELS, suanliPixelSize } from '../suanliVideo'

const input = {
  modelID: '特价/官方h3-720p', prompt: '让参考主体自然转身', duration: 5, ratio: '16:9' as const,
  imageURLs: ['https://example.com/image.jpg'], audioURLs: [] as string[],
}

describe('Suanli special video contracts', () => {
  it('recognizes only a nonempty dedicated catalog and leaves Grok or mixed groups unchanged', () => {
    expect(isExclusiveSuanliModelCatalog(SUANLI_VIDEO_MODELS.map((model) => model.id))).toBe(true)
    expect(isExclusiveSuanliModelCatalog([])).toBe(false)
    expect(isExclusiveSuanliModelCatalog(['grok-imagine-video'])).toBe(false)
    expect(isExclusiveSuanliModelCatalog([input.modelID, 'grok-imagine-video'])).toBe(false)
  })

  it('keeps the exact Chinese Seedance model name and enforces the first-release single-image scope', () => {
    const modelID = '官转稳定sd2.0(933原生过真人480P)'
    const request = buildSuanliVideoRequest({ ...input, modelID })
    expect(request).toEqual({ model: modelID, prompt: input.prompt, seconds: 5, size: '854x480', reference_images: [{ url: input.imageURLs[0], role: 'reference_image' }] })
    expect(() => buildSuanliVideoRequest({ ...input, modelID, imageURLs: [...input.imageURLs, 'https://example.com/two.jpg'] })).toThrow('1–1')
  })

  it('submits special SD2.5 as a fixed 30-second contract without leaking duration or H3 fields', () => {
    const request = buildSuanliVideoRequest({ ...input, modelID: '特价/sd-2.5', duration: 30 })
    expect(request).toEqual({ model: '特价/sd-2.5', prompt: input.prompt, resolution: '720p', aspect_ratio: '16:9', images: input.imageURLs })
    expect(request).not.toHaveProperty('seconds')
    expect(request).not.toHaveProperty('duration')
    expect(request).not.toHaveProperty('reference_videos')
  })

  it('retains all nine H3 images and three audio references', () => {
    const imageURLs = Array.from({ length: 9 }, (_, index) => `https://example.com/${index}.jpg`)
    const audioURLs = Array.from({ length: 3 }, (_, index) => `https://example.com/${index}.mp3`)
    const request = buildSuanliVideoRequest({ ...input, modelID: '特价/官方h3-2k', imageURLs, audioURLs, duration: 15 })
    expect(request.size).toBe('2560x1440')
    expect(request.reference_images).toHaveLength(9)
    expect(request.reference_audios).toHaveLength(3)
    expect(request).not.toHaveProperty('reference_videos')
  })

  it.each([4, 16, 5.5])('rejects unsupported test duration %s before a billable request', (duration) => {
    expect(() => buildSuanliVideoRequest({ ...input, duration })).toThrow('5–15')
  })

  it('rejects absent references, non-public URL syntax, and unsupported audio before submission', () => {
    expect(() => buildSuanliVideoRequest({ ...input, imageURLs: [] })).toThrow('参考图')
    expect(() => buildSuanliVideoRequest({ ...input, imageURLs: ['file:///tmp/ref.jpg'] })).toThrow('HTTP')
    expect(() => buildSuanliVideoRequest({ ...input, modelID: '特价/sd-2.5', audioURLs: ['https://example.com/a.mp3'] })).toThrow('0')
  })

  it('maps portrait, landscape, and square independently without cross-model resolution state', () => {
    expect(suanliPixelSize('720p', '9:16')).toBe('720x1280')
    expect(suanliPixelSize('1080p', '16:9')).toBe('1920x1080')
    expect(suanliPixelSize('2K', '1:1')).toBe('1440x1440')
  })
})
