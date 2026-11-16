import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import OpenAIPasswordLogin from '../OpenAIPasswordLogin.vue'
import { openaiPasswordLogin } from '@/api/admin/openaiPasswordLogin'

vi.mock('@/api/admin/openaiPasswordLogin', () => ({ openaiPasswordLogin: { saved: vi.fn().mockResolvedValue({ saved: null }), remember: vi.fn(), forget: vi.fn(), startSaved: vi.fn(), capabilities: vi.fn(), start: vi.fn(), status: vi.fn(), code: vi.fn(), cancel: vi.fn(), exchange: vi.fn() } }))
const state = { id: 'job-1', status: 'running' as const, stage: 'starting', created_at: 1 }
const i18n = () => createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })
describe('ordinary OpenAI password login', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.resetAllMocks(); vi.mocked(openaiPasswordLogin.saved).mockResolvedValue({ saved: null }); vi.mocked(openaiPasswordLogin.capabilities).mockResolvedValue({ available: true }); vi.mocked(openaiPasswordLogin.start).mockResolvedValue(state); vi.mocked(openaiPasswordLogin.cancel).mockResolvedValue({ ...state, status: 'canceled' }) })
  afterEach(() => vi.useRealTimers())
  it('uses the selected proxy, clears typed secrets and returns a normal grant', async () => {
    const wrapper = mount(OpenAIPasswordLogin, { props: { proxyId: 7 }, global: { plugins: [i18n()] } })
    await flushPromises()
    await wrapper.find('input[type="email"]').setValue('owner@example.test')
    const secrets = wrapper.findAll('input[type="password"]')
    await secrets[0].setValue('synthetic-password'); await secrets[1].setValue('JBSWY3DPEHPK3PXP')
    vi.mocked(openaiPasswordLogin.status).mockResolvedValue({ ...state, status: 'succeeded' })
    const grant = { token_info: { access_token: 'synthetic-access', client_id: 'codex' }, proxy_id: 7 }
    vi.mocked(openaiPasswordLogin.exchange).mockResolvedValue(grant)
    let submitted: any
    vi.mocked(openaiPasswordLogin.start).mockImplementation(async (input) => { submitted = { ...input }; return state })
    await wrapper.find('button').trigger('click'); await flushPromises()
    expect(submitted.proxy_id).toBe(7); expect(submitted.password).toBe('synthetic-password')
    expect((secrets[0].element as HTMLInputElement).value).toBe(''); expect((secrets[1].element as HTMLInputElement).value).toBe('')
    await vi.advanceTimersByTimeAsync(1500); await flushPromises()
    expect(wrapper.emitted('authorized')?.[0]).toEqual([grant])
    wrapper.unmount()
  })
  it('keeps manual code entry and cancels its owned task on leaving the form', async () => {
    const wrapper = mount(OpenAIPasswordLogin, { global: { plugins: [i18n()] } })
    await flushPromises()
    await wrapper.find('input[type="email"]').setValue('owner@example.test')
    await wrapper.findAll('input[type="password"]')[0].setValue('synthetic-password')
    await wrapper.find('input[value="code"]').setValue(true)
    vi.mocked(openaiPasswordLogin.status).mockResolvedValue({ ...state, status: 'awaiting_code' })
    vi.mocked(openaiPasswordLogin.code).mockResolvedValue(state)
    await wrapper.find('button').trigger('click'); await flushPromises()
    await vi.advanceTimersByTimeAsync(1500); await flushPromises()
    await wrapper.find('input[inputmode="numeric"]').setValue('123456')
    await wrapper.findAll('button')[0].trigger('click'); await flushPromises()
    expect(openaiPasswordLogin.code).toHaveBeenCalledWith('job-1', '123456')
    wrapper.unmount(); expect(openaiPasswordLogin.cancel).toHaveBeenCalledWith('job-1')
  })
  it('reuses saved details without returning passwords to the browser and can forget them', async () => {
    vi.mocked(openaiPasswordLogin.saved).mockResolvedValue({ saved: { email: 'owner@example.test', has_password: true, has_mfa_secret: true, mfa_mode: 'secret' } })
    vi.mocked(openaiPasswordLogin.startSaved).mockResolvedValue(state)
    const wrapper = mount(OpenAIPasswordLogin, { props: { accountId: 17, proxyId: 7, reauthorize: true }, global: { plugins: [i18n()] } })
    await flushPromises()
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    const start = wrapper.findAll('button').find(button => button.text().includes('startSaved'))!
    await start.trigger('click'); await flushPromises()
    expect(openaiPasswordLogin.startSaved).toHaveBeenCalledWith(17)
    expect(openaiPasswordLogin.start).not.toHaveBeenCalled()
    wrapper.unmount()
    const second = mount(OpenAIPasswordLogin, { props: { accountId: 17 }, global: { plugins: [i18n()] } })
    await flushPromises()
    await second.findAll('button').find(button => button.text().includes('forget'))!.trigger('click'); await flushPromises()
    expect(openaiPasswordLogin.forget).toHaveBeenCalledWith(17)
    expect(second.findAll('input[type="password"]')).toHaveLength(2)
    second.unmount()
  })

})
