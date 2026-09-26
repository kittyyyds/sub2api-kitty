import { describe, expect, it } from 'vitest'

import {
  GPT_IMAGE_MODEL,
  GROK_IMAGE_20_MODEL,
  GROK_IMAGE_MODEL,
  GROK_IMAGE_QUALITY_MODEL,
  GROK_VIDEO_15_MODEL,
  GROK_VIDEO_MODEL,
  studioModelOptions,
} from '../modelCapabilities'

describe('Kitty Studio model catalog', () => {
  it('uses only advertised exact video models without enabling image modes', () => {
    const models = ['Custom-Video', 'Custom-Video', '*', 'other-video']
    const options = studioModelOptions('video', models, 'text-video')
    expect(options.map(option => option.id)).toEqual(['Custom-Video', 'other-video'])
    expect(options.every(option => option.strategy === 'video-gateway')).toBe(true)
    expect(studioModelOptions('video', models, 'text-image')).toEqual([])
    expect(studioModelOptions('video', [], 'image-video')).toEqual([])
  })
  it('keeps Image 2 for groups which only advertise the old model', () => {
    expect(studioModelOptions('openai', ['gpt-5.6', GPT_IMAGE_MODEL], 'text-image').map((item) => item.id))
      .toEqual([GPT_IMAGE_MODEL])
  })

  it.each(['text-image', 'image-image'] as const)('offers advertised Image 2.5 variants in %s mode', (mode) => {
    const models = ['gpt-image-2', 'gpt-image-2.5', 'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst']
    const options = studioModelOptions('openai', ['gpt-6-astra', ...models, ' gpt-image-2.5 '], mode)
    expect(options.map((item) => item.id)).toEqual(models)
    expect(options.map((item) => item.label)).toEqual([
      'GPT Image 2', 'GPT Image 2.5', 'GPT Image 2.5 Flare', 'GPT Image 2.5 Sunburst',
    ])
    expect(options.every((item) => item.strategy === 'openai-image')).toBe(true)
  })

  it('keeps exact dated IDs and does not invent unavailable GPT variants', () => {
    const id = 'gpt-image-2.5-flare-2026-09-08'
    expect(studioModelOptions('openai', [id], 'image-image').map((item) => item.id)).toEqual([id])
    expect(studioModelOptions('openai', [], 'text-image')).toEqual([])
    expect(studioModelOptions('openai', ['gpt-6', 'gpt-image-25', 'gpt-image-2.50'], 'text-image')).toEqual([])
    expect(studioModelOptions('openai', [id], 'text-video')).toEqual([])
    expect(studioModelOptions('composite', [id], 'text-image').filter((item) => item.id.startsWith('gpt-')).map((item) => item.id)).toEqual([id])
  })

  it('exposes the three approved Grok image models', () => {
    expect(studioModelOptions('grok', [], 'text-image').map((item) => item.id)).toEqual([
      GROK_IMAGE_MODEL,
      GROK_IMAGE_20_MODEL,
      GROK_IMAGE_QUALITY_MODEL,
    ])
  })

  it('exposes exactly the two approved Grok video models', () => {
    expect(studioModelOptions('grok', [], 'text-video').map((item) => item.id)).toEqual([
      GROK_VIDEO_MODEL,
      GROK_VIDEO_15_MODEL,
    ])
  })
})
