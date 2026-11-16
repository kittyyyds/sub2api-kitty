import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import AccountIntelligenceModal from '../AccountIntelligenceModal.vue'
import { accountIntelligence } from '@/api/admin/accountIntelligence'
import type { AccountListItem } from '@/types'
vi.mock('@/api/admin/accountIntelligence', async () => ({ ...(await vi.importActual<any>('@/api/admin/accountIntelligence')), accountIntelligence: { start: vi.fn(), history: vi.fn(), recent: vi.fn() } }))
const account = { id: 27, name: 'Fixed account', platform: 'openai', credentials: { model_mapping: { 'gpt-6-astra': 'private-model' } } } as unknown as AccountListItem
const item = { id: 'test-1', request_id: 'request-1', account_id: 27, kind: 'candy' as const, model: 'gpt-6-astra', reasoning_effort: '', prompt: 'historical question', expected_answer: '21', scoring_version: 'candy-21-v1', status: 'incorrect' as const, response: '29', latency_ms: 100, created_at: '2026-11-16T12:00:00Z' }
const options = { props: { account }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } }, plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })] } }
describe('manual account intelligence', () => {
 beforeEach(() => { vi.useFakeTimers(); vi.resetAllMocks(); vi.mocked(accountIntelligence.history).mockResolvedValue([item]); vi.mocked(accountIntelligence.start).mockResolvedValue({ ...item, status: 'running' }) })
 afterEach(() => { vi.useRealTimers() })
 it('shows the saved question and answer as text and starts only the selected account', async () => {
  const w = mount(AccountIntelligenceModal, options); await flushPromises()
  expect(w.text()).toContain('historical question'); expect(w.text()).toContain('29')
  await w.find('button').trigger('click'); await flushPromises()
  expect(accountIntelligence.start).toHaveBeenCalledWith(27, expect.objectContaining({ kind: 'candy', model: 'gpt-6-astra', request_id: expect.any(String) }))
  expect(w.find('button').attributes('disabled')).toBeDefined();w.unmount()
 })
 it('reuses the request identifier on an uncertain network failure', async () => {
  vi.mocked(accountIntelligence.start).mockRejectedValueOnce(new Error('network'))
  const w = mount(AccountIntelligenceModal, options); await flushPromises(); await w.find('button').trigger('click'); await flushPromises(); await w.find('button').trigger('click'); await flushPromises()
  const calls = vi.mocked(accountIntelligence.start).mock.calls
  expect(calls[0][1].request_id).toBe(calls[1][1].request_id);w.unmount()
 })
})
