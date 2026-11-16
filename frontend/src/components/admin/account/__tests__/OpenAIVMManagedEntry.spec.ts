import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountActionMenu from '../AccountActionMenu.vue'
import OpenAIVMManagedBadge from '../OpenAIVMManagedBadge.vue'
import OpenAIVMManagedCreateModal from '../OpenAIVMManagedCreateModal.vue'
import type { Account, AdminGroup, Proxy } from '@/types'

const api = vi.hoisted(() => ({ createAccount: vi.fn() }))
vi.mock('@/api/admin/openaiVmManaged', async () => ({
  ...await vi.importActual<typeof import('@/api/admin/openaiVmManaged')>('@/api/admin/openaiVmManaged'),
  openaiVmManagedAPI: api
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, te: () => false })
}))

const base = { id: 1, name: 'acc', platform: 'openai', type: 'oauth', parent_account_id: null, schedulable: true, status: 'active' }
const managed = { ...base, extra: { openai_vm_managed: true, openai_vm_managed_status: 'needs_relogin' } } as unknown as Account
const plain = { ...base, extra: {} } as unknown as Account
const anchorRect = new DOMRect(100, 100, 24, 24)
const bodyText = () => document.body.textContent ?? ''

describe('VM2-managed entry points', () => {
  beforeEach(() => {
    api.createAccount.mockReset()
    document.body.innerHTML = ''
  })

  it('gives a managed account its panel and hides the token actions that cannot work', () => {
    const wrapper = mount(AccountActionMenu, { props: { show: true, account: managed, anchorRect }, global: { stubs: { Icon: true } } })
    expect(bodyText()).toContain('admin.accounts.vmManaged.menuOpen')
    for (const hidden of ['admin.accounts.reAuthorize', 'admin.accounts.refreshToken', 'admin.accounts.createSparkShadow', 'admin.accounts.openaiVm.title', 'admin.accounts.openaiIPv6.title', 'admin.accounts.setPrivacy']) {
      expect(bodyText()).not.toContain(hidden)
    }
    document.body.querySelector<HTMLButtonElement>('[data-testid="menu-vm-managed"]')!.click()
    expect(wrapper.emitted('openai-vm-managed')?.[0]).toEqual([managed])
    wrapper.unmount()
  })

  it('leaves an ordinary OAuth account exactly as before', () => {
    const wrapper = mount(AccountActionMenu, { props: { show: true, account: plain, anchorRect }, global: { stubs: { Icon: true } } })
    expect(bodyText()).not.toContain('admin.accounts.vmManaged.menuOpen')
    for (const shown of ['admin.accounts.reAuthorize', 'admin.accounts.refreshToken', 'admin.accounts.createSparkShadow', 'admin.accounts.openaiVm.title', 'admin.accounts.setPrivacy']) {
      expect(bodyText()).toContain(shown)
    }
    wrapper.unmount()
  })

  it('does not treat a shadow or another platform as managed even with the mark', () => {
    for (const account of [{ ...managed, parent_account_id: 9 }, { ...managed, platform: 'anthropic' }, { ...managed, type: 'apikey' }] as unknown as Account[]) {
      const wrapper = mount(AccountActionMenu, { props: { show: true, account, anchorRect }, global: { stubs: { Icon: true } } })
      expect(bodyText()).not.toContain('admin.accounts.vmManaged.menuOpen')
      wrapper.unmount()
      document.body.innerHTML = ''
    }
  })

  it('shows the synced status in the list and opens the panel on click', async () => {
    const wrapper = mount(OpenAIVMManagedBadge, { props: { account: managed } })
    const badge = wrapper.get('[data-testid="vm-managed-badge"]')
    expect(badge.text()).toBe('admin.accounts.vmManaged.status.needs_relogin')
    expect(badge.classes().join(' ')).toContain('red')
    await badge.trigger('click')
    expect(wrapper.emitted('open')?.[0]).toEqual([managed])
    // No synced status yet means the account is still waiting for its first login.
    await wrapper.setProps({ account: { ...managed, extra: { openai_vm_managed: true } } as unknown as Account })
    expect(wrapper.get('[data-testid="vm-managed-badge"]').text()).toBe('admin.accounts.vmManaged.status.awaiting_login')
    // A status this version does not know is shown as unknown, not as raw text.
    await wrapper.setProps({ account: { ...managed, extra: { openai_vm_managed: true, openai_vm_managed_status: '<b>x</b>' } } as unknown as Account })
    expect(wrapper.get('[data-testid="vm-managed-badge"]').text()).toBe('admin.accounts.vmManaged.status.unknown')
    await wrapper.setProps({ account: plain })
    expect(wrapper.find('[data-testid="vm-managed-badge"]').exists()).toBe(false)
  })

  const proxies = [
    { id: 1, name: 'exit-one', protocol: 'socks5', host: 'one.example', port: 1080, status: 'active' },
    { id: 3, name: 'http-proxy', protocol: 'http', host: 'http.example', port: 8080, status: 'active' },
    { id: 4, name: 'expired', protocol: 'socks5', host: 'old.example', port: 1080, status: 'expired' }
  ] as unknown as Proxy[]
  const groups = [{ id: 5, name: 'openai-default', platform: 'openai' }, { id: 6, name: 'claude', platform: 'anthropic' }] as unknown as AdminGroup[]
  const mountCreate = (list = proxies) => mount(OpenAIVMManagedCreateModal, {
    props: { show: true, proxies: list, groups },
    global: { stubs: {
      BaseDialog: { props: ['show'], emits: ['close'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      TotpStepUpDialog: { props: ['controller'], template: '<div />' }
    } }
  })

  it('creates a managed account with a SOCKS5 proxy, KVM by default, and no credential fields', async () => {
    api.createAccount.mockResolvedValue({ account_id: 77, name: 'new managed', managed: null })
    const wrapper = mountCreate()
    expect(wrapper.html()).not.toMatch(/type="password"/)
    const options = wrapper.findAll('#managed-create-proxy option').map(option => option.text())
    expect(options.some(text => text.includes('exit-one'))).toBe(true)
    expect(options.some(text => text.includes('http-proxy'))).toBe(false)
    expect(options.some(text => text.includes('expired'))).toBe(false)
    expect(wrapper.text()).toContain('openai-default')
    expect(wrapper.text()).not.toContain('claude')
    const submit = wrapper.get('[data-testid="managed-create-submit"]')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.get('#managed-create-name').setValue('  new managed ')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.get('#managed-create-proxy').setValue(1)
    expect(submit.attributes('disabled')).toBeUndefined()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).toHaveBeenCalledWith({ name: 'new managed', proxy_id: 1, runtime_type: 'kvm', group_ids: [], concurrency: 3, priority: 1 })
    expect(wrapper.emitted('created')?.[0]).toEqual([{ account_id: 77, name: 'new managed', managed: null }])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('says so when there is no SOCKS5 proxy to choose', () => {
    const wrapper = mountCreate([proxies[1]])
    expect(wrapper.find('[data-testid="managed-create-no-proxy"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="managed-create-submit"]').attributes('disabled')).toBeDefined()
  })

  it('rejects an out-of-range concurrency before sending anything', async () => {
    const wrapper = mountCreate()
    await wrapper.get('#managed-create-name').setValue('x')
    await wrapper.get('#managed-create-proxy').setValue(1)
    for (const bad of [0, 257, 2.5]) {
      await wrapper.get('#managed-create-concurrency').setValue(bad)
      expect(wrapper.get('[data-testid="managed-create-submit"]').attributes('disabled')).toBeDefined()
    }
    await wrapper.get('form').trigger('submit')
    expect(api.createAccount).not.toHaveBeenCalled()
  })

  it('asks for confirmation on a mixed-channel warning and only then sends it', async () => {
    api.createAccount.mockRejectedValueOnce({ status: 409, error: 'mixed_channel_warning', message: 'mixed channels in group' })
    const wrapper = mountCreate()
    await wrapper.get('#managed-create-name').setValue('x')
    await wrapper.get('#managed-create-proxy').setValue(1)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-create-mixed"]').text()).toContain('mixed channels in group')
    expect(wrapper.emitted('created')).toBeFalsy()
    expect(wrapper.get('[data-testid="managed-create-submit"]').attributes('disabled')).toBeDefined()
    api.createAccount.mockResolvedValueOnce({ account_id: 78, name: 'x', managed: null })
    await wrapper.get('[data-testid="managed-create-mixed"] input').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).toHaveBeenLastCalledWith(expect.objectContaining({ confirm_mixed_channel_risk: true }))
    expect(wrapper.emitted('created')).toHaveLength(1)
  })

  it('keeps the form open and shows the reason when creation fails', async () => {
    api.createAccount.mockRejectedValue({ status: 400, message: 'The selected proxy is inactive or expired' })
    const wrapper = mountCreate()
    await wrapper.get('#managed-create-name').setValue('x')
    await wrapper.get('#managed-create-proxy').setValue(1)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-create-error"]').text()).toBe('The selected proxy is inactive or expired')
    expect(wrapper.emitted('close')).toBeFalsy()
    expect((wrapper.get('#managed-create-name').element as HTMLInputElement).value).toBe('x')
  })
})
