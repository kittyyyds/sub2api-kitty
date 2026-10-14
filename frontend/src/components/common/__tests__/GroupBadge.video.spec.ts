import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: null }) }))

describe('video group rate badges', () => {
  it('shows the final price label without misleading personal or peak multipliers', () => {
    const wrapper = mount(GroupBadge, {
      props: {
        name: 'Video', platform: 'video', rateMultiplier: 3, userRateMultiplier: 7,
        peakRateEnabled: true, peakStart: '08:00', peakEnd: '20:00', peakRateMultiplier: 5,
      },
      global: { stubs: { PlatformIcon: true } },
    })
    expect(wrapper.text()).toContain('videoModule.finalPrice')
    expect(wrapper.text()).not.toMatch(/[357]x/)
    expect(wrapper.find('.line-through').exists()).toBe(false)
  })

  it('retains subscription labels when the badge describes entitlement rather than rates', () => {
    const wrapper = mount(GroupBadge, {
      props: { name: 'Video', platform: 'video', subscriptionType: 'subscription', rateMultiplier: 3 },
      global: { stubs: { PlatformIcon: true } },
    })
    expect(wrapper.text()).toContain('groups.subscription')
  })
})
