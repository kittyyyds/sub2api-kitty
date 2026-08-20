import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import TurnstileWidget from '../TurnstileWidget.vue'

function createTestI18n() {
  return createI18n({
    legacy: false,
    locale: 'en',
    fallbackLocale: 'en',
    messages: {
      en: {
        common: { loading: () => 'Loading...' },
        auth: { turnstileFailed: () => 'Verification failed' }
      },
      zh: {
        common: { loading: () => '加载中...' },
        auth: { turnstileFailed: () => '验证失败' }
      }
    }
  })
}

afterEach(() => {
  delete window.turnstile
  document
    .querySelectorAll('script[data-kitty-turnstile-api], script[src*="/turnstile/v0/api.js"]')
    .forEach((script) => script.remove())
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('TurnstileWidget', () => {
  it('keeps the Turnstile language in sync with the application locale', async () => {
    const render = vi.fn()
      .mockReturnValueOnce('widget-en')
      .mockReturnValueOnce('widget-zh')
    const remove = vi.fn()

    window.turnstile = {
      render,
      reset: vi.fn(),
      remove
    }

    const i18n = createTestI18n()

    const wrapper = mount(TurnstileWidget, {
      props: { siteKey: 'test-site-key' },
      global: { plugins: [i18n] }
    })
    await flushPromises()

    expect(render).toHaveBeenCalledTimes(1)
    expect(render.mock.calls[0][1]).toMatchObject({
      sitekey: 'test-site-key',
      language: 'en'
    })

    i18n.global.locale.value = 'zh'
    await nextTick()
    await flushPromises()

    expect(remove).toHaveBeenCalledWith('widget-en')
    expect(render).toHaveBeenCalledTimes(2)
    expect(render.mock.calls[1][1]).toMatchObject({
      sitekey: 'test-site-key',
      language: 'zh-CN'
    })

    wrapper.unmount()
  })

  it('renders after a cold script load without relying on a global callback', async () => {
    const render = vi.fn().mockReturnValue('widget-cold')
    const wrapper = mount(TurnstileWidget, {
      props: { siteKey: 'cold-site-key' },
      global: {
        plugins: [createTestI18n()]
      }
    })
    await flushPromises()

    const script = document.querySelector<HTMLScriptElement>('script[data-kitty-turnstile-api]')
    expect(script?.src).toBe(
      'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    )
    expect(wrapper.attributes('data-turnstile-state')).toBe('loading')

    window.turnstile = { render, reset: vi.fn(), remove: vi.fn() }
    script?.dispatchEvent(new Event('load'))
    await flushPromises()

    expect(render).toHaveBeenCalledTimes(1)
    expect(wrapper.attributes('data-turnstile-state')).toBe('ready')
    wrapper.unmount()
  })

  it('adopts an existing in-flight script and still observes delayed readiness', async () => {
    const existingScript = document.createElement('script')
    existingScript.src =
      'https://challenges.cloudflare.com/turnstile/v0/api.js?onload=legacyCallback'
    document.head.appendChild(existingScript)

    const render = vi.fn().mockReturnValue('widget-existing')
    const wrapper = mount(TurnstileWidget, {
      props: { siteKey: 'existing-site-key' },
      global: {
        plugins: [createTestI18n()]
      }
    })
    await flushPromises()

    expect(document.querySelectorAll('script[src*="/turnstile/v0/api.js"]')).toHaveLength(1)
    window.turnstile = { render, reset: vi.fn(), remove: vi.fn() }
    existingScript.dispatchEvent(new Event('load'))
    await flushPromises()

    expect(render).toHaveBeenCalledTimes(1)
    expect(wrapper.attributes('data-turnstile-state')).toBe('ready')
    wrapper.unmount()
  })

  it('retries with a fresh script after the first cold-load request fails', async () => {
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const render = vi.fn().mockReturnValue('widget-retry')
    const wrapper = mount(TurnstileWidget, {
      props: { siteKey: 'retry-site-key' },
      global: {
        plugins: [createTestI18n()]
      }
    })
    await flushPromises()

    const failedScript = document.querySelector<HTMLScriptElement>(
      'script[data-kitty-turnstile-api]'
    )
    failedScript?.dispatchEvent(new Event('error'))
    await flushPromises()
    expect(failedScript?.isConnected).toBe(false)

    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()
    const retryScript = document.querySelector<HTMLScriptElement>(
      'script[data-kitty-turnstile-api]'
    )
    expect(retryScript).not.toBe(failedScript)

    window.turnstile = { render, reset: vi.fn(), remove: vi.fn() }
    retryScript?.dispatchEvent(new Event('load'))
    await flushPromises()

    expect(render).toHaveBeenCalledTimes(1)
    expect(wrapper.attributes('data-turnstile-state')).toBe('ready')
    wrapper.unmount()
  })
})
