import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { bpsBatchImportAPI, parseBPSImportEntries, type BPSImportStatus } from '@/api/admin/bpsBatchImport'
import BPSBatchImportModal from '../BPSBatchImportModal.vue'

vi.mock('vue-i18n', async () => ({ ...await vi.importActual('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('../ModelWhitelistSelector.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), post: vi.fn() } }))
const status = (state = 'running'): BPSImportStatus => ({ id: 'batch-1', request_id: 'request-1', status: state as BPSImportStatus['status'], rows: [{ email: 'fixture@example.com', status: 'queued', stage: 'queued' }], group_ids: [123], proxy_id: null, workspace: '', concurrency: 3, priority: 50, models: ['gpt-6-astra'], created_at: '2026-11-01T00:00:00Z', expires_at: '2026-11-01T02:00:00Z' })
const wrappers: ReturnType<typeof mount>[] = []
function dialog() {
  const wrapper = mount(BPSBatchImportModal, {
    props: { show: true, groups: [{ id: 123, name: 'BPS', platform: 'openai', status: 'active' }] as never, proxies: [] },
    global: { stubs: { BaseDialog: { template: '<div><slot/><slot name="footer"/></div>' }, ModelWhitelistSelector: true } }
  }); wrappers.push(wrapper); return wrapper
}
beforeEach(() => { vi.useFakeTimers(); vi.spyOn(bpsBatchImportAPI, 'list').mockResolvedValue([]) })
afterEach(() => { for (const w of wrappers.splice(0)) w.unmount(); vi.restoreAllMocks(); vi.useRealTimers() })

describe('BPS batch input', () => {
  it('accepts three/four hyphens, optional TOTP and tab-separated passwords without trimming passwords', () => {
    const entries = parseBPSImportEntries('USER@example.com--- padded ---jbswy3dpehpk3pxp\nsecond@example.com----secret\nthird@example.com\tsecret---hyphens\t')
    expect(entries).toEqual([{ email: 'user@example.com', password: ' padded ', totp_secret: 'JBSWY3DPEHPK3PXP' }, { email: 'second@example.com', password: 'secret', totp_secret: '' }, { email: 'third@example.com', password: 'secret---hyphens', totp_secret: '' }])
  })
  it('rejects duplicates, ambiguous fields, six-digit codes and oversized batches without echoing secrets', () => {
    for (const raw of ['x@example.com---secret---123456', 'x@example.com---secret\nX@example.com---other', 'x@example.com---s---extra---fields', Array.from({ length: 21 }, (_, i) => `x${i}@example.com---secret`).join('\n')]) {
      expect(() => parseBPSImportEntries(raw)).toThrow(/^(line:\d+|count)$/)
    }
  })
})
describe('BPS batch dialog', () => {
  it('restores server progress and closing stops polling without canceling the background job', async () => {
    vi.mocked(bpsBatchImportAPI.list).mockResolvedValue([status()])
    const cancel = vi.spyOn(bpsBatchImportAPI, 'cancel')
    const w = dialog(); await flushPromises(); expect(w.text()).toContain('fixture@example.com')
    await w.setProps({ show: false }); await vi.advanceTimersByTimeAsync(6000)
    expect(bpsBatchImportAPI.list).toHaveBeenCalledTimes(1); expect(cancel).not.toHaveBeenCalled()
    await w.setProps({ show: true }); await flushPromises(); expect(bpsBatchImportAPI.list).toHaveBeenCalledTimes(2)
  })
  it('retries an uncertain submission with the same request ID and clears input after confirmation', async () => {
    const start = vi.spyOn(bpsBatchImportAPI, 'start').mockRejectedValueOnce(new Error('network'))
    start.mockImplementationOnce(async input => ({ ...status(), request_id: input.request_id }))
    const w = dialog(); await flushPromises()
    await w.get('input[type="checkbox"]').setValue(true); await w.get('textarea').setValue('fixture@example.com---synthetic-password')
    await w.findAll('button').find(b => b.text().endsWith('.start'))!.trigger('click'); await flushPromises()
    expect(w.text()).toContain('openaiAuth.batchImport.uncertain')
    await w.findAll('button').find(b => b.text().endsWith('.retry'))!.trigger('click'); await flushPromises()
    expect(start).toHaveBeenCalledTimes(2); expect(start.mock.calls[0][0].request_id).toBe(start.mock.calls[1][0].request_id)
    expect(w.find('textarea').exists()).toBe(false); expect(w.text()).not.toContain('synthetic-password')
  })
  it('does not allow starting another batch when initial status retrieval failed', async () => {
    vi.mocked(bpsBatchImportAPI.list).mockRejectedValue(new Error('offline'))
    const w = dialog(); await flushPromises()
    expect(w.findAll('button').find(b => b.text().endsWith('.start'))!.attributes('disabled')).toBeDefined()
  })
  it('reports expired server state without leaving an uncancelable running dialog', async () => {
    vi.mocked(bpsBatchImportAPI.list).mockResolvedValueOnce([status()]).mockResolvedValue([])
    const w = dialog(); await flushPromises(); await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(w.text()).toContain('openaiAuth.batchImport.expired')
    expect(w.text()).toContain('openaiAuth.batchImport.state.unavailable')
    await w.findAll('button').find(b => b.text().endsWith('.newBatch'))!.trigger('click')
    expect(w.find('textarea').exists()).toBe(true)
  })
})
