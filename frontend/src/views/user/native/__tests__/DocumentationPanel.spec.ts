import { mount, type VueWrapper } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createTestI18n } from '@/test/createTestI18n'

const { appStoreMock } = vi.hoisted(() => ({ appStoreMock: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: appStoreMock }))

let settings: {
  apiBaseUrl: string
  cachedPublicSettings: {
    api_base_url_description?: string
    custom_endpoints?: { name: string; endpoint: string; description: string }[]
  } | null
}
const wrappers: VueWrapper[] = []

async function mountDocs() {
  const { default: DocumentationPanel } = await import('../DocumentationPanel.vue')
  const wrapper = mount(DocumentationPanel, { global: { plugins: [createTestI18n()] } })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.resetModules()
  vi.stubEnv('VITE_KITTY_SITE', 'kittyyyds')
  settings = reactive({ apiBaseUrl: '', cachedPublicSettings: null })
  appStoreMock.mockReturnValue(settings)
})

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  vi.unstubAllEnvs()
})

describe('DocumentationPanel', () => {
  it.each(['kittyapi', 'kittyyyds'])('keeps %s commands and domestic entry isolated', async (site) => {
    vi.stubEnv('VITE_KITTY_SITE', site)
    const wrapper = await mountDocs()
    const primary = `https://${site}.xyz`

    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      `$env:ANTHROPIC_BASE_URL="${primary}"`
    )
    expect(wrapper.get('.endpoint-grid').text()).toContain(`https://cdn.${site}.xyz`)

    await wrapper.get('[data-testid="tool-codex"]').trigger('click')
    await wrapper.get('[data-testid="system-linux"]').trigger('click')
    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      `export OPENAI_BASE_URL="${primary}/v1"`
    )

    await wrapper.get('[data-testid="tool-openai"]').trigger('click')
    for (const sdk of ['curl', 'node', 'python']) {
      await wrapper.get(`[data-testid="sdk-${sdk}"]`).trigger('click')
      expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(`${primary}/v1`)
    }
    const otherSite = site === 'kittyapi' ? 'kittyyyds' : 'kittyapi'
    expect(wrapper.text()).not.toContain(`${otherSite}.xyz`)
    expect(wrapper.findAll('.troubleshooting-grid details')).toHaveLength(6)
  })

  it('updates examples and endpoint descriptions when backend settings arrive', async () => {
    const wrapper = await mountDocs()
    settings.apiBaseUrl = 'https://relay.example/tenant/v1/'
    settings.cachedPublicSettings = {
      api_base_url_description: 'Operator configured primary',
      custom_endpoints: [{
        name: 'Operator configured backup',
        endpoint: 'https://backup.example/relay',
        description: 'Use when primary is unavailable',
      }],
    }
    await nextTick()

    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      'ANTHROPIC_BASE_URL="https://relay.example/tenant"'
    )
    await wrapper.get('[data-testid="tool-codex"]').trigger('click')
    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      'OPENAI_BASE_URL="https://relay.example/tenant/v1"'
    )
    expect(wrapper.text()).toContain('Operator configured primary')
    expect(wrapper.text()).toContain('Use when primary is unavailable')
    expect(wrapper.text()).toContain('https://backup.example/relay')
    expect(wrapper.text()).not.toContain('kittyyyds.xyz')
    expect(wrapper.get('.multi-layout pre').text()).toContain('https://relay.example/tenant/v1')
  })
})
