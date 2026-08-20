import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const setLocaleMock = vi.hoisted(() => vi.fn(async () => undefined))

vi.mock('@/i18n', () => ({
  setLocale: setLocaleMock,
}))

import HeyRouteHeader from '../HeyRouteHeader.vue'

const wrappers: VueWrapper[] = []

function mountHeader(locale: 'en' | 'zh'): VueWrapper {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: { en: {}, zh: {} },
  })
  const wrapper = mount(HeyRouteHeader, {
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
        sections: [{ id: 'hr-hero', label: 'Home' }],
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
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('HeyRouteHeader locale switcher', () => {
  beforeEach(() => {
    setLocaleMock.mockClear()
  })

  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  })

  it('switches an English landing page to canonical Chinese', async () => {
    const wrapper = mountHeader('en')

    await wrapper.get('.hr-language-control').trigger('click')

    expect(setLocaleMock).toHaveBeenCalledOnce()
    expect(setLocaleMock).toHaveBeenCalledWith('zh')
  })

  it('switches a Chinese landing page to English', async () => {
    const wrapper = mountHeader('zh')

    await wrapper.get('.hr-language-control').trigger('click')

    expect(setLocaleMock).toHaveBeenCalledOnce()
    expect(setLocaleMock).toHaveBeenCalledWith('en')
  })
})
