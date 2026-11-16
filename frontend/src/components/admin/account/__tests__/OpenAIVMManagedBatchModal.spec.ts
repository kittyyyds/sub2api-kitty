import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OpenAIVMManagedBatchModal from '../OpenAIVMManagedBatchModal.vue'
import type { AdminGroup, Proxy } from '@/types'
import type { OpenAIVMBatchJob, OpenAIVMBatchPreview } from '@/api/admin/openaiVmManaged'

const api = vi.hoisted(() => ({ batchPreview: vi.fn(), batchStart: vi.fn(), batchStatus: vi.fn(), batchCancel: vi.fn() }))
vi.mock('@/api/admin/openaiVmManaged', async () => ({
  ...await vi.importActual<typeof import('@/api/admin/openaiVmManaged')>('@/api/admin/openaiVmManaged'),
  openaiVmManagedAPI: api
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key),
    te: (key: string) => ['batch.reason.already_managed', 'batch.reason.invalid_format', 'attention.password_rejected', 'slotError.login_busy'].some(known => key.endsWith(known))
  })
}))

const proxies = [
  { id: 1, name: 'exit-one', protocol: 'socks5', host: 'one.example', port: 1080, status: 'active' },
  { id: 2, name: 'exit-two', protocol: 'socks5h', host: 'two.example', port: 1080, status: 'active' },
  { id: 3, name: 'http-proxy', protocol: 'http', host: 'http.example', port: 8080, status: 'active' }
] as unknown as Proxy[]
const groups = [{ id: 5, name: 'openai-default', platform: 'openai' }, { id: 6, name: 'claude', platform: 'anthropic' }] as unknown as AdminGroup[]
const text = 'a@example.test---typed-password-a---JBSWY3DPEHPK3PXP\nb@example.test---typed-password-b---JBSWY3DPEHPK3PXP'

const previewResult: OpenAIVMBatchPreview = {
  importable: 1,
  problems: [{ line_no: 2, email: 'b@example.test', code: 'already_managed' }, { line_no: 3, code: 'brand_new_code' }],
  lines: [{ line_no: 1, email: 'a@example.test', proxy_id: 1 }],
  estimated_memory_mb: 1024,
  estimated_disk_gb: 20
}
const jobOf = (status: string, items: Array<Partial<OpenAIVMBatchJob['items'][number]>>): OpenAIVMBatchJob => ({
  id: 9, status, runtime_type: 'kvm', total: items.length, created_at: '2026-11-12T08:00:00Z',
  items: items.map((item, index) => ({ id: index + 1, line_no: index + 1, email: `u${index + 1}@example.test`, proxy_id: 1, account_id: null, status: 'queued', updated_at: '2026-11-12T08:00:00Z', ...item }))
})

