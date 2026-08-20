import { nextTick, shallowRef } from 'vue'
import { mount, shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import PlazaGroupSection from '../PlazaGroupSection.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true })
}))

vi.mock('@/composables/useDocumentDarkMode', () => ({
  useDocumentDarkMode: () => ({ isDarkMode: shallowRef(false) })
}))

function group(
  id: number,
  platform: string,
  name: string,
  rateMultiplier: number
): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform,
    subscription_type: 'standard',
    rate_multiplier: rateMultiplier,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    models: []
  }
}

function mountContent(response: ModelPlazaResponse) {
  return shallowMount(ModelPlazaContent, {
    props: {
      response,
      loading: false
    },
    global: {
      stubs: {
        Transition: { template: '<div><slot /></div>' }
      }
    }
  })
}

describe('ModelPlazaContent single-group navigation', () => {
  it('selects the first platform and renders only its first group', () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(11, 'openai', 'OpenAI Plus', 0.8),
        group(12, 'openai', 'OpenAI Max', 1),
        group(21, 'anthropic', 'Claude Plus', 0.7)
      ]
    })

    const filter = wrapper.getComponent(PlazaFilterBar)
    const renderedGroups = wrapper.findAllComponents(PlazaGroupSection)

    expect(filter.props('platform')).toBe('openai')
    expect(filter.props('groupId')).toBe(11)
    expect(filter.props('groups')).toHaveLength(2)
    expect(renderedGroups).toHaveLength(1)
    expect(renderedGroups[0].props('group').id).toBe(11)
  })

  it('switches to the first group of a newly selected platform', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(11, 'openai', 'OpenAI Plus', 0.8),
        group(21, 'anthropic', 'Claude Plus', 0.7),
        group(22, 'anthropic', 'Claude Max', 1)
      ]
    })

    wrapper.getComponent(PlazaFilterBar).vm.$emit('update:platform', 'anthropic')
    await nextTick()

    const filter = wrapper.getComponent(PlazaFilterBar)
    const section = wrapper.getComponent(PlazaGroupSection)
    expect(filter.props('platform')).toBe('anthropic')
    expect(filter.props('groupId')).toBe(21)
    expect(filter.props('groups')).toHaveLength(2)
    expect(section.props('group').id).toBe(21)
  })

  it('has no all-groups selection mode', () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(11, 'openai', 'OpenAI Plus', 0.8),
        group(21, 'anthropic', 'Claude Plus', 0.7)
      ]
    })

    expect(wrapper.getComponent(PlazaFilterBar).props('groupId')).toEqual(expect.any(Number))
    expect(wrapper.findAllComponents(PlazaGroupSection)).toHaveLength(1)
  })

  it('marks a user override as personal instead of presenting it as the group default', () => {
    const personalizedGroup = group(11, 'openai', 'OpenAI Plus', 0.13)
    personalizedGroup.user_rate_multiplier = 0.01
    const wrapper = mountContent({ description: '', groups: [personalizedGroup] })
    const groups = wrapper.getComponent(PlazaFilterBar).props('groups')

    expect(groups).toEqual([expect.objectContaining({
      id: 11,
      rate: 0.01,
      usesPersonalRate: true
    })])

    const filter = mount(PlazaFilterBar, {
      props: {
        platforms: ['openai'],
        groups,
        platform: 'openai',
        groupId: 11,
        search: ''
      }
    })
    expect(filter.get('.group-tabs small').text()).toBe('modelPlaza.filters.personalRate')
  })
})
