import { nextTick, shallowRef } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import ModelPlazaContent from '../ModelPlazaContent.vue'

const { copyToClipboard } = vi.hoisted(() => ({
  copyToClipboard: vi.fn().mockResolvedValue(true)
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (
        params?.group ? `${key}:${params.group}` : key
      )
    })
  }
})
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true })
}))

vi.mock('@/composables/useDocumentDarkMode', () => ({
  useDocumentDarkMode: () => ({ isDarkMode: shallowRef(false) })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard })
}))

function tokenModel(name: string): PlazaModel {
  return {
    name,
    platform: 'openai',
    pricing: {
      billing_mode: 'token',
      input_price: 1e-6,
      output_price: 2e-6,
      cache_write_price: null,
      cache_write_1h_price: null,
      cache_read_price: 2e-7,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 1e-6,
      output_price: 2e-6,
      cache_write_price: null,
      cache_read_price: 2e-7
    }
  }
}

function group(id: number, name: string, rate: number, models: PlazaModel[]): ModelPlazaGroup {
  return {
    id,
    name,
    description: '',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: rate,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    long_context_pricing_enabled: true,
    models
  }
}

function mountContent(response: ModelPlazaResponse) {
  return mount(ModelPlazaContent, {
    props: { response, loading: false },
    global: {
      stubs: {
        Icon: true,
        ModelIcon: true,
        PlatformIcon: true
      }
    }
  })
}

describe('ModelPlazaContent model catalog', () => {
  beforeEach(() => copyToClipboard.mockClear())

  it('starts in best-price mode and renders one card for a shared model', () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, 'Stable', 0.8, [tokenModel('gpt-5.6-sol')]),
        group(2, 'Value', 0.2, [tokenModel('gpt-5.6-sol')])
      ]
    })

    expect(wrapper.get('.pricing-group-chip').classes()).toContain('active')
    expect(wrapper.findAll('.platform-card')).toHaveLength(1)
    expect(wrapper.get('.best-group-pill').text()).toContain('Value')
    expect(wrapper.text()).toContain('$0.20')
    expect(wrapper.text()).toContain('-80%')
  })

  it('switches to a concrete group and only displays that group models', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, 'Stable', 0.8, [tokenModel('gpt-stable')]),
        group(2, 'Value', 0.2, [tokenModel('gpt-value')])
      ]
    })

    await wrapper.findAll('.pricing-group-chip')[1].trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('gpt-stable')
    expect(wrapper.text()).not.toContain('gpt-value')
    expect(wrapper.get('.best-group-pill').text()).toContain('Stable')
  })

  it('shows a personal multiplier on the group chip', () => {
    const personalized = group(1, 'Personal', 0.8, [tokenModel('gpt-5.6')])
    personalized.user_rate_multiplier = 0.13
    const wrapper = mountContent({ description: '', groups: [personalized] })

    expect(wrapper.findAll('.pricing-group-chip')[1].get('small').text()).toBe('×0.13')
    expect(wrapper.findAll('.pricing-group-chip')[1].get('small').attributes('title'))
      .toBe('modelPlaza.catalog.personalRate')
  })

  it('copies the normalized model ID', async () => {
    const model = tokenModel('publishers/openai/models/gpt-5.6-sol')
    const wrapper = mountContent({ description: '', groups: [group(1, 'Stable', 0.8, [model])] })

    await wrapper.get('.copy-id-button').trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith(
      'gpt-5.6-sol',
      'modelPlaza.catalog.copySuccess'
    )
  })

  it('emits retry from the error state', async () => {
    const wrapper = mount(ModelPlazaContent, {
      props: { response: null, loading: false, error: true },
      global: { stubs: { Icon: true } }
    })

    await wrapper.get('.retry-button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})
