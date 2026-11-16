import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorSettingsPanel from '../MonitorSettingsPanel.vue'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import type { MonitorConfig } from '@/api/channelMonitorV2'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  groups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))
vi.mock('@/api/channelMonitorV2', () => ({
  getConfig: mocks.getConfig,
  updateConfig: mocks.updateConfig,
  MONITOR_ERROR_CATEGORIES: ['authentication', 'network'],
}))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: mocks.groups } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({
  cachedPublicSettings: { channel_monitor_enabled: true },
  showError: mocks.showError,
  showSuccess: mocks.showSuccess,
}) }))
vi.mock('@/utils/featureFlags', () => ({
  getChannelMonitorMode: () => 'v2',
  isChannelMonitorV2Mode: () => true,
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, te: () => false }),
}))

function config(): MonitorConfig {
  return {
    version: 2,
    enabled: true,
    refresh_interval_seconds: 60,
    platforms: [
      { platform: 'openai', enabled: false, models: ['gpt-4'] },
      { platform: 'custom-provider', enabled: true, models: ['custom-model'] },
    ],
    group_ids: [42],
    ignored_error_categories: [],
    health_thresholds: {
      minimum_sample: 70, warning_error_rate: 0.1, critical_error_rate: 0.3,
      target_ttft_ms: 1500, warning_ttft_ms: 2000, critical_ttft_ms: 5000,
      warning_cache_rate: 0.7, critical_cache_rate: 0.4,
      error_weight: 0.5, ttft_weight: 0.3, cache_weight: 0.2,
    },
  }
}

async function panel(value = config()) {
  mocks.getConfig.mockResolvedValue(value)
  const wrapper = mount(MonitorSettingsPanel, { global: { stubs: { RouterLink: true, teleport: true } } })
  await flushPromises()
  return wrapper
}

function button(wrapper: Awaited<ReturnType<typeof panel>>, key: string) {
  return wrapper.findAll('button').find((item) => item.text() === 'channelMonitorV2.settings.' + key)!
}

const select = (wrapper: Awaited<ReturnType<typeof panel>>) => wrapper.get('[data-testid="monitor-v2-platform-select"]')
const options = (wrapper: Awaited<ReturnType<typeof panel>>) => select(wrapper).findAll('option').map(o => o.attributes('value'))
async function add(wrapper: Awaited<ReturnType<typeof panel>>, platform: string) {
  await wrapper.get('[data-testid="monitor-v2-platform-custom"]').setValue(platform)
  await button(wrapper, 'addPlatform').trigger('click')
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.groups.mockResolvedValue([])
  mocks.updateConfig.mockImplementation(async (value) => JSON.parse(JSON.stringify(value)))
})

describe('MonitorSettingsPanel platform configuration', () => {
  it('adds a missing provider for automatic called-model discovery without changing existing settings', async () => {
    const original = config()
    const wrapper = await panel(original)
    expect(button(wrapper, 'save').attributes('disabled')).toBeDefined()
    expect(options(wrapper)).not.toContain('openai')
    await select(wrapper).setValue('deepseek')
    await button(wrapper, 'addPlatform').trigger('click')
    expect(options(wrapper)).not.toContain('deepseek')
    await button(wrapper, 'save').trigger('click')
    await flushPromises()

    expect(mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      ...original,
      platforms: [...original.platforms, {
        platform: 'deepseek', enabled: true, models: [],
      }],
    }))
    expect(button(wrapper, 'save').attributes('disabled')).toBeDefined()
    expect(original.platforms).toHaveLength(2)

    const persisted = mocks.updateConfig.mock.results[0].value
    wrapper.unmount()
    const reloaded = await panel(await persisted)
    expect(reloaded.find('[data-testid="monitor-v2-platform-custom"]').exists()).toBe(true)
    expect(options(reloaded)).not.toContain('deepseek')
    reloaded.unmount()
  })

  it('can start with no configured platforms and rejects duplicate additions', async () => {
    const wrapper = await panel({ ...config(), platforms: [] })
    await button(wrapper, 'addPlatform').trigger('click')
    expect(mocks.showError).toHaveBeenCalledWith('channelMonitorV2.settings.platformRequired')
    await add(wrapper, 'kimi')
    await add(wrapper, 'kimi')
    await button(wrapper, 'save').trigger('click')
    await flushPromises()
    expect(mocks.updateConfig.mock.calls[0][0].platforms).toEqual([
      { platform: 'kimi', enabled: true, models: [] },
    ])
    wrapper.unmount()
  })

  it('restores the server config and available options when saving fails', async () => {
    const wrapper = await panel()
    mocks.updateConfig.mockRejectedValueOnce(new Error('save failed'))
    await add(wrapper, 'minimax')
    await button(wrapper, 'save').trigger('click')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledOnce()
    expect(options(wrapper)).toContain('minimax')
    expect(wrapper.find('[data-testid="monitor-v2-platform-custom"]').exists()).toBe(true)
    expect(button(wrapper, 'save').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('retains custom provider input after known platforms are configured', async () => {
    const wrapper = await panel({
      ...config(),
      platforms: CONCRETE_PLATFORM_OPTIONS.map(({ value }) => ({ platform: value, enabled: true, models: [] })),
    })
    expect(options(wrapper)).not.toContain('openai')
    await add(wrapper, 'another-provider')
    await button(wrapper, 'save').trigger('click')
    await flushPromises()
    expect(mocks.updateConfig.mock.calls[0][0].platforms).toContainEqual({ platform: 'another-provider', enabled: true, models: [] })
    wrapper.unmount()
  })
})
