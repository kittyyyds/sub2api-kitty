import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: `
    <select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `,
}

function videoEntry(overrides: Partial<PricingFormEntry> = {}): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'video',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...overrides,
  }
}

function mountCard(entry: PricingFormEntry, allowVideoBilling = true) {
  return mount(PricingEntryCard, {
    props: { entry, platform: 'grok', allowVideoBilling },
    global: {
      stubs: {
        Icon: true,
        ModelTagInput: true,
        Select: SelectStub,
      },
    },
  })
}

describe('PricingEntryCard video billing', () => {
  it('offers video mode with a per-second resolution-tier editor', () => {
    const wrapper = mountCard(videoEntry())

    expect(wrapper.text()).toContain('admin.channels.billingMode.video')
    expect(wrapper.text()).toContain('admin.channels.form.defaultVideoPrice')
    expect(wrapper.text()).toContain('admin.channels.form.videoTiers')
    expect(wrapper.text()).toContain('admin.channels.form.perSecondUnit')
  })

  it('hides video billing when the host workflow does not support it', () => {
    const wrapper = mountCard(videoEntry(), false)

    expect(wrapper.find('option[value="video"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.channels.form.defaultVideoPrice')
    expect(wrapper.text()).not.toContain('admin.channels.form.videoTiers')
  })

  it('adds the standard 480p, 720p, and 1080p tiers in order', async () => {
    const wrapper = mountCard(videoEntry())
    const addTier = () => wrapper.findAll('button').find((button) =>
      button.text().includes('admin.channels.form.addTier')
    )

    for (const expected of ['480p', '720p', '1080p']) {
      await addTier()?.trigger('click')
      const updates = wrapper.emitted('update')
      const latest = updates?.at(-1)?.[0] as PricingFormEntry
      expect(latest.intervals.at(-1)?.tier_label).toBe(expected)
      await wrapper.setProps({ entry: latest })
    }

    expect(wrapper.findAll('input[placeholder="480p / 720p / 1080p"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('admin.channels.form.perSecondPrice')
  })
})
