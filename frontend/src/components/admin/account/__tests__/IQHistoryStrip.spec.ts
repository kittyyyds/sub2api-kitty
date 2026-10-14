import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import IQHistoryStrip from '../IQHistoryStrip.vue'
import type { IQResult, IQSummary } from '@/api/admin/iqInterval'

const result = (id: number, verdict: IQResult['verdict'], answer = '', error = ''): IQResult => ({ id, verdict, answer, error, model: 'fixture', reasoning: 'medium', finished_at: '2026-10-15T00:00:00Z' })
const summary = (results: IQResult[], extra: Partial<IQSummary> = {}): IQSummary => ({ account_id: 1, plan_id: 1, enabled: true, interval_seconds: 60, model: 'fixture', reasoning: 'medium', running_until: null, next_run_at: null, results, ...extra })
let wrapper: ReturnType<typeof mount>
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-15T00:01:00Z')) })
afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })
it('keeps 20 chronological cells and gives each record its own answer or cause', () => {
  wrapper = mount(IQHistoryStrip, { props: { summary: summary([result(3, 'error', '', '账号业务并发已满，测试等待空闲容量'), result(2, 'other', '29'), result(1, 'correct', '21')]) } })
  const cells = wrapper.findAll('[data-verdict]')
  expect(cells).toHaveLength(20)
  expect(cells.slice(-3).map(c => c.attributes('data-verdict'))).toEqual(['correct','other','error'])
  expect(cells[0].attributes('title')).toBe('未测试')
  expect(cells[17].attributes('title')).toContain('答案：21')
  expect(cells[18].attributes('title')).toContain('答案：29')
  expect(cells[19].attributes('title')).toContain('报错：账号业务并发已满，测试等待空闲容量')
  expect(wrapper.get('[data-testid="iq-headline"]').text()).toBe('答案测试：未完成')
  expect(wrapper.get('[data-testid="iq-error"]').text()).toContain('等待空闲容量')
  expect(wrapper.text()).toContain('1/3')
})
it('shows the newest answer and changes to running without overwriting history', async () => {
  wrapper = mount(IQHistoryStrip, { props: { summary: summary([result(1, 'correct', '21')]) } })
  expect(wrapper.get('[data-testid="iq-headline"]').text()).toBe('答案测试：匹配21')
  await wrapper.setProps({ summary: summary([result(1, 'correct', '21')], { running_until: '2026-10-15T00:02:00Z' }) })
  expect(wrapper.get('[data-testid="iq-headline"]').text()).toBe('答案测试：测试中')
  expect(wrapper.text()).toContain('上次：匹配21')
  await wrapper.setProps({ summary: summary([result(2, 'other', '29'), result(1, 'correct', '21')]) })
  expect(wrapper.get('[data-testid="iq-headline"]').text()).toContain('答案：29')
})
it('updates the completion countdown every second and does not invent a running test', async () => {
  wrapper = mount(IQHistoryStrip, { props: { summary: summary([], { next_run_at: '2026-10-15T00:01:02Z' }) } })
  expect(wrapper.text()).toContain('完成后等待 2 秒')
  await vi.advanceTimersByTimeAsync(1000)
  expect(wrapper.text()).toContain('完成后等待 1 秒')
  await vi.advanceTimersByTimeAsync(1000)
  expect(wrapper.text()).toContain('等待 / 排队')
  expect(wrapper.get('[data-testid="iq-headline"]').text()).toBe('答案测试：未测试')
})
it('does not present a stale successful answer as current after refresh failure', () => {
  wrapper = mount(IQHistoryStrip, { props: { error: true, summary: summary([result(1, 'correct', '21')]) } })
  expect(wrapper.get('[data-testid="iq-headline"]').text()).toBe('答案测试：状态读取失败')
  expect(wrapper.text()).toContain('记录读取失败')
  expect(wrapper.text()).not.toContain('1/1')
})
it('escapes unknown model answers instead of interpreting HTML', () => {
  wrapper = mount(IQHistoryStrip, { props: { summary: summary([result(1, 'other', '<img src=x onerror=alert(1)>')]) } })
  expect(wrapper.find('img').exists()).toBe(false)
  expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
})
