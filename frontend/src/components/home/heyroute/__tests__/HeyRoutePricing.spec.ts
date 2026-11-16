import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import HeyRoutePricing from '../HeyRoutePricing.vue'
import { heyRouteCopyByLanguage } from '../heyRouteCopy'

const summaries = [
  {
    platform: 'openai' as const,
    label: 'GPT / Codex',
    iconModel: 'gpt-5.6',
    rateMultiplier: 0.1,
    groupName: 'Codex - 福利特惠【尽量不断】',
    subscriptionType: 'standard',
    modelCount: 10,
    featuredModel: 'gpt-5.6-sol',
    imageRateMultiplier: null,
    videoRateMultiplier: null,
  },
  {
    platform: 'anthropic' as const,
    label: 'Claude / Kiro',
    iconModel: 'claude-sonnet',
    rateMultiplier: 0.4,
    groupName: 'Claude - Kiro -【99缓】',
    subscriptionType: 'standard',
    modelCount: 10,
    featuredModel: 'claude-fable-5',
    imageRateMultiplier: null,
    videoRateMultiplier: null,
  },
  {
    platform: 'grok' as const,
    label: 'Grok',
    iconModel: 'grok-4.5',
    rateMultiplier: 0.2,
    groupName: 'Grok - 正品会员池【稳定】',
    subscriptionType: 'standard',
    modelCount: 7,
    featuredModel: 'grok-4.5',
    imageRateMultiplier: null,
    videoRateMultiplier: null,
  },
  {
    platform: 'anthropic' as const,
    label: 'Claude / Kiro',
    iconModel: 'claude-sonnet',
    rateMultiplier: 0.4,
    groupName: 'Claude - Kiro -【99缓】',
    subscriptionType: 'standard',
    modelCount: 10,
    featuredModel: 'claude-sonnet-5',
    imageRateMultiplier: null,
    videoRateMultiplier: null,
  },
]

describe('HeyRoutePricing', () => {
  it('renders each representative model with its actual matching group rate', () => {
    const wrapper = mount(HeyRoutePricing, {
      props: {
        copy: heyRouteCopyByLanguage.zh.pricing,
        summaries,
        loading: false,
        failed: false,
        modelPlazaEnabled: true,
      },
      global: {
        stubs: {
          ModelIcon: { template: '<span />' },
          Icon: { template: '<span />' },
          RouterLink: { props: ['to'], template: '<a><slot /></a>' },
        },
      },
    })

    const rows = wrapper.findAll('.hr-pricing-row:not(.hr-pricing-head)')
    expect(rows).toHaveLength(4)
    expect(rows.map((row) => [
      row.get('.hr-price-model strong').text(),
      row.get('.hr-rate-value').text(),
      row.get('.hr-group-value strong').text(),
      row.get('.hr-group-value small').text(),
    ])).toEqual([
      ['gpt-5.6-sol', 'x0.1', 'Codex - 福利特惠【尽量不断】', '10 个模型'],
      ['claude-fable-5', 'x0.4', 'Claude - Kiro -【99缓】', '10 个模型'],
      ['grok-4.5', 'x0.2', 'Grok - 正品会员池【稳定】', '7 个模型'],
      ['claude-sonnet-5', 'x0.4', 'Claude - Kiro -【99缓】', '10 个模型'],
    ])
  })

  it('labels the model count in the visitor language', () => {
    const wrapper = mount(HeyRoutePricing, {
      props: { copy: heyRouteCopyByLanguage.en.pricing, summaries, loading: false, failed: false, modelPlazaEnabled: true },
      global: {
        stubs: {
          ModelIcon: { template: '<span />' },
          Icon: { template: '<span />' },
          RouterLink: { props: ['to'], template: '<a><slot /></a>' },
        },
      },
    })
    expect(wrapper.get('.hr-pricing-row:not(.hr-pricing-head) .hr-group-value small').text()).toBe('10 models')
  })

  const stubs = {
    ModelIcon: { template: '<span />' },
    Icon: { template: '<span />' },
    RouterLink: { props: ['to'], template: '<a><slot /></a>' },
  }

  it('shows a dash and an inactive dot when a rate is missing after loading', () => {
    const copy = heyRouteCopyByLanguage.zh.pricing
    const wrapper = mount(HeyRoutePricing, {
      props: { copy, summaries: [], loading: false, failed: false, modelPlazaEnabled: true },
      global: { stubs },
    })

    const rows = wrapper.findAll('.hr-pricing-row:not(.hr-pricing-head)')
    expect(rows).toHaveLength(copy.fallbacks.length)
    rows.forEach((row) => {
      expect(row.get('.hr-rate-value').text()).toBe('—')
      expect(row.get('.hr-rate-value').classes()).not.toContain('loading')
      expect(row.get('.hr-live-dot').classes()).toContain('is-unavailable')
      expect(row.get('.hr-interface-value').text()).toBe(copy.unavailable)
    })
  })

  it('keeps the loading look while rates are still loading', () => {
    const wrapper = mount(HeyRoutePricing, {
      props: { copy: heyRouteCopyByLanguage.zh.pricing, summaries: [], loading: true, failed: false, modelPlazaEnabled: true },
      global: { stubs },
    })

    const row = wrapper.get('.hr-pricing-row:not(.hr-pricing-head)')
    expect(row.get('.hr-rate-value').text()).toBe('...')
    expect(row.get('.hr-rate-value').classes()).toContain('loading')
    expect(row.get('.hr-live-dot').classes()).not.toContain('is-unavailable')
  })

  it('keeps the live dot on for rows that have a rate', () => {
    const wrapper = mount(HeyRoutePricing, {
      props: { copy: heyRouteCopyByLanguage.en.pricing, summaries, loading: false, failed: false, modelPlazaEnabled: true },
      global: { stubs },
    })

    const row = wrapper.get('.hr-pricing-row:not(.hr-pricing-head)')
    expect(row.get('.hr-live-dot').classes()).not.toContain('is-unavailable')
    expect(row.get('.hr-interface-value').text()).toBe(heyRouteCopyByLanguage.en.pricing.live)
  })

  it('exposes consistent table semantics', () => {
    const copy = heyRouteCopyByLanguage.en.pricing
    const wrapper = mount(HeyRoutePricing, {
      props: { copy, summaries, loading: false, failed: false, modelPlazaEnabled: true },
      global: { stubs },
    })

    const table = wrapper.get('[role="table"]')
    expect(table.attributes('aria-label')).toBe(copy.title)
    const rows = table.findAll('[role="row"]')
    expect(rows).toHaveLength(copy.fallbacks.length + 1)
    expect(rows[0].findAll('[role="columnheader"]')).toHaveLength(4)
    rows.slice(1).forEach((row) => {
      expect(row.findAll('[role="cell"]')).toHaveLength(4)
    })
  })
})
