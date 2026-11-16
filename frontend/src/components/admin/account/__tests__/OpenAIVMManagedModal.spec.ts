import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OpenAIVMManagedModal from '../OpenAIVMManagedModal.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import type { Account, Proxy } from '@/types'
import type { OpenAIVMManagedView } from '@/api/admin/openaiVmManaged'

const api = vi.hoisted(() => ({
  createAccount: vi.fn(), getStatus: vi.fn(), retrySlot: vi.fn(), deleteAccount: vi.fn(), changeProxy: vi.fn(),
  setSchedulable: vi.fn(), refreshUsage: vi.fn(), oauthUrl: vi.fn(), oauthCode: vi.fn(), passwordStatus: vi.fn(),
  passwordLogin: vi.fn(), passwordCode: vi.fn(), passwordCancel: vi.fn(), forgetSavedLogin: vi.fn(),
  importAccountFile: vi.fn(), refreshToken: vi.fn(), useResetCredit: vi.fn(), powerOn: vi.fn(), powerOff: vi.fn(),
  recover: vi.fn(), exitProbe: vi.fn(), diagnose: vi.fn()
}))
vi.mock('@/api/admin/openaiVmManaged', async () => ({
  ...await vi.importActual<typeof import('@/api/admin/openaiVmManaged')>('@/api/admin/openaiVmManaged'),
  openaiVmManagedAPI: api
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  // Keys are returned as-is; keys under slotError/attention count as "known" only for the listed codes.
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key),
    te: (key: string) => ['account_busy', 'password_rejected', 'running', 'awaiting_code', 'succeeded', 'failed'].some(code => key.endsWith(`.${code}`))
  })
}))

const account = { id: 42, name: 'managed', platform: 'openai', type: 'oauth', parent_account_id: null, schedulable: true, extra: { openai_vm_managed: true } } as unknown as Account
const proxies = [
  { id: 1, name: 'exit-one', protocol: 'socks5', host: 'one.example', port: 1080, status: 'active' },
  { id: 2, name: 'exit-two', protocol: 'socks5h', host: 'two.example', port: 1080, status: 'active' },
  { id: 3, name: 'http-proxy', protocol: 'http', host: 'http.example', port: 8080, status: 'active' },
  { id: 4, name: 'off', protocol: 'socks5', host: 'off.example', port: 1080, status: 'inactive' }
] as unknown as Proxy[]

function view(overrides: Partial<OpenAIVMManagedView> = {}): OpenAIVMManagedView {
  return {
    account_id: 42, status: 'available', binding_status: 'ready', runtime_type: 'kvm', slot_id: 's2a-abcdefabcdef-42', proxy_id: 1,
    activated_at: '2026-11-12T08:00:00Z', synced_at: '2026-11-12T09:00:00Z',
    snapshot: { email: 'demo@example.test', plan_type: 'plus', has_token: true, reset_credits: { available_count: 2 }, usage: { windows: [{ id: '5h', used_percent: 12.34, reset_at: '2026-11-12T15:00:00Z', reset_after_seconds: null, window_minutes: 300 }, { id: '7d', used_percent: null, reset_at: null, reset_after_seconds: null, window_minutes: 10080 }] } },
    ...overrides
  }
}

