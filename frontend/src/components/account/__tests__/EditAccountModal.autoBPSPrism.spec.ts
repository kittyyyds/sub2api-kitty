import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { defaultQualityBPS } from '@/utils/qualityRulePatch'

enableAutoUnmount(afterEach)

const mocks = vi.hoisted(() => ({
  updateAccount: vi.fn(),
  listByAccount: vi.fn(),
  createPlan: vi.fn(),
  updatePlan: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: mocks.showError, showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: mocks.showWarning })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getManagementCapabilities: vi.fn().mockResolvedValue({ web_search_enabled: false, account_quota_notify_enabled: false }),
      update: mocks.updateAccount,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) },
    scheduledTests: { listByAccount: mocks.listByAccount, create: mocks.createPlan, update: mocks.updatePlan }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function buildOAuthAccount(patch: Record<string, unknown> = {}) {
  return {
    id: 7, name: 'OpenAI OAuth', notes: '', platform: 'openai', type: 'oauth',
    credentials: { access_token: 'offline-fixture-token', chatgpt_account_id: 'offline-fixture' }, extra: {},
    credentials_status: { has_prism_cookie_encrypted: true },
    proxy_id: null, concurrency: 1, priority: 1, rate_multiplier: 1, status: 'active',
    group_ids: [], expires_at: null, auto_pause_on_expired: false, parent_account_id: null,
    ...patch
  } as any
}

function buildRule(patch: Record<string, unknown> = {}) {
  return {
    id: 31, account_id: 7, model_id: 'gpt-6-astra', cron_expression: '*/30 * * * *', enabled: true, max_results: 100,
    auto_recover: false, last_run_at: null, next_run_at: null, created_at: '', updated_at: '',
    pelican_config: { question_kind: 'state_probe', prompt: '', quality: {
      expected_answer: '', action: 'enable_bps', remove_group_ids: [], auto_restore: true, bps: { ...defaultQualityBPS(), models: [], all_models: true } } },
    ...patch
  }
}

function mountModal(account = buildOAuthAccount()) {
  return mount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
    global: {
      stubs: { BaseDialog: BaseDialogStub, Select: true, Icon: true, ProxySelector: true, GroupSelector: true, ModelWhitelistSelector: true }
    }
  })
}

async function submit(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
  await flushPromises()
}

const toggleSelector = '[data-testid="account-auto-bps-toggle"]'

describe('EditAccountModal Prism and automatic BPS interoperability', () => {
  beforeEach(() => {
    Object.values(mocks).forEach(mock => mock.mockReset())
    mocks.updateAccount.mockImplementation(async (_id: number, payload: Record<string, unknown>) => ({ ...buildOAuthAccount(), ...payload }))
    mocks.listByAccount.mockResolvedValue([])
    mocks.createPlan.mockImplementation(async (request: Record<string, unknown>) => ({ ...buildRule(), ...request, id: 40 }))
    mocks.updatePlan.mockImplementation(async (id: number, request: Record<string, unknown>) => ({ ...buildRule(), ...request, id }))
  })

  async function enablePrism(wrapper: ReturnType<typeof mountModal>) {
    await wrapper.get('[data-testid="prism-toggle"]').trigger('click')
    await wrapper.get('[data-testid="prism-all-models"]').setValue(true)
  }

  function expectNoWrites() {
    expect(mocks.updateAccount).not.toHaveBeenCalled()
    expect(mocks.createPlan).not.toHaveBeenCalled()
    expect(mocks.updatePlan).not.toHaveBeenCalled()
    expect(mocks.showError).toHaveBeenCalledWith('admin.accounts.openai.prismAutoBPSConflict')
  }

  it('does not enable Prism while an existing automatic BPS rule can still switch the protocol', async () => {
    mocks.listByAccount.mockResolvedValue([buildRule()])
    const wrapper = mountModal()
    await flushPromises()
    await enablePrism(wrapper)
    await submit(wrapper)
    expectNoWrites()
    expect(wrapper.emitted('updated')).toBeUndefined()
  })

  it('does not save a new automatic BPS rule together with Prism', async () => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get(toggleSelector).trigger('click')
    await enablePrism(wrapper)
    await submit(wrapper)
    expectNoWrites()
  })

  it('requires persisting the pause before enabling Prism instead of treating an unsaved toggle as a stopped rule', async () => {
    mocks.listByAccount.mockResolvedValue([buildRule()])
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get(toggleSelector).trigger('click')
    await enablePrism(wrapper)
    await submit(wrapper)
    expectNoWrites()
  })

  it('allows Prism after the automatic BPS rule has been paused and the saved state reloaded', async () => {
    mocks.listByAccount.mockResolvedValue([buildRule()])
    const first = mountModal()
    await flushPromises()
    await first.get(toggleSelector).trigger('click')
    await submit(first)
    expect(mocks.updatePlan).toHaveBeenCalledWith(31, { enabled: false })
    expect(first.emitted('updated')).toHaveLength(1)
    first.unmount()

    mocks.listByAccount.mockResolvedValue([buildRule({ enabled: false })])
    mocks.updateAccount.mockClear()
    mocks.updatePlan.mockClear()
    const reopened = mountModal()
    await flushPromises()
    await enablePrism(reopened)
    await submit(reopened)
    expect(mocks.updateAccount).toHaveBeenCalledWith(7, expect.objectContaining({
      extra: expect.objectContaining({ openai_prism: true }),
    }))
    expect(mocks.updatePlan).not.toHaveBeenCalled()
    expect(mocks.createPlan).not.toHaveBeenCalled()
    expect(reopened.emitted('updated')).toHaveLength(1)
  })

  it('blocks an enabled BPS rule from another probe type instead of assuming the managed rule is the only writer', async () => {
    mocks.listByAccount.mockResolvedValue([buildRule({ pelican_config: {
      question_kind: 'custom', quality: { action: 'enable_bps' },
    } })])
    const wrapper = mountModal()
    await flushPromises()
    await enablePrism(wrapper)
    await submit(wrapper)
    expectNoWrites()
  })

  it('keeps ordinary non-Prism automatic BPS configuration working', async () => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get(toggleSelector).trigger('click')
    await submit(wrapper)
    expect(mocks.updateAccount).toHaveBeenCalledTimes(1)
    expect(mocks.createPlan).toHaveBeenCalledWith(expect.objectContaining({
      account_id: 7, enabled: true,
      pelican_config: expect.objectContaining({ quality: expect.objectContaining({ action: 'enable_bps' }) }),
    }))
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it('does not prevent Prism because of a quality rule that never enables BPS', async () => {
    mocks.listByAccount.mockResolvedValue([buildRule({ pelican_config: {
      question_kind: 'state_probe', quality: { action: 'disable_scheduling' },
    } })])
    const wrapper = mountModal()
    await flushPromises()
    await enablePrism(wrapper)
    await submit(wrapper)
    expect(mocks.updateAccount).toHaveBeenCalledTimes(1)
    expect(mocks.createPlan).not.toHaveBeenCalled()
    expect(mocks.updatePlan).not.toHaveBeenCalled()
    expect(mocks.showError).not.toHaveBeenCalled()
  })
})
