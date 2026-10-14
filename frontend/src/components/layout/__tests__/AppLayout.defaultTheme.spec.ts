import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AppLayout from '../AppLayout.vue'

vi.mock('@/stores', () => ({
  useAppStore: () => ({ sidebarCollapsed: false })
}))

const auth = vi.hoisted(() => ({ isAdmin: false, user: { role: 'user' } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ setReplayCallback: vi.fn() })
}))

vi.mock('@/composables/useOnboardingTour', () => ({
  useOnboardingTour: () => ({ replayTour: vi.fn() })
}))

vi.mock('../AppSidebar.vue', () => ({
  default: { template: '<aside data-testid="sidebar" />' }
}))

vi.mock('../AppHeader.vue', () => ({
  default: { template: '<header data-testid="header" />' }
}))

const directory = dirname(fileURLToPath(import.meta.url))
const sourceRoot = resolve(directory, '../../..')
const appSource = readFileSync(resolve(sourceRoot, 'App.vue'), 'utf8')
const layoutSource = readFileSync(resolve(directory, '../AppLayout.vue'), 'utf8')
const styleSource = readFileSync(resolve(sourceRoot, 'style.css'), 'utf8')
const consoleThemePath = resolve(sourceRoot, 'styles/console-theme.css')

afterEach(() => {
  document.body.className = ''
})

beforeEach(() => { auth.isAdmin = false })

describe('default SUB2API application layout', () => {
  it('uses the standard shell for an ordinary customer without mutating the body theme', () => {
    document.body.classList.add('existing-body-class')

    const wrapper = mount(AppLayout, {
      slots: { default: '<section data-testid="content">content</section>' }
    })

    expect(wrapper.classes()).toEqual(
      expect.arrayContaining(['min-h-screen', 'bg-gray-50', 'dark:bg-dark-950'])
    )
    expect(wrapper.classes()).not.toContain('customer-console-shell')
    expect(wrapper.find('.bg-mesh-gradient').exists()).toBe(false)
    expect(wrapper.get('[data-testid="content"]').text()).toBe('content')
    expect(document.body.className).toBe('existing-body-class')

    wrapper.unmount()
    expect(document.body.className).toBe('existing-body-class')
  })

  it('scopes the homepage-matched console theme without mutating the body', () => {
    document.body.classList.add('existing-body-class')

    const wrapper = mount(AppLayout, {
      slots: { default: '<section data-testid="content">content</section>' }
    })

    expect(wrapper.attributes('data-console-shell')).toBe('true')
    expect(wrapper.classes()).toContain('kitty-console-shell')
    expect(layoutSource).toContain("import '@/styles/console-theme.css'")
    expect(existsSync(consoleThemePath)).toBe(true)
    expect(document.body.className).toBe('existing-body-class')
    wrapper.unmount()
  })

  it('preserves administrator dimensions while sharing the customer theme with portaled controls', () => {
    auth.isAdmin = true
    const wrapper = mount(AppLayout)
    expect(wrapper.classes()).toContain('kitty-admin-shell')
    expect(wrapper.classes()).not.toContain('kitty-customer-shell')
    expect(wrapper.get('.kitty-console-main').classes()).toContain('lg:ml-[230px]')
    expect(wrapper.get('main').classes()).toContain('lg:p-5')
    expect(wrapper.find('.bg-mesh-gradient').exists()).toBe(false)
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(true)
    wrapper.unmount()
    expect(document.documentElement.classList.contains('kitty-console-theme')).toBe(false)
  })

  it('keeps the retired customer scene and theme out of the application sources', () => {
    for (const marker of [
      'CustomerConsoleScene',
      'VITE_KITTY_SCENE_VARIANT',
      'CUSTOMER_CONSOLE_VARIANT_CLASS',
      'customer-console-theme',
      'customer-console-variant'
    ]) {
      expect(appSource).not.toContain(marker)
      expect(layoutSource).not.toContain(marker)
      expect(styleSource).not.toContain(marker)
    }
  })
})
