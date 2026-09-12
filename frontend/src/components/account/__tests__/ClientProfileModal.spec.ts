import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { Account } from '@/types'
import ClientProfileModal from '../ClientProfileModal.vue'

const api = vi.hoisted(() => ({ getById: vi.fn(), updateClientProfile: vi.fn(), getClientProfileEgress: vi.fn(), probeClientProfileEgress: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: api } }))
const account = (id = 1, extra: Record<string, unknown> = {}): Account => ({ id, name: `GPT #${id}`, platform: 'openai', type: 'oauth', extra, proxy_id: null } as Account)
const mountModal = (props: { account?: Account | null; accountIds?: number[] } = {}) => mount(ClientProfileModal, {
  props: { show: true, account: account(), ...props },
  global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } }
})

describe('GPT client profile controls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getClientProfileEgress.mockResolvedValue(null)
    api.updateClientProfile.mockImplementation(async (id: number, profile: unknown) => account(id, { openai_client_profile: profile }))
  })

  it('opens existing accounts disabled and saves only the profile without overwriting extra or convergence', async () => {
    const wrapper = mountModal({ account: account(1, { codex_fingerprint_mode: 'device', usage_snapshot: { used: 100 } }) })
    await flushPromises()
    expect((wrapper.get('[data-testid="profile-enabled"]').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateClientProfile).toHaveBeenCalledWith(1, expect.objectContaining({ enabled: false, preset: 'default' }), undefined)
    expect(api.updateClientProfile.mock.calls[0][1]).not.toHaveProperty('usage_snapshot')
    expect(api.updateClientProfile.mock.calls[0][1]).not.toHaveProperty('concurrency')
    expect(api.updateClientProfile.mock.calls[0][1]).not.toHaveProperty('rpm')
  })

  it('supports explicit Mac choice while leaving connection settings inherited', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="profile-enabled"]').setValue(true)
    await wrapper.get('[data-testid="profile-preset"]').setValue('macos')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateClientProfile).toHaveBeenCalledWith(1, expect.objectContaining({ enabled: true, preset: 'macos', ws_mode: 'inherit', tls_profile: 'inherit' }), undefined)
  })

  it('rejects incompatible HTTP/2 and custom TLS before sending a save request', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="profile-enabled"]').setValue(true)
    await wrapper.get('[data-testid="profile-http"]').setValue('http2')
    await wrapper.get('[data-testid="profile-tls"]').setValue('chrome')
    await wrapper.get('form').trigger('submit')
    expect(api.updateClientProfile).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('HTTP/1.1')
  })

  it('only changes convergence when it is explicitly selected', async () => {
    const wrapper = mountModal({ account: account(1, { codex_fingerprint_mode: 'full' }) })
    await wrapper.get('#profile-convergence').setValue('device')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateClientProfile.mock.calls[0][2]).toBe('device')
  })

  it('does not claim failed batch accounts were saved and skips other platforms', async () => {
    api.getById.mockImplementation(async (id: number) => id === 2 ? { ...account(id), platform: 'anthropic' } : account(id))
    api.updateClientProfile.mockImplementation(async (id: number) => { if (id === 3) throw new Error('保存失败'); return account(id) })
    const wrapper = mountModal({ account: null, accountIds: [1, 2, 3] })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateClientProfile.mock.calls.map(call => call[0])).toEqual([1, 3])
    expect(wrapper.get('[role="status"]').text()).toContain('成功 1 个，跳过非 GPT 授权账号 1 个，失败 1 个')
    expect(wrapper.get('[role="alert"]').text()).toContain('#3')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('skips shadow accounts whose credentials belong to a parent', async () => {
    api.getById.mockResolvedValue({ ...account(2), parent_account_id: 1 })
    const wrapper = mountModal({ account: null, accountIds: [2] })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateClientProfile).not.toHaveBeenCalled()
    expect(wrapper.get('[role="status"]').text()).toContain('成功 0 个，跳过')
  })

  it('clears a previous account egress result when a late response arrives', async () => {
    let resolve!: (value: unknown) => void
    api.getClientProfileEgress.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const wrapper = mountModal()
    await wrapper.setProps({ account: account(2) })
    resolve({ account_id: 1, proxy_id: null, ip: '192.0.2.1', timezone: 'Asia/Tokyo', observed_at: '2026-09-12T00:00:00Z' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('192.0.2.1')
    expect(wrapper.text()).toContain('当前出口尚无检测结果')
  })
})
