import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import DashboardAnnouncementFeed from '../DashboardAnnouncementFeed.vue'
import { homeMessages } from '@/i18n/homeOverview'
import { useAnnouncementStore } from '@/stores/announcements'

const api = vi.hoisted(() => ({ list: vi.fn(), markRead: vi.fn() }))
vi.mock('@/api', () => ({ announcementsAPI: api }))
const notice = { id: 1, title: '公告', content: '**正文** <img src=x onerror="alert(1)"><script>alert(1)</script>', notify_mode: 'silent', created_at: '2026-10-07T00:00:00Z', updated_at: '2026-10-07T00:00:00Z' }
beforeEach(() => { setActivePinia(createPinia()); api.list.mockResolvedValue([structuredClone(notice)]); api.markRead.mockResolvedValue({}); vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => { vi.restoreAllMocks() })
function render() {
  return mount(DashboardAnnouncementFeed, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { homeOverview: homeMessages.zh } } })], stubs: { Icon: true, BaseDialog: { props: ['show'], template: '<div v-if="show" role="dialog"><slot /><slot name="footer" /></div>' } } } })
}
describe('dashboard announcement content and read state', () => {
  it('marks each opened notice even if the previous read is still pending', async () => {
    api.list.mockResolvedValue([structuredClone(notice), { ...notice, id: 2, title: '第二条' }])
    const finishes: Array<() => void> = []
    api.markRead.mockImplementation(() => new Promise<void>(resolve => finishes.push(resolve)))
    const wrapper = render(); await flushPromises()
    await wrapper.findAll('.home-notice-list button')[0].trigger('click')
    await wrapper.findAll('.home-notice-list button')[1].trigger('click')
    expect(api.markRead.mock.calls.map(([id]) => id).sort()).toEqual([1, 2])
    finishes.forEach(finish => finish()); await flushPromises()
    expect(useAnnouncementStore().unreadCount).toBe(0)
    wrapper.unmount()
  })
  it('renders sanitized Markdown and leaves visible announcements unread until requested', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.get('.home-notice-excerpt').text()).toContain('正文')
    expect(wrapper.find('.home-notice-excerpt img').exists()).toBe(false)
    expect(api.markRead).not.toHaveBeenCalled()
    expect(useAnnouncementStore().unreadCount).toBe(1)
    await wrapper.get('.home-notice-list button').trigger('click'); await flushPromises()
    expect(wrapper.get('.home-notice-dialog strong').text()).toBe('正文')
    expect(wrapper.find('.home-notice-dialog script').exists()).toBe(false)
    expect(wrapper.get('.home-notice-dialog img').attributes('onerror')).toBeUndefined()
    expect(api.markRead).toHaveBeenCalledWith(1)
    wrapper.unmount()
  })
  it('reports a read error without marking the notice read, then permits retry', async () => {
    api.markRead.mockRejectedValueOnce(new Error('offline'))
    const wrapper = render(); await flushPromises()
    await wrapper.get('.home-notice-list button').trigger('click'); await flushPromises()
    expect(wrapper.findAll('[role="alert"]')[0].text()).toContain('已读状态保存失败')
    expect(useAnnouncementStore().unreadCount).toBe(1)
    await wrapper.get('[role="dialog"] button').trigger('click'); await flushPromises()
    expect(useAnnouncementStore().unreadCount).toBe(0)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
