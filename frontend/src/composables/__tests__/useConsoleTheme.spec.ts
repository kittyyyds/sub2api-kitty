import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useConsoleTheme } from '../useConsoleTheme'

describe('console theme ownership', () => {
  it('keeps portaled controls themed during layout overlap and cleans up for the homepage', () => {
    const Layout = defineComponent({ setup() { useConsoleTheme(); return () => null } })
    const first = mount(Layout)
    const second = mount(Layout)
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
    first.unmount()
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
    second.unmount()
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(false)
  })

  it('supports an optional enabled flag and releases ownership when disabled', async () => {
    const enabled = ref(false)
    const Layout = defineComponent({ setup() { useConsoleTheme(enabled); return () => null } })
    const wrapper = mount(Layout)
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(false)
    enabled.value = true
    await nextTick()
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
    enabled.value = false
    await nextTick()
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(false)
    wrapper.unmount()
  })
})
