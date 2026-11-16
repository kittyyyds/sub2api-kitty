import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put, post, remove } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn(), remove: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, put, post, delete: remove } }))
import { openaiVmManagedAPI as api } from '@/api/admin/openaiVmManaged'

describe('VM2-managed account API', () => {
  const options = { timeout: 120_000 }
  beforeEach(() => {
    for (const fn of [get, put, post, remove]) fn.mockReset().mockResolvedValue({ data: {} })
  })

  it('creates the account without any credential field', async () => {
    const input = { name: 'managed', proxy_id: 7, runtime_type: 'kvm' as const, group_ids: [3], concurrency: 3, priority: 1 }
    await api.createAccount(input)
    expect(post).toHaveBeenCalledWith('/admin/openai-vm/managed-accounts', input, options)
    const sent = JSON.stringify(post.mock.calls[0][1])
    for (const field of ['token', 'password', 'secret', 'credential']) expect(sent).not.toContain(field)
  })

  it('uses account-scoped endpoints for status, retry, proxy, scheduling, usage and deletion', async () => {
    await api.getStatus(42)
    await api.retrySlot(42)
    await api.changeProxy(42, 9)
    await api.setSchedulable(42, false)
    await api.refreshUsage(42)
    await api.deleteAccount(42)
    expect(get).toHaveBeenCalledWith('/admin/accounts/42/vm-managed', options)
    expect(post).toHaveBeenCalledWith('/admin/accounts/42/vm-managed/retry', undefined, options)
    expect(put).toHaveBeenCalledWith('/admin/accounts/42/vm-managed/proxy', { proxy_id: 9 }, options)
    expect(post).toHaveBeenCalledWith('/admin/accounts/42/vm-managed/schedulable', { schedulable: false }, options)
    expect(post).toHaveBeenCalledWith('/admin/accounts/42/vm-managed/usage/refresh', undefined, options)
    expect(remove).toHaveBeenCalledWith('/admin/accounts/42/vm-managed', options)
  })

  it('sends each login step to its own operation', async () => {
    await api.oauthUrl(42)
    await api.oauthCode(42, { session_id: 's', callback_url: 'http://localhost:1455/auth/callback?code=c' })
    await api.passwordLogin(42, { email: 'a@example.test', password: 'p', mfa_mode: 'secret', mfa_secret: 'S' })
    await api.passwordLogin(42, { use_saved: true })
    await api.passwordCode(42, 'job', '123456')
    await api.passwordCancel(42, 'job')
    await api.forgetSavedLogin(42)
    await api.importAccountFile(42, '{"k":1}')
    await api.passwordStatus(42)
    const base = '/admin/accounts/42/vm-managed/op/'
    expect(post.mock.calls.map(call => [call[0], call[1]])).toEqual([
      [base + 'login/oauth-url', {}],
      [base + 'login/oauth-code', { session_id: 's', callback_url: 'http://localhost:1455/auth/callback?code=c' }],
      [base + 'login/password', { email: 'a@example.test', password: 'p', mfa_mode: 'secret', mfa_secret: 'S' }],
      [base + 'login/password', { use_saved: true }],
      [base + 'login/password/code', { job_id: 'job', code: '123456' }],
      [base + 'login/password/cancel', { job_id: 'job' }],
      [base + 'login/password/forget', {}],
      [base + 'login/import', { raw: '{"k":1}' }]
    ])
    expect(get).toHaveBeenCalledWith(base + 'login/password', options)
  })

  it('sends each maintenance step to its own operation and reads diagnosis with GET', async () => {
    await api.refreshToken(42)
    await api.useResetCredit(42)
    await api.powerOn(42)
    await api.powerOff(42)
    await api.recover(42)
    await api.exitProbe(42)
    await api.diagnose(42)
    const base = '/admin/accounts/42/vm-managed/op/'
    expect(post.mock.calls.map(call => call[0])).toEqual(['token/refresh', 'usage/reset-credit', 'power/on', 'power/off', 'recover', 'exit-probe'].map(name => base + name))
    expect(get).toHaveBeenCalledWith(base + 'diagnose', options)
  })

  it('offers nothing that reads a saved login back', () => {
    for (const name of Object.keys(api)) expect(name.toLowerCase()).not.toMatch(/reveal|export/)
  })

  it('propagates failures instead of returning success', async () => {
    post.mockRejectedValueOnce(new Error('refused'))
    await expect(api.passwordLogin(42, { use_saved: true })).rejects.toThrow('refused')
    remove.mockRejectedValueOnce(new Error('busy'))
    await expect(api.deleteAccount(42)).rejects.toThrow('busy')
  })
})
