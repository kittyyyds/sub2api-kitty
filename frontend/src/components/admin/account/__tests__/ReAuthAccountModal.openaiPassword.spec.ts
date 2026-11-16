import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import ReAuthAccountModal from '../ReAuthAccountModal.vue'
import { openaiPasswordLogin } from '@/api/admin/openaiPasswordLogin'
import { adminAPI } from '@/api/admin'
import type { Account } from '@/types'

const { showError, showSuccess, showWarning } = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn(), showWarning: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess, showWarning }) }))
vi.mock('@/api/admin/openaiPasswordLogin', () => ({
  openaiPasswordLogin: { saved: vi.fn().mockResolvedValue({ saved: null }), remember: vi.fn(), forget: vi.fn(), startSaved: vi.fn(), capabilities: vi.fn(), start: vi.fn(), status: vi.fn(), code: vi.fn(), cancel: vi.fn(), exchange: vi.fn() }
}))
vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { applyOAuthCredentials: vi.fn(), create: vi.fn(), update: vi.fn() } }
}))

const state = { id: 'job-1', status: 'running' as const, stage: 'starting', created_at: 1 }
const grant = {
  token_info: { access_token: 'synthetic-access', refresh_token: 'synthetic-refresh', client_id: 'codex', email: 'owner@example.test' },
  proxy_id: 7 as number | null
}
const original = (overrides: Partial<Account> = {}) => ({
  id: 17, name: 'Original account', platform: 'openai', type: 'oauth', proxy_id: 7,
  credentials: { model_mapping: { public: 'upstream' }, email: 'owner@example.test' },
  extra: { retained_setting: true }, group_ids: [3, 4], rate_multiplier: 0.15,
  ...overrides
}) as Account
const dialog = defineComponent({
  props: ['show'], template: '<div v-if="show"><slot /><footer><slot name="footer" /></footer></div>'
})
let wrappers: VueWrapper[] = []
function mountModal(account = original()) {
  const wrapper = mount(ReAuthAccountModal, {
    props: { show: true, account },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })],
      stubs: { BaseDialog: dialog, Icon: true }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}
