import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { useAccountRequestMetrics } from '../useAccountRequestMetrics'

const api = vi.hoisted(() => ({ getAccountRequestStats: vi.fn() }))
vi.mock('@/api/admin/accountRequestStats', () => api)
describe('optional account metrics', () => {
  beforeEach(() => { vi.useFakeTimers(); api.getAccountRequestStats.mockReset().mockResolvedValue({}); Object.defineProperty(document, 'hidden', { configurable: true, value: false }) })
  afterEach(() => { vi.useRealTimers() })
  it('does not request or poll until enabled, and stops on close', async () => {
    const enabled = ref(false), ids = ref([1, 2])
    const wrapper = mount(defineComponent({ setup() { useAccountRequestMetrics(ids, enabled); return () => null } }))
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.getAccountRequestStats).not.toHaveBeenCalled()
    enabled.value = true
    await flushPromises()
    expect(api.getAccountRequestStats).toHaveBeenCalledTimes(1)
    expect(api.getAccountRequestStats).toHaveBeenCalledWith([1, 2], expect.any(AbortSignal))
    enabled.value = false
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.getAccountRequestStats).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('batches visible accounts in groups of at most 100 and skips hidden pages', async () => {
    const enabled = ref(true), ids = ref(Array.from({ length: 201 }, (_, i) => i + 1))
    const wrapper = mount(defineComponent({ setup() { useAccountRequestMetrics(ids, enabled); return () => null } }))
    await flushPromises()
    expect(api.getAccountRequestStats.mock.calls.map(call => call[0].length)).toEqual([100, 100, 1])
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    await vi.advanceTimersByTimeAsync(30_000)
    expect(api.getAccountRequestStats).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })
})
