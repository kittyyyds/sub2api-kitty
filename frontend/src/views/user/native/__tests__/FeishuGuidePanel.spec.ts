import { mount, type VueWrapper } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createTestI18n } from '@/test/createTestI18n'

const { appStoreMock } = vi.hoisted(() => ({ appStoreMock: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: appStoreMock }))

let settings: { docUrl: string; apiBaseUrl: string; cachedPublicSettings: null }
const wrappers: VueWrapper[] = []
const guideUrl = 'https://docs.example/start/'

async function mountGuide() {
  const { default: FeishuGuidePanel } = await import('../FeishuGuidePanel.vue')
  const wrapper = mount(FeishuGuidePanel, { global: { plugins: [createTestI18n()] } })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.resetModules()
  vi.stubEnv('VITE_KITTY_SITE', 'kittyyyds')
  settings = reactive({ docUrl: guideUrl, apiBaseUrl: '', cachedPublicSettings: null })
  appStoreMock.mockReturnValue(settings)
})
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  vi.unstubAllEnvs()
})

describe('FeishuGuidePanel', () => {
  it('embeds the backend guide and exposes a safe new-window link', async () => {
    const wrapper = await mountGuide()
    const frame = wrapper.get('[data-testid="feishu-guide-frame"]')
    const openLink = wrapper.get('[data-testid="feishu-guide-open"]')

    expect(frame.attributes('src')).toBe(guideUrl)
    expect(openLink.attributes('href')).toBe(guideUrl)
    expect(openLink.attributes('target')).toBe('_blank')
    expect(openLink.attributes('rel')).toContain('noopener')
    await frame.trigger('load')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)

    settings.docUrl = 'https://docs.example/updated/'
    await nextTick()
    expect(frame.attributes('src')).toBe(settings.docUrl)
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
  })

  it.each(['kittyapi', 'kittyyyds'])('uses local %s documentation without a configured guide', async (site) => {
    vi.stubEnv('VITE_KITTY_SITE', site)
    settings.docUrl = ''
    const wrapper = await mountGuide()

    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.get('.docs-panel').text()).toContain(`https://${site}.xyz`)
    expect(wrapper.get('.docs-panel').text()).toContain(`https://cdn.${site}.xyz`)
    expect(wrapper.html()).not.toContain('tb.kittyyyds.xyz')

    settings.docUrl = 'javascript:alert(1)'
    await nextTick()
    expect(wrapper.find('iframe').exists()).toBe(false)
  })
})
