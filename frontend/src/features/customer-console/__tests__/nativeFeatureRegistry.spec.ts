import { describe, expect, it } from 'vitest'

import {
  NATIVE_CUSTOM_FEATURES,
  getNativeFeature,
  getNativeFeatureComponent,
  isNativeCustomFeatureId,
  isReservedCustomerFeatureId
} from '../nativeFeatureRegistry'

describe('native customer feature registry', () => {
  it('keeps the remaining native customer tools in sidebar order', () => {
    expect(NATIVE_CUSTOM_FEATURES.map(({ id, path }) => ({ id, path }))).toEqual([
      { id: 'reset-radar', path: '/custom/reset-radar' },
      { id: 'billing-rules', path: '/custom/billing-rules' },
      { id: 'docs', path: '/custom/docs' }
    ])
  })

  it('resolves only known IDs to native async components', () => {
    for (const feature of NATIVE_CUSTOM_FEATURES) {
      expect(isNativeCustomFeatureId(feature.id)).toBe(true)
      expect(getNativeFeature(feature.id)).toMatchObject({ id: feature.id })
      expect(getNativeFeatureComponent(feature.id)).toBeTruthy()
    }

    expect(isNativeCustomFeatureId('support')).toBe(false)
    expect(isNativeCustomFeatureId('image-generator')).toBe(false)
    expect(isReservedCustomerFeatureId('image-generator')).toBe(true)
    expect(getNativeFeature('support')).toBeNull()
    expect(getNativeFeatureComponent('support')).toBeNull()
  })
})
