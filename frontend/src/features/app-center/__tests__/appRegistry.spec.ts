import { describe, expect, it } from 'vitest'

import { APPLICATION_SLUGS } from '../types'
import { APP_PRESENTATIONS, isSupportedApplicationSlug } from '../appRegistry'

describe('application center registry', () => {
  it('includes the client-managed free media studio beside the planned workflows', () => {
    expect(APPLICATION_SLUGS).toContain('free-media-studio')
    expect(APP_PRESENTATIONS['free-media-studio']).toMatchObject({
      name: '自由媒体工作台',
      category: 'media',
      requiresReference: false,
      acceptsMultipleReferences: true,
    })
    expect(isSupportedApplicationSlug('free-media-studio')).toBe(true)
  })

  it('keeps every declared slug backed by a presentation', () => {
    expect(Object.keys(APP_PRESENTATIONS)).toEqual([...APPLICATION_SLUGS])
  })

  it('uses a distinct icon for every customer-facing application', () => {
    expect(Object.values(APP_PRESENTATIONS).map((presentation) => presentation.icon)).toEqual([
      'swatches',
      'lightbulb',
      'shoppingBag',
      'videoCamera',
    ])
  })
})
