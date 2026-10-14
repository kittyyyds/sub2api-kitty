import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import VideoTasksView from '../VideoTasksView.vue'

const mocks = vi.hoisted(() => ({
  supported: true,
  list: vi.fn(), stats: vi.fn(), blob: vi.fn(), request: vi.fn(), picker: vi.fn(), disk: vi.fn(),
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ path: '/video-tasks' }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 2 } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteVersion: 'test' }) }))
vi.mock('@/api/videoTasks', () => ({ listVideoTasks: mocks.list, getVideoTaskStats: mocks.stats,
  downloadVideoTask: mocks.blob, refreshVideoTask: vi.fn(), fetchVideoTaskContent: mocks.request }))
vi.mock('@/utils/videoDownload', () => ({ supportsVideoFileDownload: () => mocks.supported,
  pickVideoDownloadFile: mocks.picker, downloadVideoToFile: mocks.disk }))

const task = { id: 'local', task_id: 'existing-task', user_id: 2, api_key_id: 166,
  model: 'video', status: 'done', seconds: 5, resolution: '720p', actual_cost: 1,
  created_at: '2026-09-20T00:00:00Z', updated_at: '2026-09-20T00:00:00Z' }
const setup = () => mount(VideoTasksView, { global: { stubs: {
  AppLayout: { template: '<div><slot /></div>' }, RouterLink: true,
} } })

describe('optional video file saving', () => {
  beforeEach(() => {
    vi.clearAllMocks(); mocks.supported = true
    mocks.list.mockResolvedValue({ items: [task], has_more: false }); mocks.stats.mockResolvedValue(null)
    mocks.picker.mockResolvedValue({ identity: '2:166:existing-task', bytes: 0, complete: false })
    mocks.disk.mockResolvedValue({ fileName: 'video.mp4', bytes: 6, resumed: false })
  })

  it('file mode reads the existing result without using the full Blob downloader', async () => {
    mocks.disk.mockImplementationOnce(async (options) => {
      expect(options.identity).toBe('2:166:existing-task')
      await options.request({ Range: 'bytes=3-', 'If-Range': '"v1"' }, options.signal)
      return { fileName: 'video.mp4', bytes: 6, resumed: true }
    })
    const wrapper = setup(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === '保存到文件 / 继续')!.trigger('click'); await flushPromises()
    expect(mocks.picker).toHaveBeenCalledWith('2:166:existing-task', 'kitty-video-existing-task.mp4')
    expect(mocks.request).toHaveBeenCalledWith(expect.objectContaining({ task_id: 'existing-task' }), { Range: 'bytes=3-', 'If-Range': '"v1"' }, expect.any(AbortSignal))
    expect(mocks.blob).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('完整视频已保存为 video.mp4')
    wrapper.unmount()
  })

  it('retry retains the same file session for the same task', async () => {
    mocks.disk.mockRejectedValueOnce(new Error('connection dropped'))
    const wrapper = setup(); await flushPromises()
    const button = () => wrapper.findAll('button').find(b => b.text() === '保存到文件 / 继续')!
    await button().trigger('click'); await flushPromises()
    await button().trigger('click'); await flushPromises()
    expect(mocks.picker).toHaveBeenCalledTimes(1)
    expect(mocks.disk).toHaveBeenCalledTimes(2)
    expect(mocks.disk.mock.calls[0][0].session).toBe(mocks.disk.mock.calls[1][0].session)
    wrapper.unmount()
  })

  it('unsupported browsers retain ordinary download and explain full-file buffering', async () => {
    mocks.supported = false
    const wrapper = setup(); await flushPromises()
    expect(wrapper.findAll('button').some(b => b.text() === '保存到文件 / 继续')).toBe(false)
    expect(wrapper.findAll('button').some(b => b.text() === '下载视频')).toBe(true)
    expect(wrapper.text()).toContain('普通下载会先缓存完整视频再保存')
    expect(mocks.picker).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('another owner cannot use either download action', async () => {
    mocks.list.mockResolvedValue({ items: [{ ...task, user_id: 9 }], has_more: false })
    const wrapper = setup(); await flushPromises()
    expect(wrapper.text()).toContain('由任务所属用户下载')
    expect(wrapper.findAll('button').some(b => b.text().includes('保存到文件'))).toBe(false)
    wrapper.unmount()
  })
})
