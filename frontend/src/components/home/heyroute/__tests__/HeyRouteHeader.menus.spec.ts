import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/i18n', () => ({
  setLocale: vi.fn(async () => undefined),
}))

import HeyRouteHeader from '../HeyRouteHeader.vue'

const wrappers: VueWrapper[] = []

function mountHeader(): VueWrapper {
  const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {}, zh: {} } })
  const wrapper = mount(HeyRouteHeader, {
    attachTo: document.body,
    props: {
      copy: {
        home: 'Home',
        models: 'Models',
        docs: 'Docs',
        login: 'Sign in',
        dashboard: 'Dashboard',
        menu: 'Open navigation',
        closeMenu: 'Close navigation',
        switchLanguage: 'Switch language',
        switchTheme: 'Switch theme',
        primary: 'Primary navigation',
        sections: [{ id: 'hr-hero', label: 'Home' }, { id: 'hr-pricing', label: 'Pricing' }],
      },
      siteName: 'Kitty奇妙屋',
      siteLogo: '/logo.svg',
      docUrl: '',
      activeSection: 'hr-hero',
      isDark: false,
      isAuthenticated: false,
      dashboardPath: '/dashboard',
      modelPlazaEnabled: false,
      isHeroActive: true,
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        // The shared setup clears VTU's default stubs; real leave transitions never finish in jsdom.
        transition: true,
        RouterLink: { template: '<a href="#"><slot /></a>' },
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

async function pressEscape(wrapper: VueWrapper): Promise<void> {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  await wrapper.vm.$nextTick()
}

async function pointerDownOn(wrapper: VueWrapper, target: Element): Promise<void> {
  target.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  await wrapper.vm.$nextTick()
}

describe('HeyRouteHeader menus', () => {
  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  })

  it('closes the mobile menu with Escape and returns focus to the menu button', async () => {
    const wrapper = mountHeader()
    const menuButton = wrapper.get('.hr-menu-control')

    await menuButton.trigger('click')
    expect(wrapper.find('.hr-mobile-menu').exists()).toBe(true)
    ;(wrapper.get('.hr-mobile-menu button').element as HTMLElement).focus()

    await pressEscape(wrapper)

    expect(wrapper.find('.hr-mobile-menu').exists()).toBe(false)
    expect(menuButton.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(menuButton.element)
  })

  it('closes the section menu with Escape and returns focus to its trigger', async () => {
    const wrapper = mountHeader()
    const trigger = wrapper.get('.hr-section-trigger')

    await trigger.trigger('click')
    expect(wrapper.find('.hr-section-menu').exists()).toBe(true)
    ;(wrapper.get('.hr-section-menu button').element as HTMLElement).focus()

    await pressEscape(wrapper)

    expect(wrapper.find('.hr-section-menu').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
  })

  it('closes both menus on a pointer down outside them but not inside', async () => {
    const wrapper = mountHeader()

    await wrapper.get('.hr-menu-control').trigger('click')
    await wrapper.get('.hr-section-trigger').trigger('click')

    await pointerDownOn(wrapper, wrapper.get('.hr-mobile-menu button').element)
    expect(wrapper.find('.hr-mobile-menu').exists()).toBe(true)

    await pointerDownOn(wrapper, document.body)
    expect(wrapper.find('.hr-mobile-menu').exists()).toBe(false)
    expect(wrapper.find('.hr-section-menu').exists()).toBe(false)
  })

  it('removes its window listeners on unmount', () => {
    const removeSpy = vi.spyOn(window, 'removeEventListener')
    const wrapper = mountHeader()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    wrapper.unmount()

    const removed = removeSpy.mock.calls.map(([type]) => type)
    expect(removed).toEqual(expect.arrayContaining(['pointerdown', 'keydown']))
    removeSpy.mockRestore()
  })
})
