import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import CandyTestView from '../CandyTestView.vue'
import CandyResult from '@/components/candy-test/CandyResult.vue'
import { candyQuestion } from '@/components/candy-test/preset'
import { candyTestsAPI, type CandyCard, type CandyRun } from '@/api/candyTests'

vi.mock('@/api/candyTests', async () => {
  const actual = await vi.importActual('@/api/candyTests')
  return { ...actual, candyTestsAPI: { list: vi.fn(), keys: vi.fn(), save: vi.fn(), run: vi.fn(), history: vi.fn() } }
})

const run: CandyRun = { id: 1, test_id: 1, status: 'passed', question: '测试题', model: 'test-model', reasoning_effort: 'high', output: '最终答案：8', answer: '8', input_tokens: 10, output_tokens: 15, cache_read_tokens: null, latency_ms: 1200, created_at: '2026-09-14T00:00:00Z', finished_at: '2026-09-14T00:00:01Z' }
const card: CandyCard = { id: 1, history_count: 1, label: '测试分组', model: 'test-model', reasoning_effort: 'high', enabled: false, interval_minutes: 25, next_run_at: null, history: [run] }
const stubs = { AppLayout: { template: '<div><slot /></div>' }, BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' } }
const setup = (admin = false) => mount(CandyTestView, { props: { admin }, global: { stubs } })

describe('group candy test', () => {
  beforeEach(() => {
    vi.useFakeTimers(); vi.resetAllMocks()
    vi.mocked(candyTestsAPI.list).mockResolvedValue({ cards: [card], updated_at: run.created_at })
    vi.mocked(candyTestsAPI.keys).mockResolvedValue([])
    vi.mocked(candyTestsAPI.history).mockResolvedValue([run])
  })
  afterEach(() => { vi.useRealTimers() })

  it('page load, manual refresh and polling never start a paid test', async () => {
    const wrapper = setup(); await flushPromises()
    expect(wrapper.text()).toContain('答案通过')
    const refresh = wrapper.findAll('button').find(b => b.text() === '刷新结果')!
    await refresh.trigger('click'); await flushPromises()
    await vi.advanceTimersByTimeAsync(15000)
    expect(candyTestsAPI.run).not.toHaveBeenCalled()
    expect(candyTestsAPI.keys).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('测试一次（计费）')
    const calls = vi.mocked(candyTestsAPI.list).mock.calls.length
    wrapper.unmount(); await vi.advanceTimersByTimeAsync(30000)
    expect(candyTestsAPI.list).toHaveBeenCalledTimes(calls)
  })

  it('only the admin run button starts a test', async () => {
    vi.mocked(candyTestsAPI.run).mockResolvedValue({ run_id: 2 })
    const wrapper = setup(true); await flushPromises()
    const start = wrapper.findAll('button').find(b => b.text() === '测试一次（计费）')!
    await start.trigger('click'); await flushPromises()
    expect(candyTestsAPI.run).toHaveBeenCalledTimes(1)
    expect(candyTestsAPI.run).toHaveBeenCalledWith(1)
    wrapper.unmount()
  })

  it('ticks down locally and never creates a request when the deadline arrives', async () => {
    vi.setSystemTime(new Date('2026-09-14T00:00:00Z'))
    vi.mocked(candyTestsAPI.list).mockResolvedValue({ cards: [{ ...card, enabled: true, next_run_at: '2026-09-14T00:00:02Z' }], updated_at: run.created_at })
    const wrapper = setup(); await flushPromises()
    expect(wrapper.text()).toContain('距下次测试 00 分 02 秒')
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.text()).toContain('距下次测试 00 分 01 秒')
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.text()).toContain('已到测试时间，等待开始')
    expect(candyTestsAPI.run).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('测试日期：')
    wrapper.unmount()
  })

  it('prefills the user question without enabling paid tests automatically', async () => {
    const wrapper = setup(true); await flushPromises()
    await wrapper.findAll('button').find(b => b.text().includes('添加测试分组'))!.trigger('click')
    const form = wrapper.find('form')
    expect(form.findAll('input[type="checkbox"]').every(c => !(c.element as HTMLInputElement).checked)).toBe(true)
    expect((form.find('textarea').element as HTMLTextAreaElement).value).toBe(candyQuestion)
    expect(candyQuestion).not.toContain('21')
    expect(candyQuestion).not.toContain('29')
    expect(candyTestsAPI.run).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('loads history only when requested', async () => {
    const wrapper = setup(); await flushPromises()
    expect(candyTestsAPI.history).not.toHaveBeenCalled()
    await wrapper.find('button.history-button').trigger('click'); await flushPromises()
    expect(candyTestsAPI.history).toHaveBeenCalledTimes(1)
    expect(candyTestsAPI.history).toHaveBeenCalledWith(1, false)
    expect(candyTestsAPI.run).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('renders upstream HTML as plain text and distinguishes missing usage from zero', () => {
    const wrapper = mount(CandyResult, { props: { run: { ...run, output: '<img src=x onerror=alert(1)>', input_tokens: 0 } } })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.text()).toContain('输入 0')
    expect(wrapper.text()).toContain('缓存命中 未返回')
  })
})