async function startLogin(wrapper: VueWrapper) {
  await wrapper.find('input[value="openai_password"]').setValue(true)
  await flushPromises()
  const form = wrapper.find('[data-testid="openai-password-login"]')
  await form.find('input[type="email"]').setValue('owner@example.test')
  const secrets = form.findAll('input[type="password"]')
  await secrets[0].setValue('synthetic-password')
  await secrets[1].setValue('JBSWY3DPEHPK3PXP')
  await form.find('button').trigger('click')
  await flushPromises()
}
async function finishLogin() {
  await vi.advanceTimersByTimeAsync(1500)
  await flushPromises()
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

describe('existing OpenAI account password / 2FA reauthorization', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetAllMocks(); vi.mocked(openaiPasswordLogin.saved).mockResolvedValue({ saved: null })
    vi.mocked(openaiPasswordLogin.capabilities).mockResolvedValue({ available: true })
    vi.mocked(openaiPasswordLogin.start).mockResolvedValue(state)
    vi.mocked(openaiPasswordLogin.status).mockResolvedValue({ ...state, status: 'succeeded' })
    vi.mocked(openaiPasswordLogin.exchange).mockResolvedValue(grant)
    vi.mocked(openaiPasswordLogin.cancel).mockResolvedValue({ ...state, status: 'canceled' })
    vi.mocked(adminAPI.accounts.applyOAuthCredentials).mockResolvedValue(original({ status: 'active' }))
  })
  afterEach(() => {
    for (const wrapper of wrappers) wrapper.unmount()
    wrappers = []
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it.each([7, null])('logs in through the original proxy %s and updates only credentials on the original ID', async (proxy) => {
    const account = original({ proxy_id: proxy })
    const snapshot = JSON.stringify(account)
    const wrapper = mountModal(account)
    let input: unknown
    vi.mocked(openaiPasswordLogin.start).mockImplementation(async (value) => { input = { ...value }; return state })
    vi.mocked(openaiPasswordLogin.exchange).mockResolvedValue({ ...grant, proxy_id: proxy })
    await startLogin(wrapper)
    expect(input).toEqual(expect.objectContaining({ proxy_id: proxy, email: 'owner@example.test', mfa_mode: 'secret' }))
    expect(wrapper.find('footer').text()).not.toContain('admin.accounts.oauth.completeAuth')
    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.passwordLogin.reauthDescription')
    await finishLogin()
    expect(adminAPI.accounts.applyOAuthCredentials).toHaveBeenCalledTimes(1)
    expect(adminAPI.accounts.applyOAuthCredentials).toHaveBeenCalledWith(17, {
      type: 'oauth', credentials: {
        access_token: 'synthetic-access', refresh_token: 'synthetic-refresh', expires_at: undefined,
        email: 'owner@example.test', client_id: 'codex'
      }, extra: { email: 'owner@example.test' }
    })
    expect(JSON.stringify(account)).toBe(snapshot)
    expect(adminAPI.accounts.create).not.toHaveBeenCalled()
    expect(adminAPI.accounts.update).not.toHaveBeenCalled()
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('keeps a granted result for retry when saving fails without logging in twice', async () => {
    const wrapper = mountModal()
    vi.mocked(adminAPI.accounts.applyOAuthCredentials).mockRejectedValueOnce({ response: { data: { message: 'Temporary save failure' } } })
    await startLogin(wrapper)
    await finishLogin()
    expect(wrapper.text()).toContain('Temporary save failure')
    expect(wrapper.emitted('close')).toBeUndefined()
    const save = wrapper.find('[data-testid="openai-password-login"] button')
    expect(save.text()).toBe('admin.accounts.oauth.openai.passwordLogin.reauthSave')
    await save.trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.applyOAuthCredentials).toHaveBeenCalledTimes(2)
    expect(openaiPasswordLogin.start).toHaveBeenCalledTimes(1)
    expect(openaiPasswordLogin.exchange).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
  })

  it('does not write the original account when sign-in fails', async () => {
    const wrapper = mountModal()
    vi.mocked(openaiPasswordLogin.status).mockResolvedValue({ ...state, status: 'failed', message: 'Sign-in rejected' })
    await startLogin(wrapper)
    await finishLogin()
    expect(wrapper.text()).toContain('Sign-in rejected')
    expect(openaiPasswordLogin.exchange).not.toHaveBeenCalled()
    expect(adminAPI.accounts.applyOAuthCredentials).not.toHaveBeenCalled()
  })

  it('rejects a grant from a different proxy instead of changing the original account', async () => {
    const wrapper = mountModal()
    vi.mocked(openaiPasswordLogin.exchange).mockResolvedValue({ ...grant, proxy_id: null })
    await startLogin(wrapper)
    await finishLogin()
    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.passwordLogin.reauthProxyChanged')
    expect(adminAPI.accounts.applyOAuthCredentials).not.toHaveBeenCalled()
  })

  it.each(['close', 'account', 'proxy'] as const)('cancels a pending login when changing %s', async (change) => {
    const wrapper = mountModal()
    const pending = deferred<typeof grant>()
    vi.mocked(openaiPasswordLogin.exchange).mockReturnValue(pending.promise)
    await startLogin(wrapper)
    await finishLogin()
    if (change === 'close') await wrapper.setProps({ show: false })
    if (change === 'account') await wrapper.setProps({ account: original({ id: 18 }) })
    if (change === 'proxy') await wrapper.setProps({ account: original({ proxy_id: 8 }) })
    expect(openaiPasswordLogin.cancel).toHaveBeenCalledWith('job-1')
    pending.resolve(grant)
    await flushPromises()
    expect(adminAPI.accounts.applyOAuthCredentials).not.toHaveBeenCalled()
    expect(wrapper.emitted('reauthorized')).toBeUndefined()
  })

  it.each([false, true])('ignores a late save success/failure after switching accounts (failure=%s)', async (fail) => {
    const wrapper = mountModal()
    const pending = deferred<Account>()
    vi.mocked(adminAPI.accounts.applyOAuthCredentials).mockReturnValue(pending.promise)
    await startLogin(wrapper)
    await finishLogin()
    expect(adminAPI.accounts.applyOAuthCredentials).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ account: original({ id: 18 }) })
    if (fail) pending.reject(new Error('Old save failed'))
    else pending.resolve(original())
    await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('reauthorized')).toBeUndefined()
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Original account')
  })

  it('keeps manual OAuth and RT methods and hides the new method for VM-managed and other platforms', async () => {
    const wrapper = mountModal()
    expect(wrapper.find('input[value="manual"]').exists()).toBe(true)
    expect(wrapper.find('input[value="refresh_token"]').exists()).toBe(true)
    await wrapper.setProps({ account: original({ id: 18, extra: { openai_vm_managed: true } }) })
    expect(wrapper.find('input[value="openai_password"]').exists()).toBe(false)
    await wrapper.setProps({ account: original({ id: 19, platform: 'anthropic' }) })
    expect(wrapper.find('input[value="openai_password"]').exists()).toBe(false)
    expect(wrapper.find('input[value="cookie"]').exists()).toBe(true)
  })
  it('attaches remembered login details only after saving the original account grant', async () => {
    vi.mocked(openaiPasswordLogin.exchange).mockResolvedValue({ ...grant, remember_job_id: 'job-1' })
    const wrapper = mountModal()
    await startLogin(wrapper); await finishLogin()
    expect(openaiPasswordLogin.remember).toHaveBeenCalledWith(17, 'job-1')
    expect(vi.mocked(adminAPI.accounts.applyOAuthCredentials).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(openaiPasswordLogin.remember).mock.invocationCallOrder[0])
    expect(adminAPI.accounts.create).not.toHaveBeenCalled()
  })
  it('reports remember failure without repeating successful authorization', async () => {
    vi.mocked(openaiPasswordLogin.exchange).mockResolvedValue({ ...grant, remember_job_id: 'job-1' })
    vi.mocked(openaiPasswordLogin.remember).mockRejectedValue(new Error('disk unavailable'))
    const wrapper = mountModal(); await startLogin(wrapper); await finishLogin()
    expect(showWarning).toHaveBeenCalledWith('admin.accounts.oauth.openai.passwordLogin.rememberFailed')
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
    expect(adminAPI.accounts.applyOAuthCredentials).toHaveBeenCalledTimes(1)
  })

})
