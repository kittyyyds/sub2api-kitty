import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorModelDetails from '../MonitorModelDetails.vue'
import { formatMonitorOutputTPS } from '../monitorFormat'

const mocks = vi.hoisted(() => ({ getMatrix: vi.fn() }))
vi.mock('@/api/channelMonitorV2', () => ({ getMatrix: mocks.getMatrix }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
const group = { platform: 'openai', group_id: 7, group_name: 'Kitty' } as any
const filter = { range: '90m', platforms: [], groupIds: [], models: [] } as any
const row = { ...group, model: 'gpt-test', metrics: { has_samples: true, error_rate: .1, cache_rate: .5, output_tps: 32, tpm: 999999, ttft: { p50_ms: 123 } }, health: { overall: 'healthy' } }
const global = { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }

describe('model monitoring details', () => {
  beforeEach(() => vi.clearAllMocks())
  it('does not load closed panels and uses the selected group and existing range when opened', async () => {
    mocks.getMatrix.mockResolvedValue({ items: [row] })
    const wrapper = mount(MonitorModelDetails, { props: { group: null, filter, admin: false, showThroughput: true }, global })
    await flushPromises()
    expect(mocks.getMatrix).not.toHaveBeenCalled()
    await wrapper.setProps({ group })
    await flushPromises()
    expect(mocks.getMatrix).toHaveBeenCalledWith({ ...filter, platforms: ['openai'], groupIds: [7] }, 'platform_group_model', false, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('gpt-test')
    expect(wrapper.text()).toContain('32')
    expect(wrapper.text()).not.toContain('999999')
    wrapper.unmount()
  })
  it('honors throughput privacy and prevents stale group replies from replacing the current panel', async () => {
    let finish: (data: unknown) => void = () => undefined
    mocks.getMatrix.mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockResolvedValueOnce({ items: [{ ...row, group_id: 9, model: 'current' }] })
    const wrapper = mount(MonitorModelDetails, { props: { group, filter, showThroughput: false }, global })
    await wrapper.setProps({ group: { ...group, group_id: 9 } })
    await flushPromises()
    finish({ items: [row] })
    await flushPromises()
    expect(wrapper.text()).toContain('current')
    expect(wrapper.text()).not.toContain('gpt-test')
    expect(wrapper.text()).not.toContain('channelMonitorV2.metrics.tps')
    wrapper.unmount()
  })
  it('aborts the pending request when closed, without starting another request', async () => {
    mocks.getMatrix.mockImplementation(() => new Promise(() => undefined))
    const wrapper = mount(MonitorModelDetails, { props: { group, filter }, global })
    const signal = mocks.getMatrix.mock.calls[0][3] as AbortSignal
    await wrapper.setProps({ group: null })
    expect(signal.aborted).toBe(true)
    expect(mocks.getMatrix).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('keeps unknown generation speed distinct from zero or aggregate throughput', () => {
    expect(formatMonitorOutputTPS(null)).toBe('—')
    expect(formatMonitorOutputTPS(undefined)).toBe('—')
    expect(formatMonitorOutputTPS(Number.NaN)).toBe('—')
    expect(formatMonitorOutputTPS(0)).toBe('0')
    expect(formatMonitorOutputTPS(12.5)).toBe('12.5')
  })
})
