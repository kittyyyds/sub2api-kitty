import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { videoGenerationAPI, type VideoGenerationStatus } from '@/api/videoGeneration'
import { SUANLI_VIDEO_MODELS } from '../../../suanliVideo'
import SuanliSpecialVideoRunner from '../SuanliSpecialVideoRunner.vue'
import { uploadVideoReference } from '@/api/videoReferences'

vi.mock('@/api/videoReferences', async (importOriginal) => ({ ...(await importOriginal<typeof import('@/api/videoReferences')>()), uploadVideoReference: vi.fn() }))

vi.mock('@/api/videoGeneration', () => ({
  videoGenerationAPI: {
    submitGateway: vi.fn(), status: vi.fn(), createContentURL: vi.fn(),
  },
}))

const props = {
  apiKey: 'customer-test-key',
  modelIds: SUANLI_VIDEO_MODELS.map((model) => model.id),
  disabled: false,
}
const statusResult = (status = 'completed'): VideoGenerationStatus => ({
  requestId: 'task-one', status, videoUrl: 'https://outside.example/video.mp4', errorMessage: '',
})
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}
const taskInput = '[data-testid="suanli-task-id"]'
const downloadButton = '[data-testid="suanli-download"]'
const queryButton = '[data-testid="suanli-query"]'
const stopButton = '[data-testid="suanli-stop"]'

