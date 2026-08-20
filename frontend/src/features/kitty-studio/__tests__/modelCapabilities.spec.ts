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
  it('keeps GPT image generation limited to gpt-image-2', () => {
    expect(studioModelOptions('openai', ['gpt-5.6', GPT_IMAGE_MODEL], 'text-image').map((item) => item.id))
      .toEqual([GPT_IMAGE_MODEL])
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