function mountModal(value: Account = account) {
  return mount(OpenAIVMManagedModal, {
    props: { show: true, account: value, proxies },
    global: { stubs: {
      BaseDialog: { props: ['show', 'showCloseButton', 'closeOnEscape'], emits: ['close'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      TotpStepUpDialog: { props: ['controller'], template: '<div />' },
      Icon: true
    } }
  })
}

const stepUpRequired = { status: 403, code: 'STEP_UP_REQUIRED', reason: 'STEP_UP_REQUIRED', message: 'step up' }

describe('VM2-managed account panel', () => {
  beforeEach(() => {
    Object.values(api).forEach(fn => fn.mockReset())
    api.getStatus.mockResolvedValue(view())
    api.passwordStatus.mockResolvedValue({ available: true, job: null, saved: null, email: 'demo@example.test' })
  })
  afterEach(() => { vi.useRealTimers() })

  it('shows the slot status, identity and usage without any credential field in the DOM', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(api.getStatus).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-testid="managed-status"]').text()).toBe('admin.accounts.vmManaged.status.available')
    expect(wrapper.text()).toContain('demo@example.test')
    expect(wrapper.text()).toContain('plus')
    const usage = wrapper.get('[data-testid="managed-usage"]').text()
    expect(usage).toContain('"percent":12.3')
    expect(usage).not.toContain('7d')
    expect(wrapper.get('[data-testid="managed-reset-count"]').text()).toContain('"count":2')
    // Nothing offers to read a saved password back.
    expect(wrapper.html()).not.toMatch(/reveal/i)
    // Only usable SOCKS5 proxies can be chosen.
    const options = wrapper.findAll('#managed-proxy option').map(option => option.text())
    expect(options.some(text => text.includes('exit-one'))).toBe(true)
    expect(options.some(text => text.includes('exit-two'))).toBe(true)
    expect(options.some(text => text.includes('http-proxy'))).toBe(false)
    expect(options.some(text => text.includes('off'))).toBe(false)
  })

  it('offers only login to a slot that nobody has logged in to', async () => {
    api.getStatus.mockResolvedValue(view({ status: 'awaiting_login', binding_status: 'awaiting_login', activated_at: null, snapshot: { has_token: false } }))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-status"]').text()).toBe('admin.accounts.vmManaged.status.awaiting_login')
    expect(wrapper.find('[data-testid="managed-password-submit"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="managed-usage-refresh"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="managed-schedulable"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="managed-token-refresh"]').attributes('disabled')).toBeDefined()
  })

  it('sends the login to the slot once and forgets what was typed', async () => {
    api.passwordLogin.mockResolvedValue({ job: { id: 'job-1', status: 'running' } })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('#managed-email').setValue(' owner@example.test ')
    await wrapper.get('#managed-password').setValue('typed-password')
    await wrapper.get('#managed-mfa').setValue(' JBSWY3DPEHPK3PXP ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.passwordLogin).toHaveBeenCalledTimes(1)
    expect(api.passwordLogin).toHaveBeenCalledWith(42, { email: 'owner@example.test', password: 'typed-password', mfa_mode: 'secret', mfa_secret: 'JBSWY3DPEHPK3PXP' })
    expect((wrapper.get('#managed-password').element as HTMLInputElement).value).toBe('')
    expect((wrapper.get('#managed-mfa').element as HTMLInputElement).value).toBe('')
    expect(wrapper.html()).not.toContain('typed-password')
    expect(wrapper.html()).not.toContain('JBSWY3DPEHPK3PXP')
    expect(wrapper.get('[data-testid="managed-job"]').text()).toContain('admin.accounts.vmManaged.job.running')
    // While a login runs, a second one cannot be started.
    expect(wrapper.get('[data-testid="managed-password-submit"]').attributes('disabled')).toBeDefined()
  })

  it('logs in without a 2FA secret in code mode and takes the code afterwards', async () => {
    api.passwordLogin.mockResolvedValue({ job: { id: 'job-2', status: 'awaiting_code' } })
    api.passwordCode.mockResolvedValue({ job: { id: 'job-2', status: 'running' } })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('#managed-email').setValue('owner@example.test')
    await wrapper.get('#managed-password').setValue('typed-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.passwordLogin).toHaveBeenCalledWith(42, { email: 'owner@example.test', password: 'typed-password', mfa_mode: 'code' })
    await wrapper.get('#managed-code').setValue(' 123456 ')
    await wrapper.get('[data-testid="managed-code-submit"]').trigger('click')
    await flushPromises()
    expect(api.passwordCode).toHaveBeenCalledWith(42, 'job-2', '123456')
    expect(wrapper.find('#managed-code').exists()).toBe(false)
  })

  it('re-logs in with the saved login without sending anything typed', async () => {
    api.passwordStatus.mockResolvedValue({ available: true, job: null, saved: { email: 'demo@example.test', has_password: true, has_mfa_secret: true } })
    api.passwordLogin.mockResolvedValue({ job: { id: 'job-3', status: 'running' } })
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-saved"]').text()).toContain('savedWithMfa')
    await wrapper.get('#managed-password').setValue('must-not-be-sent')
    await wrapper.get('[data-testid="managed-relogin-saved"]').trigger('click')
    await flushPromises()
    expect(api.passwordLogin).toHaveBeenCalledWith(42, { use_saved: true })
    expect((wrapper.get('#managed-password').element as HTMLInputElement).value).toBe('')
  })

  it('follows a running login until it ends and then refreshes the status', async () => {
    vi.useFakeTimers()
    api.passwordLogin.mockResolvedValue({ job: { id: 'job-4', status: 'running' } })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('#managed-email').setValue('owner@example.test')
    await wrapper.get('#managed-password').setValue('typed-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    api.passwordStatus.mockResolvedValue({ available: true, job: { id: 'job-4', status: 'succeeded' }, saved: null })
    const statusCalls = api.getStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(2600)
    await flushPromises()
    expect(api.getStatus.mock.calls.length).toBe(statusCalls + 1)
    expect(wrapper.get('[data-testid="managed-notice"]').text()).toBe('admin.accounts.vmManaged.loginSucceeded')
    expect(wrapper.emitted('updated')).toBeTruthy()
    // A finished login is not polled again.
    const polls = api.passwordStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.passwordStatus.mock.calls.length).toBe(polls)
  })

  it('stops following a login when the panel closes', async () => {
    vi.useFakeTimers()
    api.passwordStatus.mockResolvedValue({ available: true, job: { id: 'job-5', status: 'running' }, saved: null })
    const wrapper = mountModal()
    await flushPromises()
    const polls = api.passwordStatus.mock.calls.length
    await wrapper.setProps({ show: false })
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.passwordStatus.mock.calls.length).toBe(polls)
  })

  it('accepts either the redirected address or a bare code for OAuth', async () => {
    api.oauthUrl.mockResolvedValue({ auth_url: 'https://auth.example/authorize?x=1', session_id: 'sess-1' })
    api.oauthCode.mockResolvedValue({})
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-method-oauth"]').trigger('click')
    await wrapper.get('[data-testid="managed-oauth-url"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('[data-testid="managed-oauth-link"]').element as HTMLTextAreaElement).value).toBe('https://auth.example/authorize?x=1')
    await wrapper.get('#managed-oauth-callback').setValue(' http://localhost:1455/auth/callback?code=abc&state=s ')
    await wrapper.get('[data-testid="managed-oauth-submit"]').trigger('click')
    await flushPromises()
    expect(api.oauthCode).toHaveBeenCalledWith(42, { session_id: 'sess-1', callback_url: 'http://localhost:1455/auth/callback?code=abc&state=s' })
    expect(wrapper.find('[data-testid="managed-oauth-link"]').exists()).toBe(false)

    await wrapper.get('[data-testid="managed-oauth-url"]').trigger('click')
    await flushPromises()
    await wrapper.get('#managed-oauth-callback').setValue('bare-code')
    await wrapper.get('[data-testid="managed-oauth-submit"]').trigger('click')
    await flushPromises()
    expect(api.oauthCode).toHaveBeenLastCalledWith(42, { session_id: 'sess-1', code: 'bare-code' })
  })

  it('imports an account file and clears the pasted content', async () => {
    api.importAccountFile.mockResolvedValue({})
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-method-file"]').trigger('click')
    await wrapper.get('#managed-file').setValue(' {"tokens":{"refresh_token":"pasted-refresh"}} ')
    await wrapper.get('[data-testid="managed-file-submit"]').trigger('click')
    await flushPromises()
    expect(api.importAccountFile).toHaveBeenCalledWith(42, '{"tokens":{"refresh_token":"pasted-refresh"}}')
    expect((wrapper.get('#managed-file').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.html()).not.toContain('pasted-refresh')
  })

  it('asks before spending a reset credit and says it cannot be refunded', async () => {
    api.useResetCredit.mockResolvedValue({})
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="managed-reset-confirm"]').exists()).toBe(false)
    await wrapper.get('[data-testid="managed-reset"]').trigger('click')
    expect(api.useResetCredit).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.vmManaged.resetConfirm')
    await wrapper.get('[data-testid="managed-reset-confirm"]').trigger('click')
    await flushPromises()
    expect(api.useResetCredit).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="managed-reset-confirm"]').exists()).toBe(false)
  })

  it('hides the reset button when there is no credit', async () => {
    api.getStatus.mockResolvedValue(view({ snapshot: { reset_credits: { available_count: 0 } } }))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="managed-reset"]').exists()).toBe(false)
  })

  it('changes the proxy only to a different one and shows the slot refusal in plain words', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-proxy-current"]').text()).toContain('exit-one')
    expect(wrapper.get('[data-testid="managed-proxy-apply"]').attributes('disabled')).toBeDefined()
    await wrapper.get('#managed-proxy').setValue(2)
    api.changeProxy.mockRejectedValueOnce({ status: 409, message: 'The slot refused this operation: account_busy', metadata: { code: 'account_busy' } })
    await wrapper.get('[data-testid="managed-proxy-apply"]').trigger('click')
    await flushPromises()
    expect(api.changeProxy).toHaveBeenCalledWith(42, 2)
    expect(wrapper.get('[data-testid="managed-error"]').text()).toBe('admin.accounts.vmManaged.slotError.account_busy')
    api.changeProxy.mockResolvedValueOnce(view({ proxy_id: 2 }))
    await wrapper.get('[data-testid="managed-proxy-apply"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="managed-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="managed-proxy-current"]').text()).toContain('exit-two')
  })

  it('falls back to the code itself for a refusal it has no wording for', async () => {
    api.refreshToken.mockRejectedValue({ status: 400, message: 'x', metadata: { code: 'brand_new_code' } })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-token-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-error"]').text()).toContain('admin.accounts.vmManaged.slotError.other')
    expect(wrapper.get('[data-testid="managed-error"]').text()).toContain('brand_new_code')
  })

  it('shows the exit address the slot really uses', async () => {
    api.exitProbe.mockResolvedValue({ ip: '203.0.113.9', country: 'US', city: 'Phoenix' })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-exit-probe"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-exit"]').text()).toBe('203.0.113.9 · US / Phoenix')
  })

  it('explains why a slot needs a person and why automatic re-login stopped', async () => {
    api.getStatus.mockResolvedValue(view({ status: 'needs_attention', binding_status: 'awaiting_login', snapshot: { auto_relogin: { needs_attention: true, attention_reason: 'password_rejected' } } }))
    const wrapper = mountModal()
    await flushPromises()
    const line = wrapper.get('[data-testid="managed-attention"]').text()
    expect(line).toContain('admin.accounts.vmManaged.attentionLine')
    expect(line).toContain('admin.accounts.vmManaged.attention.password_rejected')
  })

  it('shows how long a restriction lasts', async () => {
    api.getStatus.mockResolvedValue(view({ status: 'restricted', snapshot: { restriction_text: '7d', restriction_until: Date.UTC(2026, 10, 15, 19, 47) } }))
    const wrapper = mountModal()
    await flushPromises()
    const line = wrapper.get('[data-testid="managed-restriction"]').text()
    expect(line).toContain('admin.accounts.vmManaged.restrictionUntil')
    expect(line).toContain('"reason":"7d"')
  })

  it('offers retry for a slot that failed to create and nothing that needs a slot', async () => {
    api.getStatus.mockResolvedValue(view({ status: 'error', binding_status: 'error', slot_id: '', activated_at: null, last_error: 'The slot refused this operation: provision_failed', snapshot: null }))
    api.retrySlot.mockResolvedValue(view({ status: 'awaiting_login', binding_status: 'awaiting_login', activated_at: null }))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="managed-password-submit"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="managed-proxy-apply"]').exists()).toBe(false)
    expect(api.passwordStatus).not.toHaveBeenCalled()
    api.getStatus.mockResolvedValue(view({ status: 'awaiting_login', binding_status: 'awaiting_login', activated_at: null }))
    await wrapper.get('[data-testid="managed-retry"]').trigger('click')
    await flushPromises()
    expect(api.retrySlot).toHaveBeenCalledWith(42)
    expect(wrapper.find('[data-testid="managed-password-submit"]').exists()).toBe(true)
  })

  it('deletes only after a second confirmation, and a failed deletion can be retried', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="managed-delete-confirm"]').exists()).toBe(false)
    await wrapper.get('[data-testid="managed-delete"]').trigger('click')
    expect(api.deleteAccount).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.vmManaged.deleteConfirm')
    api.deleteAccount.mockRejectedValueOnce({ status: 409, message: 'x', metadata: { code: 'account_busy' } })
    api.getStatus.mockResolvedValue(view({ status: 'available', binding_status: 'delete_failed' }))
    await wrapper.get('[data-testid="managed-delete-confirm"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('deleted')).toBeFalsy()
    expect(wrapper.get('[data-testid="managed-retry"]').text()).toBe('admin.accounts.vmManaged.retryDelete')
    // A slot that is being removed offers no login or maintenance.
    expect(wrapper.find('[data-testid="managed-password-submit"]').exists()).toBe(false)
    api.deleteAccount.mockResolvedValueOnce(undefined)
    await wrapper.get('[data-testid="managed-retry"]').trigger('click')
    await flushPromises()
    expect(api.deleteAccount).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('deleted')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('pauses and resumes scheduling through the account switch', async () => {
    api.setSchedulable.mockResolvedValue(view())
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-schedulable"]').trigger('click')
    await flushPromises()
    expect(api.setSchedulable).toHaveBeenCalledWith(42, false)
    await wrapper.setProps({ account: { ...account, id: 43, schedulable: false } as unknown as Account })
    await flushPromises()
    await wrapper.get('[data-testid="managed-schedulable"]').trigger('click')
    await flushPromises()
    expect(api.setSchedulable).toHaveBeenLastCalledWith(43, true)
  })

  it('runs diagnosis as a plain read and restart as a guarded action', async () => {
    api.diagnose.mockResolvedValue({ ok: false, checks: [{ id: 'proxy', ok: true, label: 'Proxy' }, { id: 'kernel', ok: false, detail: 'not running' }] })
    api.recover.mockResolvedValue({ ok: true, checks: [{ id: 'kernel', ok: true }] })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-diagnose"]').trigger('click')
    await flushPromises()
    const report = wrapper.get('[data-testid="managed-diagnosis"]').text()
    expect(report).toContain('✓ Proxy')
    expect(report).toContain('✗ kernel')
    expect(report).toContain('not running')
    const before = api.getStatus.mock.calls.length
    await wrapper.get('[data-testid="managed-recover"]').trigger('click')
    await flushPromises()
    expect(api.recover).toHaveBeenCalledWith(42)
    expect(api.getStatus.mock.calls.length).toBe(before + 1)
  })

  it('retries an operation once after the second factor and drops it when that is cancelled', async () => {
    api.refreshToken.mockRejectedValueOnce(stepUpRequired).mockResolvedValueOnce({})
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-token-refresh"]').trigger('click')
    await flushPromises()
    // The second-factor dialog is open; the operation has been tried once.
    expect(api.refreshToken).toHaveBeenCalledTimes(1)
    const controller = wrapper.findComponent(TotpStepUpDialog).props('controller') as { onVerified: () => void; onCancel: () => void }
    controller.onVerified()
    await flushPromises()
    expect(api.refreshToken).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="managed-notice"]').text()).toBe('admin.accounts.vmManaged.tokenRefreshed')

    api.powerOff.mockRejectedValueOnce(stepUpRequired)
    await wrapper.get('[data-testid="managed-power-off"]').trigger('click')
    await flushPromises()
    controller.onCancel()
    await flushPromises()
    expect(api.powerOff).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="managed-error"]').exists()).toBe(false)
  })

  it('shows a starting slot in plain words and follows it until it is up', async () => {
    vi.useFakeTimers()
    api.getStatus.mockResolvedValue(view({ status: 'starting', binding_status: 'awaiting_login', activated_at: null }))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-status"]').text()).toBe('admin.accounts.vmManaged.status.starting')
    expect(wrapper.get('[data-testid="managed-attention"]').text()).toBe('admin.accounts.vmManaged.startingHint')
    // Re-read by itself while it starts; nobody has to click anything.
    const reads = api.getStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(5100)
    await flushPromises()
    expect(api.getStatus.mock.calls.length).toBe(reads + 1)
    expect(wrapper.emitted('updated')).toBeFalsy()
    api.getStatus.mockResolvedValue(view())
    await vi.advanceTimersByTimeAsync(5100)
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-status"]').text()).toBe('admin.accounts.vmManaged.status.available')
    expect(wrapper.emitted('updated')).toHaveLength(1)
    // Once it is up the panel stops reading on its own.
    const after = api.getStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(30000)
    expect(api.getStatus.mock.calls.length).toBe(after)
  })

  it('stops following a starting slot when the panel closes, and gives up after six minutes', async () => {
    vi.useFakeTimers()
    api.getStatus.mockResolvedValue(view({ status: 'starting' }))
    const wrapper = mountModal()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6 * 60 * 1000 + 30000)
    await flushPromises()
    // One read on opening plus at most 72 follow-ups.
    expect(api.getStatus.mock.calls.length).toBeLessThanOrEqual(73)
    const reads = api.getStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.getStatus.mock.calls.length).toBe(reads)
    const other = mountModal()
    await flushPromises()
    const before = api.getStatus.mock.calls.length
    await other.setProps({ show: false })
    await vi.advanceTimersByTimeAsync(30000)
    expect(api.getStatus.mock.calls.length).toBe(before)
    wrapper.unmount()
  })

  it('says a slow power-on or restart is still starting instead of done', async () => {
    api.powerOn.mockResolvedValue({ starting: true })
    api.recover.mockResolvedValue({ starting: true })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="managed-power-on"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-notice"]').text()).toBe('admin.accounts.vmManaged.startingNotice')
    await wrapper.get('[data-testid="managed-recover"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-notice"]').text()).toBe('admin.accounts.vmManaged.startingNotice')
    expect(wrapper.find('[data-testid="managed-diagnosis"]').exists()).toBe(false)
    api.powerOn.mockResolvedValue({ ok: true })
    await wrapper.get('[data-testid="managed-power-on"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-notice"]').text()).toBe('admin.accounts.vmManaged.poweredOn')
  })

  it('shows a token expiry given as epoch digits as a date, not a number', async () => {
    const at = Date.UTC(2026, 10, 22, 12, 54, 12)
    for (const expires of [String(at), at, String(at / 1000), '2026-11-22T12:54:12Z']) {
      api.getStatus.mockResolvedValue(view({ snapshot: { has_token: true, expires_at: expires as string } }))
      const wrapper = mountModal()
      await flushPromises()
      const text = wrapper.text()
      expect(text).toContain(new Date(at).toLocaleString())
      expect(text).not.toContain(String(at))
      wrapper.unmount()
    }
  })

  it('ignores answers that arrive after the panel moved to another account', async () => {
    let release: (value: OpenAIVMManagedView) => void = () => {}
    api.getStatus.mockImplementationOnce(() => new Promise<OpenAIVMManagedView>(resolve => { release = resolve }))
    const wrapper = mountModal()
    await flushPromises()
    api.getStatus.mockResolvedValue(view({ account_id: 43, slot_id: 's2a-abcdefabcdef-43', snapshot: { email: 'second@example.test' } }))
    await wrapper.setProps({ account: { ...account, id: 43, name: 'second' } as unknown as Account })
    await flushPromises()
    release(view({ slot_id: 'stale-slot', snapshot: { email: 'stale@example.test' } }))
    await flushPromises()
    expect(wrapper.text()).toContain('second@example.test')
    expect(wrapper.text()).not.toContain('stale@example.test')
    expect(wrapper.text()).not.toContain('stale-slot')
  })
})
