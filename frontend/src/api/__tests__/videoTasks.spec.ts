import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))
import { downloadVideoTask, listVideoTasks, refreshVideoTask, type VideoTaskSummary } from '../videoTasks'

const task = { task_id: 'task/with space', api_key_id: 17 } as VideoTaskSummary
describe('video task account binding and download', () => {
  beforeEach(() => get.mockReset())
  it('uses authenticated owner listing and a separate admin route', async () => {
    get.mockResolvedValue({ data: { items: [], has_more: false } })
    await listVideoTasks(false, { page: 1, page_size: 20 })
    expect(get.mock.calls[0][0]).toBe('/video-tasks')
    await listVideoTasks(true, { page: 1, page_size: 20, user_id: 42 })
    expect(get.mock.calls[1][0]).toBe('/admin/video-tasks')
  })
  it('keeps the original Key ID for query and download; never resubmits generation', async () => {
    get.mockResolvedValueOnce({ data: { status: 'completed' } })
    await refreshVideoTask(task)
    expect(get).toHaveBeenLastCalledWith('/canvas-api/v1/videos/task%2Fwith%20space', expect.objectContaining({ headers: { 'X-Canvas-Key-ID': '17' } }))
    const body = new Blob(['complete-video'], { type: 'video/mp4' })
    get.mockResolvedValueOnce({ data: body })
    const controller = new AbortController()
    const result = await downloadVideoTask(task, controller.signal, vi.fn())
    expect(result).toBe(body)
    expect(get).toHaveBeenLastCalledWith('/canvas-api/v1/videos/task%2Fwith%20space/content', expect.objectContaining({ responseType: 'blob', signal: controller.signal, headers: { 'X-Canvas-Key-ID': '17' } }))
  })
  it('rejects empty files and a successful HTTP response containing an error document', async () => {
    for (const body of [new Blob([]), new Blob(['{"error":"expired"}'], { type: 'application/json' }), new Blob(['<html>error</html>'], { type: 'text/html' })]) {
      get.mockResolvedValueOnce({ data: body })
      await expect(downloadVideoTask(task, new AbortController().signal, vi.fn())).rejects.toThrow('未取得完整视频文件')
    }
  })
  it('rejects an unsolicited partial video range and mismatched length', async () => {
    const body = new Blob(['video'], { type: 'video/mp4' })
    get.mockResolvedValueOnce({ data: body, status: 206, headers: { 'content-range': 'bytes 0-4/100' } })
    await expect(downloadVideoTask(task, new AbortController().signal, vi.fn())).rejects.toThrow('只返回部分内容')
    get.mockResolvedValueOnce({ data: body, status: 200, headers: { 'content-length': '100' } })
    await expect(downloadVideoTask(task, new AbortController().signal, vi.fn())).rejects.toThrow('下载不完整')
  })
})
