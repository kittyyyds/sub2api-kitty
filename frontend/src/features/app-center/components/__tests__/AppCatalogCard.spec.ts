import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { AppPresentation, ApplicationDefinition } from '../../types'
import AppCatalogCard from '../AppCatalogCard.vue'

const application: ApplicationDefinition = {
  slug: 'inspiration-image-refiner',
  revision: 1,
  version: '1.0',
  name: '灵感精修生图',
  description: '把灵感整理成图片。',
  category: 'image',
  status: '可用',
  tags: [],
  supported_modes: [],
  form: { fields: [] },
}

const presentation: AppPresentation = {
  slug: 'inspiration-image-refiner',
  name: '灵感精修生图',
  description: '把灵感整理成图片。',
  category: 'image',
  version: '1.0',
  icon: 'lightbulb',
  requiresReference: false,
  acceptsMultipleReferences: true,
}

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span class="icon-stub" :data-icon="name" />',
}

describe('AppCatalogCard', () => {
  it('renders a category tone and an explicit selected marker', async () => {
    const wrapper = mount(AppCatalogCard, {
      props: { application, presentation, selected: true },
      global: { stubs: { Icon: IconStub } },
    })

    const card = wrapper.get('button.app-card')
    expect(card.attributes('data-tone')).toBe('image')
    expect(card.attributes('aria-pressed')).toBe('true')
    expect(card.classes()).toContain('selected')
    expect(wrapper.get('.app-icon .icon-stub').attributes('data-icon')).toBe('lightbulb')
    expect(wrapper.get('.selection-mark .icon-stub').attributes('data-icon')).toBe('check')
    expect(wrapper.find('.app-arrow').exists()).toBe(false)

    await card.trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
  })

  it('shows the navigation arrow when the card is not selected', () => {
    const wrapper = mount(AppCatalogCard, {
      props: { application, presentation, selected: false },
      global: { stubs: { Icon: IconStub } },
    })

    expect(wrapper.get('button.app-card').attributes('aria-pressed')).toBe('false')
    expect(wrapper.find('.selection-mark').exists()).toBe(false)
    expect(wrapper.get('.app-arrow').attributes('data-icon')).toBe('chevronRight')
  })
})
