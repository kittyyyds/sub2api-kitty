import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CursorAccountFields from '../CursorAccountFields.vue'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Cursor account credentials', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue({ data: { available: true } })
    post.mockReset().mockResolvedValue({ data: { authenticated: true } })
  })
  const verifyButton = (wrapper: ReturnType<typeof mount>) => wrapper.findAll('button').find(b => b.text() === 'admin.accounts.cursor.verify')!

  it('verifies the saved account without retrieving its secret', async () => {
    const wrapper = mount(CursorAccountFields, { props: { modelValue: '', editing: true, accountId: 42 } })
    await flushPromises()
    await verifyButton(wrapper).trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/admin/accounts/cursor/verify', { account_id: 42 }, expect.any(Object))
    expect(wrapper.text()).toContain('admin.accounts.cursor.verified')
    expect(wrapper.get('input').attributes('type')).toBe('password')
    wrapper.unmount()
  })

  it('does not report a stale verification after the key changes', async () => {
    let finish!: (value: unknown) => void
    post.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mount(CursorAccountFields, { props: { modelValue: 'crsr_test_account_aaaaaaaa' } })
    await flushPromises()
    await verifyButton(wrapper).trigger('click')
    const signal = post.mock.calls[0][2].signal as AbortSignal
    await wrapper.setProps({ modelValue: 'crsr_test_account_bbbbbbbb' })
    expect(signal.aborted).toBe(true)
    finish({ data: { authenticated: true } })
    await flushPromises()
    expect(wrapper.text()).not.toContain('admin.accounts.cursor.verified')
    wrapper.unmount()
  })

  it('rejects the wrong key format locally and shows safe server errors', async () => {
    const wrapper = mount(CursorAccountFields, { props: { modelValue: 'password-not-a-user-key' } })
    await flushPromises()
    await verifyButton(wrapper).trigger('click')
    expect(post).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toBe('admin.accounts.cursor.invalidKey')
    await wrapper.setProps({ modelValue: 'crsr_test_account_aaaaaaaa' })
    post.mockRejectedValue({ response: { data: { message: 'Cursor 认证未通过' } } })
    await verifyButton(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Cursor 认证未通过')
    wrapper.unmount()
  })
})
