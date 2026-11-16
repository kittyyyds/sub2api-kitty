import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import AccountIntelligenceHistory from '../AccountIntelligenceHistory.vue'
import type { AccountIntelligenceTest } from '@/api/admin/accountIntelligence'
import accounts from '@/i18n/locales/zh/admin/accounts'

const item = (minute: number, overrides: Partial<AccountIntelligenceTest> = {}): AccountIntelligenceTest => ({
  id: `test-${minute}`, request_id: `request-${minute}`, account_id: 27, source: 'scheduled',
  kind: 'candy', model: 'gpt-6-astra', reasoning_effort: 'low', prompt: '固定糖果题',
  expected_answer: '21', scoring_version: 'candy-21-v1', status: 'correct', response: '21',
  latency_ms: 200, created_at: new Date(Date.UTC(2026, 10, 16, 12, minute)).toISOString(),
  finished_at: new Date(Date.UTC(2026, 10, 16, 12, minute, 20)).toISOString(), ...overrides,
})
const render = (items: AccountIntelligenceTest[]) => mount(AccountIntelligenceHistory, {
  props: { items },
  global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { admin: accounts } } })] },
})

describe('account intelligence status summary', () => {
  it('shows the latest result and 20 real records in chronological order without altering the input', async () => {
    const records = Array.from({ length: 21 }, (_, i) => item(i, i === 0 ? { status: 'incorrect', response: '29' } : {}))
    const original = records.map(v => v.id)
    const w = render(records)
    expect(w.get('[data-testid=intelligence-result]').text()).toBe('答案测试：匹配 21')
    expect(w.get('[data-testid=intelligence-match-rate]').text()).toContain('匹配率 100.0% · 20 次')
    const bars = w.findAll('[data-testid=intelligence-history-bar]')
    expect(bars).toHaveLength(20)
    expect(bars[0].attributes('title')).toContain('21')
    expect(bars[0].attributes('title')).not.toContain('29')
    expect(w.get('[data-testid=intelligence-time]').attributes('datetime')).toBe(records[20].finished_at)
    expect(w.get('[data-testid=intelligence-model]').text()).toBe('gpt-6-astra')
    expect(records.map(v => v.id)).toEqual(original)
    await bars[0].trigger('click')
    await w.get('[data-testid=intelligence-result]').trigger('click')
    expect(w.emitted('open')).toHaveLength(2)
    w.unmount()
  })

  it('calculates matches only for comparable scored answers, not errors, running tests or other models and questions', () => {
    const w = render([
      item(12), item(11), item(10, { status: 'incorrect', response: '29' }),
      item(9, { status: 'error', error: 'upstream unavailable' }),
      item(8, { status: 'running', response: '', finished_at: undefined }),
      item(7, { kind: 'custom', status: 'completed', expected_answer: undefined }),
      item(6, { model: 'other-model', status: 'incorrect' }),
      item(5, { reasoning_effort: 'high', status: 'incorrect' }),
      item(4, { prompt: '另一道题目', status: 'incorrect' }),
      item(3, { scoring_version: 'other-version', status: 'incorrect' }),
      item(2, { expected_answer: '22', status: 'incorrect' }),
    ])
    expect(w.get('[data-testid=intelligence-match-rate]').text()).toContain('匹配率 66.7% · 3 次')
    const bars = w.findAll('[data-testid=intelligence-history-bar]')
    expect(bars.filter(v => v.classes().includes('bg-red-500'))).toHaveLength(1)
    expect(bars.filter(v => v.classes().includes('bg-blue-400'))).toHaveLength(1)
    expect(bars.filter(v => v.classes().includes('bg-slate-400'))).toHaveLength(1)
    expect(w.get('[data-testid=intelligence-match-rate]').attributes('title')).toContain('同题、同模型、同思考强度')
    w.unmount()
  })

  it.each([
    ['incorrect', '答案测试：未匹配 21', 'text-amber-700', 'bg-amber-400'],
    ['error', '答案测试：请求异常', 'text-red-600', 'bg-red-500'],
    ['running', '答案测试：检测中', 'text-blue-600', 'bg-blue-400'],
  ] as const)('keeps the latest %s outcome visible', (status, text, textColor, barColor) => {
    const w = render([item(1), item(2, { status, response: status === 'incorrect' ? '29' : '', error: status === 'error' ? 'upstream unavailable' : undefined })])
    expect(w.get('[data-testid=intelligence-result]').text()).toBe(text)
    expect(w.get('[data-testid=intelligence-result]').classes()).toContain(textColor)
    expect(w.findAll('[data-testid=intelligence-history-bar]')[1].classes()).toContain(barColor)
    w.unmount()
  })

  it('shows unscored and unavailable rates honestly rather than displaying a false zero', () => {
    const unscored = render([item(1), item(2, { kind: 'custom', status: 'completed', expected_answer: undefined })])
    expect(unscored.get('[data-testid=intelligence-match-rate]').text()).toBe('自定义题目 · 未评分')
    expect(unscored.text()).not.toContain('100.0%')
    unscored.unmount()
    const error = render([item(2, { status: 'error', error: 'unavailable', response: '' })])
    expect(error.get('[data-testid=intelligence-match-rate]').text()).toBe('匹配率 — · 暂无有效评分')
    error.unmount()
  })

  it('shows the original question and answer in the hover text without rendering their HTML', async () => {
    const w = render([item(1, { prompt: '<img src=x onerror=alert(1)>', response: '<b>21</b>' })])
    const bar = w.get('[data-testid=intelligence-history-bar]')
    expect(bar.attributes('title')).toContain('题目：<img src=x onerror=alert(1)>')
    expect(bar.attributes('title')).toContain('回答 / 错误：<b>21</b>')
    expect(bar.attributes('aria-label')).toBe(bar.attributes('title'))
    expect(w.find('img').exists()).toBe(false)
    expect(w.find('b').exists()).toBe(false)
    await w.setProps({ items: [item(2, { status: 'incorrect', response: '29' })] })
    expect(w.get('[data-testid=intelligence-result]').text()).toBe('答案测试：未匹配 21')
    w.unmount()
  })

  it('does not invent a result or empty history bars before any test has run', () => {
    const w = render([])
    expect(w.find('[data-testid=account-intelligence-summary]').exists()).toBe(false)
    expect(w.findAll('[data-testid=intelligence-history-bar]')).toHaveLength(0)
    w.unmount()
  })
})
