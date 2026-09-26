import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import { createTestI18n } from '@/test/createTestI18n'
import MediaModeSwitch from '../MediaModeSwitch.vue'

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span class="icon-stub" :data-icon="name" />',
}

describe('MediaModeSwitch', () => {
  it('exposes all four visual modes and clearly marks the active one', async () => {
    const updateModel = vi.fn()
    const wrapper = mount(MediaModeSwitch, {
      props: {
        modelValue: 'text',
        'onUpdate:modelValue': updateModel,
      },
      global: {
        plugins: [createTestI18n()],
        stubs: { Icon: IconStub },
      },
    })

    const buttons = wrapper.findAll('button[data-mode]')
    expect(buttons.map((button) => button.attributes('data-mode'))).toEqual([
      'text',
      'image',
      'text-video',
      'image-video',
    ])
    expect(buttons[0]?.classes()).toContain('active')
    expect(buttons[0]?.attributes('aria-pressed')).toBe('true')
    expect(buttons.slice(1).every((button) => button.attributes('aria-pressed') === 'false')).toBe(true)

    expect(buttons.map((button) => button.get('.icon-stub').attributes('data-icon'))).toEqual([
      'sparkles',
      'photo',
      'videoCamera',
      'videoCamera',
    ])

    await buttons[3]!.trigger('click')
    expect(updateModel).toHaveBeenCalledWith('image-video')
  })

  it('disables every mode while a generation is running', () => {
    const wrapper = mount(MediaModeSwitch, {
      props: { modelValue: 'text-video', disabled: true },
      global: {
        plugins: [createTestI18n()],
        stubs: { Icon: IconStub },
      },
    })

    expect(wrapper.findAll('button[data-mode]').every((button) => button.attributes('disabled') !== undefined)).toBe(true)
  })
})
