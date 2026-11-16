import { describe, expect, it } from 'vitest'
import { getBundledGuideChapters } from '../guideSections'
describe('Kitty bundled guide', () => {
  it('loads the default chapters from frontend/docs without sharing mutable state', () => {
    const chapters = getBundledGuideChapters()
    expect(chapters.map(c => c.slug)).toEqual(['start', 'api-key', 'help'])
    expect(chapters.every(c => c.title && c.content.startsWith('## '))).toBe(true)
    chapters[0].title = 'changed'
    expect(getBundledGuideChapters()[0].title).not.toBe('changed')
  })
})