describe('SuanliSpecialVideoRunner', () => {
  enableAutoUnmount((unmount) => afterEach(() => {
    unmount()
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  }))
  beforeEach(() => {
    vi.resetAllMocks()
    vi.stubGlobal('URL', class extends URL { static revokeObjectURL = vi.fn() })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  it('shows the task panel before generation fields and offers nine models without making requests', () => {
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    expect(wrapper.get('[data-testid="suanli-model"]').findAll('option').map((option) => option.attributes('value'))).toEqual(props.modelIds)
    expect(wrapper.get(downloadButton).isVisible()).toBe(true)
    expect(wrapper.get(downloadButton).attributes('disabled')).toBeDefined()
    const panel = wrapper.get('[data-testid="suanli-task-result"]').element
    expect(panel.nextElementSibling?.tagName).toBe('FIELDSET')
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(videoGenerationAPI.status).not.toHaveBeenCalled()
    expect(videoGenerationAPI.createContentURL).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('本站测试范围')
  })

  it('fixes SD2.5 at 30 seconds and removes audio fields when switching from H3', async () => {
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get('[data-testid="suanli-model"]').setValue('特价/官方h3-2k')
    expect(wrapper.text()).toContain('参考音频链接')
    await wrapper.get('[data-testid="suanli-model"]').setValue('特价/sd-2.5')
    expect(wrapper.find('input[type="number"]').element).toHaveProperty('value', '30')
    expect(wrapper.find('input[type="number"]').attributes()).toHaveProperty('readonly')
    expect(wrapper.text()).not.toContain('参考音频链接')
  })

  it('does not offer cancellation before a pending submission returns its task ID', async () => {
    const pending = deferred<{ requestId: string }>()
    vi.mocked(videoGenerationAPI.submitGateway).mockReturnValue(pending.promise)
    vi.mocked(videoGenerationAPI.status).mockResolvedValue(statusResult())
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get('textarea').setValue('角色自然转身')
    await wrapper.get('[data-testid="suanli-images"]').setValue('https://example.com/reference.jpg')
    await wrapper.get('form').trigger('submit')

    expect(wrapper.text()).toContain('任务正在提交，请等待返回任务 ID。')
    expect(wrapper.find(stopButton).exists()).toBe(false)
    expect(wrapper.get(taskInput).attributes('disabled')).toBeDefined()
    expect(wrapper.get(downloadButton).attributes('disabled')).toBeDefined()
    expect(videoGenerationAPI.status).not.toHaveBeenCalled()

    pending.resolve({ requestId: 'task-one' })
    await flushPromises()
    expect(videoGenerationAPI.status).toHaveBeenCalledWith(props.apiKey, 'task-one', expect.any(AbortSignal))
    expect(videoGenerationAPI.createContentURL).not.toHaveBeenCalled()
  })

  it('submits only on user action and displays completed status without fetching the file', async () => {
    vi.mocked(videoGenerationAPI.submitGateway).mockResolvedValue({ requestId: 'task-one' })
    vi.mocked(videoGenerationAPI.status).mockResolvedValue(statusResult())
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get('textarea').setValue('角色自然转身')
    await wrapper.get('[data-testid="suanli-images"]').setValue('https://example.com/reference.jpg')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(videoGenerationAPI.submitGateway).toHaveBeenCalledWith(props.apiKey, '/v1/videos/generations', expect.objectContaining({ model: props.modelIds[0], seconds: 5 }), expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('生成完成，点击“下载视频”')
    expect(wrapper.get(downloadButton).attributes('disabled')).toBeUndefined()
    expect(videoGenerationAPI.createContentURL).not.toHaveBeenCalled()
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('outside.example')
  })

  it('queries an existing task with empty generation fields and no currently available model', async () => {
    vi.mocked(videoGenerationAPI.status).mockResolvedValue(statusResult())
    const wrapper = mount(SuanliSpecialVideoRunner, { props: { ...props, modelIds: [] } })
    await wrapper.get(taskInput).setValue('  recovered-task  ')
    await wrapper.get(queryButton).trigger('click')
    await flushPromises()

    expect(videoGenerationAPI.status).toHaveBeenCalledWith(props.apiKey, 'recovered-task', expect.any(AbortSignal))
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(videoGenerationAPI.createContentURL).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('completed')
  })

  it('downloads by task ID through the authenticated API, keeps the control visible, and reuses the local file', async () => {
    const pending = deferred<string>()
    vi.mocked(videoGenerationAPI.createContentURL).mockReturnValue(pending.promise)
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('saved-task')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()

    expect(wrapper.get(downloadButton).isVisible()).toBe(true)
    expect(wrapper.get(downloadButton).text()).toContain('正在获取视频')
    expect(wrapper.get(stopButton).text()).toBe('取消下载')
    expect(wrapper.get(taskInput).attributes('disabled')).toBeUndefined()
    await wrapper.get(downloadButton).trigger('click')
    expect(videoGenerationAPI.createContentURL).toHaveBeenCalledTimes(1)
    expect(videoGenerationAPI.createContentURL).toHaveBeenCalledWith(props.apiKey, 'saved-task', expect.any(AbortSignal))
    expect(videoGenerationAPI.status).not.toHaveBeenCalled()
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()

    pending.resolve('blob:site-video')
    await flushPromises()
    expect(wrapper.get('video').attributes('src')).toBe('blob:site-video')
    expect(wrapper.get('a[download]').attributes('href')).toBe('blob:site-video')
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
    expect(wrapper.html()).not.toContain(props.apiKey)
    await wrapper.get(downloadButton).trigger('click')
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(2)
    expect(videoGenerationAPI.createContentURL).toHaveBeenCalledTimes(1)
  })

  it('can switch from polling to downloading without starting a second generation', async () => {
    const pendingStatus = deferred<VideoGenerationStatus>()
    vi.mocked(videoGenerationAPI.status).mockReturnValue(pendingStatus.promise)
    vi.mocked(videoGenerationAPI.createContentURL).mockResolvedValue('blob:downloaded')
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(queryButton).trigger('click')
    await flushPromises()
    const statusSignal = vi.mocked(videoGenerationAPI.status).mock.calls[0][2]
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()

    expect(statusSignal?.aborted).toBe(true)
    pendingStatus.resolve(statusResult('failed'))
    await flushPromises()
    expect(wrapper.get('video').attributes('src')).toBe('blob:downloaded')
    expect(wrapper.text()).not.toContain('状态：failed')
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it('ends a stuck status request after 30 seconds and allows retry without resubmission', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const pending = deferred<VideoGenerationStatus>()
    vi.mocked(videoGenerationAPI.status).mockReturnValueOnce(pending.promise).mockResolvedValueOnce(statusResult())
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(queryButton).trigger('click')
    await flushPromises()
    const signal = vi.mocked(videoGenerationAPI.status).mock.calls[0][2]
    await vi.advanceTimersByTimeAsync(30000)
    await flushPromises()

    expect(signal?.aborted).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('状态查询超时')
    expect(wrapper.get(queryButton).attributes('disabled')).toBeUndefined()
    await wrapper.get(queryButton).trigger('click')
    await flushPromises()
    pending.resolve(statusResult('failed'))
    await flushPromises()
    expect(wrapper.text()).toContain('状态：completed')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(videoGenerationAPI.createContentURL).not.toHaveBeenCalled()
  })

  it('ends a stuck file request after 90 seconds, retries, and revokes a late result', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const pending = deferred<string>()
    vi.mocked(videoGenerationAPI.createContentURL).mockReturnValueOnce(pending.promise).mockResolvedValueOnce('blob:retry')
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    const signal = vi.mocked(videoGenerationAPI.createContentURL).mock.calls[0][2]
    await vi.advanceTimersByTimeAsync(90000)
    await flushPromises()

    expect(signal?.aborted).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('获取视频超时')
    expect(wrapper.get(downloadButton).text()).toBe('重试下载')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    pending.resolve('blob:late-timeout')
    await flushPromises()
    expect(wrapper.get('video').attributes('src')).toBe('blob:retry')
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:late-timeout')
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it.each(['cancel', 'key', 'task', 'unmount'] as const)('discards a late download after %s', async (action) => {
    const pending = deferred<string>()
    vi.mocked(videoGenerationAPI.createContentURL).mockReturnValue(pending.promise)
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    const signal = vi.mocked(videoGenerationAPI.createContentURL).mock.calls[0][2]
    if (action === 'cancel') await wrapper.get(stopButton).trigger('click')
    if (action === 'key') await wrapper.setProps({ apiKey: 'another-key' })
    if (action === 'task') await wrapper.get(taskInput).setValue('task-two')
    if (action === 'unmount') wrapper.unmount()
    expect(signal?.aborted).toBe(true)
    pending.resolve('blob:late')
    await flushPromises()

    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:late')
    expect(HTMLAnchorElement.prototype.click).not.toHaveBeenCalled()
    if (action !== 'unmount') {
      expect(wrapper.find('video').exists()).toBe(false)
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(wrapper.get(taskInput).element).toHaveProperty('value', action === 'task' ? 'task-two' : action === 'key' ? '' : 'task-one')
    }
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it('ignores old status and submission promises when their task or key changes', async () => {
    const pendingStatus = deferred<VideoGenerationStatus>()
    vi.mocked(videoGenerationAPI.status).mockReturnValueOnce(pendingStatus.promise).mockResolvedValueOnce(statusResult())
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('old-task')
    await wrapper.get(queryButton).trigger('click')
    await flushPromises()
    await wrapper.get(taskInput).setValue('new-task')
    await wrapper.get(queryButton).trigger('click')
    await flushPromises()
    pendingStatus.resolve(statusResult('failed'))
    await flushPromises()
    expect(wrapper.text()).toContain('状态：completed')

    const submission = deferred<{ requestId: string }>()
    vi.mocked(videoGenerationAPI.submitGateway).mockReturnValue(submission.promise)
    await wrapper.get('textarea').setValue('角色自然转身')
    await wrapper.get('[data-testid="suanli-images"]').setValue('https://example.com/reference.jpg')
    await wrapper.get('form').trigger('submit')
    await wrapper.setProps({ apiKey: 'another-key' })
    submission.resolve({ requestId: 'late-submit' })
    await flushPromises()
    expect(wrapper.get(taskInput).element).toHaveProperty('value', '')
    expect(videoGenerationAPI.status).toHaveBeenCalledTimes(2)
  })

  it.each(['key', 'task', 'unmount'] as const)('reclaims the saved Blob URL on %s', async (action) => {
    vi.mocked(videoGenerationAPI.createContentURL).mockResolvedValue('blob:current')
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    if (action === 'key') await wrapper.setProps({ apiKey: 'another-key' })
    if (action === 'task') await wrapper.get(taskInput).setValue('task-two')
    if (action === 'unmount') wrapper.unmount()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:current')
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1)
    if (action !== 'unmount') expect(wrapper.find('video').exists()).toBe(false)
  })

  it('keeps failures retryable and hides the key if an error response echoes it', async () => {
    vi.mocked(videoGenerationAPI.createContentURL)
      .mockRejectedValueOnce(new Error(`temporary failure ${props.apiKey}`))
      .mockResolvedValueOnce('blob:recovered')
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    expect(wrapper.get(downloadButton).text()).toBe('重试下载')
    expect(wrapper.get('[role="alert"]').text()).not.toContain(props.apiKey)
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    expect(wrapper.get('video').attributes('src')).toBe('blob:recovered')
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it('never uses an external URL as a download or preview', async () => {
    vi.mocked(videoGenerationAPI.createContentURL).mockResolvedValue('https://outside.example/video.mp4')
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    await wrapper.get(taskInput).setValue('task-one')
    await wrapper.get(downloadButton).trigger('click')
    await flushPromises()
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.find('a[download]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('视频文件响应无效')
    expect(HTMLAnchorElement.prototype.click).not.toHaveBeenCalled()
  })

  it('uploads selected local files only on submit and sends the temporary URL with one generation', async () => {
    const file = new File(['image-fixture'], 'image.png', { type: 'image/png' })
    vi.mocked(uploadVideoReference).mockResolvedValue({ id: 'reference-id', url: 'https://media.example/reference', expires_at: '2026-09-23T00:00:00Z', bytes: file.size, mime_type: 'image/png' })
    vi.mocked(videoGenerationAPI.submitGateway).mockResolvedValue({ requestId: 'task-one' })
    vi.mocked(videoGenerationAPI.status).mockResolvedValue(statusResult())
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    const input = wrapper.get('[data-testid="suanli-image-files"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    expect(uploadVideoReference).not.toHaveBeenCalled()
    await wrapper.get('textarea').setValue('参考图人物自然转身')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(uploadVideoReference).toHaveBeenCalledWith(props.apiKey, file, 'image', expect.any(AbortSignal))
    expect(videoGenerationAPI.submitGateway).toHaveBeenCalledTimes(1)
    const sent = vi.mocked(videoGenerationAPI.submitGateway).mock.calls[0][2]
    expect(sent).toMatchObject({ reference_images: [{ url: 'https://media.example/reference', role: 'reference_image' }] })
    expect(JSON.stringify(sent)).not.toContain('reference.invalid')
    expect(wrapper.text()).toContain('24 小时后自动清理')
  })

  it('never submits a generation when local upload fails or finishes under a replaced key', async () => {
    const file = new File(['fixture'], 'image.png', { type: 'image/png' })
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    const input = wrapper.get('[data-testid="suanli-image-files"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change'); await wrapper.get('textarea').setValue('参考素材')
    vi.mocked(uploadVideoReference).mockRejectedValueOnce(new Error('素材上传失败'))
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('素材上传失败')
    const pending = deferred<Awaited<ReturnType<typeof uploadVideoReference>>>()
    vi.mocked(uploadVideoReference).mockReturnValue(pending.promise)
    await wrapper.get('form').trigger('submit'); await wrapper.setProps({ apiKey: 'replacement-key' })
    pending.resolve({ id: 'reference-id', url: 'https://media.example/reference', expires_at: '2026-09-23T00:00:00Z', bytes: 7, mime_type: 'image/png' })
    await flushPromises()
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
  })

  it('validates combined local and URL reference counts before upload', async () => {
    const wrapper = mount(SuanliSpecialVideoRunner, { props })
    const input = wrapper.get('[data-testid="suanli-image-files"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['fixture'], 'image.png', { type: 'image/png' })] })
    await input.trigger('change'); await wrapper.get('textarea').setValue('参考素材')
    await wrapper.get('[data-testid="suanli-images"]').setValue('https://example.com/reference.png')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(uploadVideoReference).not.toHaveBeenCalled()
    expect(videoGenerationAPI.submitGateway).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('1–1 张参考图')
  })
})