function mountModal() {
  return mount(OpenAIVMManagedBatchModal, {
    props: { show: true, proxies, groups },
    global: { stubs: {
      BaseDialog: { props: ['show'], emits: ['close'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      TotpStepUpDialog: { props: ['controller'], template: '<div />' }
    } }
  })
}

async function fill(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('#batch-text').setValue(text)
  await wrapper.findAll('input[type="checkbox"]')[0].setValue(true)
}

describe('VM2-managed batch import', () => {
  beforeEach(() => {
    Object.values(api).forEach(fn => fn.mockReset())
    api.batchPreview.mockResolvedValue(previewResult)
  })
  afterEach(() => { vi.useRealTimers() })

  it('lists only SOCKS5 proxies and OpenAI groups, and cannot start before a check', async () => {
    const wrapper = mountModal()
    expect(wrapper.text()).toContain('exit-one')
    expect(wrapper.text()).toContain('exit-two')
    expect(wrapper.text()).not.toContain('http-proxy')
    expect(wrapper.text()).toContain('openai-default')
    expect(wrapper.text()).not.toContain('claude')
    expect(wrapper.get('[data-testid="batch-start"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="batch-preview-button"]').attributes('disabled')).toBeDefined()
    await fill(wrapper)
    expect(wrapper.get('[data-testid="batch-preview-button"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="batch-start"]').attributes('disabled')).toBeDefined()
  })

  it('shows what will happen per line before anything is created', async () => {
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    expect(api.batchPreview).toHaveBeenCalledWith({ text, proxy_ids: [1], per_proxy: 1, runtime_type: 'kvm', group_ids: [], concurrency: 3, priority: 1 })
    expect(api.batchStart).not.toHaveBeenCalled()
    const preview = wrapper.get('[data-testid="batch-preview"]').text()
    expect(preview).toContain('"ok":1')
    expect(preview).toContain('"bad":2')
    expect(preview).toContain('"proxy":"exit-one"')
    expect(wrapper.get('[data-testid="batch-footprint"]').text()).toContain('"memory":1')
    const problems = wrapper.get('[data-testid="batch-problems"]').text()
    expect(problems).toContain('admin.accounts.vmManaged.batch.reason.already_managed')
    // A code without its own wording is shown as the code, not hidden.
    expect(problems).toContain('brand_new_code')
    expect(wrapper.get('[data-testid="batch-start"]').attributes('disabled')).toBeUndefined()
  })

  it('makes the administrator check again after any change to the input', async () => {
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    await wrapper.get('#batch-text').setValue(text + '\nc@example.test---typed-password-c---JBSWY3DPEHPK3PXP')
    expect(wrapper.find('[data-testid="batch-preview"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="batch-start"]').attributes('disabled')).toBeDefined()
  })

  it('refuses to start when nothing can be imported', async () => {
    api.batchPreview.mockResolvedValue({ ...previewResult, importable: 0, lines: [] })
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="batch-start"]').attributes('disabled')).toBeDefined()
  })

  it('starts the batch, drops the pasted logins and follows progress to the end', async () => {
    vi.useFakeTimers()
    api.batchStart.mockResolvedValue(jobOf('running', [{ status: 'creating' }, { status: 'queued' }]))
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="batch-start"]').trigger('click')
    await flushPromises()
    expect(api.batchStart).toHaveBeenCalledTimes(1)
    expect(wrapper.find('#batch-text').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('typed-password')
    expect(wrapper.get('[data-testid="batch-progress"]').text()).toContain('"done":0,"total":2,"ok":0')

    api.batchStatus.mockResolvedValue(jobOf('running', [{ status: 'succeeded' }, { status: 'logging_in' }]))
    await vi.advanceTimersByTimeAsync(3100)
    await flushPromises()
    expect(wrapper.get('[data-testid="batch-progress"]').text()).toContain('"done":1,"total":2,"ok":1')
    expect(wrapper.emitted('updated')).toBeFalsy()

    api.batchStatus.mockResolvedValue(jobOf('finished', [{ status: 'succeeded' }, { status: 'failed', error_code: 'login_password_rejected' }]))
    await vi.advanceTimersByTimeAsync(3100)
    await flushPromises()
    const progress = wrapper.get('[data-testid="batch-progress"]').text()
    expect(progress).toContain('admin.accounts.vmManaged.batch.jobStatus.finished')
    expect(progress).toContain('"done":2,"total":2,"ok":1')
    // A slot's refusal reason is shown in plain words when one is known.
    expect(progress).toContain('admin.accounts.vmManaged.attention.password_rejected')
    expect(wrapper.emitted('updated')).toHaveLength(1)
    expect(wrapper.find('[data-testid="batch-cancel"]').exists()).toBe(false)
    // A finished batch is not polled again.
    const polls = api.batchStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(20000)
    expect(api.batchStatus.mock.calls.length).toBe(polls)
  })

  it('keeps polling through a failed status read', async () => {
    vi.useFakeTimers()
    api.batchStart.mockResolvedValue(jobOf('running', [{ status: 'creating' }]))
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="batch-start"]').trigger('click')
    await flushPromises()
    api.batchStatus.mockRejectedValueOnce(new Error('network')).mockResolvedValue(jobOf('finished', [{ status: 'succeeded' }]))
    await vi.advanceTimersByTimeAsync(3100)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3100)
    await flushPromises()
    expect(wrapper.get('[data-testid="batch-progress"]').text()).toContain('admin.accounts.vmManaged.batch.jobStatus.finished')
  })

  it('stops a running batch on request and stops polling when closed', async () => {
    vi.useFakeTimers()
    api.batchStart.mockResolvedValue(jobOf('running', [{ status: 'logging_in' }, { status: 'queued' }]))
    api.batchCancel.mockResolvedValue(jobOf('canceled', [{ status: 'succeeded' }, { status: 'canceled' }]))
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="batch-start"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="batch-cancel"]').trigger('click')
    await flushPromises()
    expect(api.batchCancel).toHaveBeenCalledWith(9)
    expect(wrapper.get('[data-testid="batch-progress"]').text()).toContain('admin.accounts.vmManaged.batch.jobStatus.canceled')
    await vi.advanceTimersByTimeAsync(20000)
    expect(api.batchStatus).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="batch-close"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('shows the reason when the check is refused and creates nothing', async () => {
    api.batchPreview.mockRejectedValue({ status: 400, message: 'A batch holds at most 20 accounts' })
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.get('[data-testid="batch-preview-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="batch-error"]').text()).toBe('A batch holds at most 20 accounts')
    expect(wrapper.find('[data-testid="batch-preview"]').exists()).toBe(false)
    expect(api.batchStart).not.toHaveBeenCalled()
  })

  it('keeps "accounts per proxy" within what VM2 can carry', async () => {
    const wrapper = mountModal()
    await fill(wrapper)
    const perProxy = wrapper.get('#batch-per-proxy')
    expect(perProxy.attributes('max')).toBe('32')
    expect(wrapper.text()).toContain('admin.accounts.vmManaged.batch.perProxyHint')
    await perProxy.setValue(33)
    expect(wrapper.get('[data-testid="batch-preview-button"]').attributes('disabled')).toBeDefined()
    await perProxy.setValue(32)
    expect(wrapper.get('[data-testid="batch-preview-button"]').attributes('disabled')).toBeUndefined()
  })

  it('forgets everything typed when it is closed and reopened', async () => {
    const wrapper = mountModal()
    await fill(wrapper)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    expect((wrapper.get('#batch-text').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.html()).not.toContain('typed-password')
  })
})
