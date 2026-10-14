import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import VideoTasksView from '../VideoTasksView.vue'

const { route, listVideoTasks, getVideoTaskStats } = vi.hoisted(() => ({
  route: { path: '/video-tasks' }, listVideoTasks: vi.fn(), getVideoTaskStats: vi.fn(),
}))
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 2 } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteVersion: 'test' }) }))
vi.mock('@/api/videoTasks', () => ({ listVideoTasks, getVideoTaskStats, downloadVideoTask: vi.fn(), refreshVideoTask: vi.fn() }))

const task = { id: 'local-id', task_id: 'remote-id', user_id: 2, api_key_id: 166,
  model: 'video-model', status: 'done', seconds: 5, resolution: '720p',
  usage_id: 461985, actual_cost: 3.75, billed: true, usage_recorded: true,
  created_at: '2026-01-03T12:00:00Z', updated_at: '2026-01-06T12:00:00Z' }

describe('video receipt navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listVideoTasks.mockResolvedValue({ items: [task], has_more: false })
    getVideoTaskStats.mockResolvedValue({ total: 3, succeeded: 1, failed: 1, in_progress: 1, needs_review: 0, success_rate: 50,
      successful_seconds: 5, actual_cost: 3.75, unreconciled: 0, average_completion_seconds: 60, models: [] })
  })
  for (const admin of [false, true]) {
    it(`includes the original key and dates for ${admin ? 'admin' : 'owner'}`, async () => {
      route.path = admin ? '/admin/video-tasks' : '/video-tasks'
      const wrapper = mount(VideoTasksView, { global: { stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        RouterLink: { name: 'RouterLink', props: ['to'], template: '<a><slot /></a>' },
      } } })
      await flushPromises()
      expect(wrapper.text()).toContain('50.0%')
      expect(wrapper.text()).toContain('默认近 30 天')
      const listParams = listVideoTasks.mock.calls[0][1]
      const statsParams = getVideoTaskStats.mock.calls[0][1]
      const { page, page_size, ...listFilters } = listParams
      expect(page).toBe(1)
      expect(page_size).toBe(20)
      expect(listFilters).toEqual(statsParams)
      expect(statsParams.user_id).toBeUndefined()
      const link = wrapper.findAllComponents({ name: 'RouterLink' }).find(item => item.text().includes('461985'))!
      expect(link.props('to')).toEqual({
        path: admin ? '/admin/usage' : '/usage', query: {
          api_key_id: '166', start_date: '2026-01-03', end_date: '2026-01-06',
          ...(admin ? { user_id: '2' } : {}),
        },
      })
      wrapper.unmount()
    })
  }

  it('rejects a date window over 93 days before either request', async () => {
    route.path = '/video-tasks'
    const wrapper = mount(VideoTasksView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, RouterLink: true } } })
    await flushPromises()
    vi.clearAllMocks()
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2020-01-01')
    await dates[1].setValue('2026-01-01')
    await wrapper.findAll('button').find(button => button.text() === '查询 / 刷新')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('单次查询范围最多 93 天')
    expect(listVideoTasks).not.toHaveBeenCalled()
    expect(getVideoTaskStats).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps the task list available when statistics time out', async () => {
    route.path = '/video-tasks'
    getVideoTaskStats.mockRejectedValueOnce(new Error('统计超时'))
    const wrapper = mount(VideoTasksView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, RouterLink: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('video-model')
    expect(wrapper.text()).toContain('统计暂未读取成功')
    expect(wrapper.find('[aria-label="视频任务统计"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
