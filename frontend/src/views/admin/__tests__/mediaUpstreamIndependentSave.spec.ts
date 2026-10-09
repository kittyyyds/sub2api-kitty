import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import View from '../VideoUpstreamView.vue'
import { imageUpstreamAPI } from '@/api/admin/imageUpstream'
import { videoUpstreamAPI } from '@/api/admin/videoUpstream'
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

vi.mock('@/api/admin/groups', () => ({ default: {}, list: vi.fn().mockResolvedValue({ items: [] }) }))
vi.mock('@/api/admin/videoUpstream', () => ({ videoUpstreamAPI: {
  get: vi.fn().mockResolvedValue({ settings: { markup: {}, interval_seconds: 60 }, catalog: { models: {}, changes: [] } }),
  save: vi.fn().mockResolvedValue({}), sync: vi.fn(), import: vi.fn()
} }))
vi.mock('@/api/admin/imageUpstream', () => ({ imageUpstreamAPI: {
  get: vi.fn().mockResolvedValue({ settings: { auto_sync: false, auto_import_account_models: false, markup: 0, follow_increase: true, follow_decrease: false, interval_seconds: 60 }, catalog: { models: {} }, groups: [] }),
  save: vi.fn().mockResolvedValue({}), sync: vi.fn().mockResolvedValue({}), price: vi.fn()
} }))
describe('media upstream independent settings', () => {
  beforeEach(() => vi.clearAllMocks())
  it('saves image settings without sending video settings', async () => {
    const view = mount(View, { global: { stubs: { AppLayout: { template: '<main><slot /></main>' } } } })
    await flushPromises()
    await view.findAll('[role="tab"]')[1]!.trigger('click')
    const panel = view.get('[data-testid="image-upstream-panel"]')
    await panel.get('[aria-label="图片每次加价"]').setValue('0.17')
    await panel.get('form').trigger('submit')
    await flushPromises()
    expect(imageUpstreamAPI.save).toHaveBeenCalledWith(expect.objectContaining({ markup: 0.17 }))
    expect(videoUpstreamAPI.save).not.toHaveBeenCalled()
  })
  it('read-only price lookup never applies customer prices', async () => {
    const view = mount(View, { global: { stubs: { AppLayout: { template: '<main><slot /></main>' } } } })
    await flushPromises()
    const button = view.findAll('button').find(x => x.text() === '只读取上游价格')!
    await button.trigger('click'); await flushPromises()
    expect(imageUpstreamAPI.sync).toHaveBeenCalledWith(false)
    expect(imageUpstreamAPI.save).not.toHaveBeenCalled()
    expect(videoUpstreamAPI.sync).not.toHaveBeenCalled()
  })
})
