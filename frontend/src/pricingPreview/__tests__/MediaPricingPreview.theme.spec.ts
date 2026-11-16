import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import Preview from '../MediaPricingPreview.vue'

describe('Kitty custom pricing preview theme', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: null, captured_at: '' })
    }))
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    document.documentElement.classList.remove('dark')
    vi.unstubAllGlobals()
  })

  it('adopts the existing Kitty console theme for the page and portaled details', async () => {
    wrapper = mount(Preview)
    await flushPromises()
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
    expect(wrapper.find('.preview-app').exists()).toBe(true)
    wrapper.unmount()
    wrapper = undefined
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(false)
  })

  it('keeps the same custom theme when switching to the canvas model picker', async () => {
    wrapper = mount(Preview)
    await flushPromises()
    await wrapper.get('.preview-nav > div button:nth-child(2)').trigger('click')
    expect(wrapper.find('.picker-preview').exists()).toBe(true)
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
  })

  it('switches the existing theme to dark mode without loading a different palette', async () => {
    wrapper = mount(Preview)
    await flushPromises()
    await wrapper.get('.theme-button').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
    await wrapper.get('.theme-button').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
