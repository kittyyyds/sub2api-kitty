import { describe, expect, it } from 'vitest'

import { heyRouteCopyByLanguage, withSiteName } from '../heyRouteCopy'

describe('withSiteName', () => {
  it('shows the configured site name everywhere the copy names the brand', () => {
    const copy = withSiteName(heyRouteCopyByLanguage.zh, '星河 API')
    const text = JSON.stringify(copy)

    expect(text).not.toContain('Kitty奇妙屋')
    expect(text).toContain('星河 API')
    expect(copy.pricing.kittyRate).toBe('星河 API最低倍率')
    expect(copy.nav.sections).toHaveLength(heyRouteCopyByLanguage.zh.nav.sections.length)
    // The source copy stays untouched for the next render.
    expect(JSON.stringify(heyRouteCopyByLanguage.zh)).toContain('Kitty奇妙屋')
  })

  it('returns the original copy when the name is the default or empty', () => {
    expect(withSiteName(heyRouteCopyByLanguage.en, 'Kitty奇妙屋')).toBe(heyRouteCopyByLanguage.en)
    expect(withSiteName(heyRouteCopyByLanguage.en, '  ')).toBe(heyRouteCopyByLanguage.en)
  })
})
