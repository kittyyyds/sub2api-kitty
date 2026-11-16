import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import HeyRouteStats from '../HeyRouteStats.vue'
import { heyRouteCopyByLanguage } from '../heyRouteCopy'

describe('HeyRouteStats accessibility', () => {
  it('gives every stat a readable value and label while the animated digits stay hidden', () => {
    const copy = heyRouteCopyByLanguage.en.stats
    const wrapper = mount(HeyRouteStats, { props: { copy } })

    const cards = wrapper.findAll('.hr-stat-card')
    expect(cards).toHaveLength(4)
    expect(cards.map((card) => card.get('.hr-stat-value .sr-only').text()))
      .toEqual(['99.998%', '984ms', '120B+', '98.4%'])

    cards.forEach((card, index) => {
      const value = card.get('.hr-stat-value')
      expect(value.attributes('aria-label')).toBeUndefined()
      expect(value.get('strong').attributes('aria-hidden')).toBe('true')
      expect(value.get('span:not(.sr-only)').attributes('aria-hidden')).toBe('true')
      expect(card.get('h3').text()).toBe(copy.items[index]?.label)
    })
  })

  it('no longer carries the dead tilt attribute', () => {
    const wrapper = mount(HeyRouteStats, { props: { copy: heyRouteCopyByLanguage.zh.stats } })
    expect(wrapper.find('[data-hr-tilt]').exists()).toBe(false)
  })
})
