import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import type { Account } from '@/types'
import { refreshOpenAIQuota, resetOpenAIQuota } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({
  refreshOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
  refreshOpenAIReferrals: vi.fn(),
  sendOpenAIReferralInvite: vi.fn(),
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const account = (extra: Record<string, unknown>) => ({
  id: 1, name: 'acc', platform: 'openai', type: 'oauth', status: 'active', schedulable: true, extra,
}) as unknown as Account

// The quota query and the quota reset both go to OpenAI with the account's own
// token. A VM2-managed account has none here, so the cell must not offer them:
// its reset credits are shown and spent from the managed panel.
describe('OpenAI quota cell on a VM2-managed account', () => {
  it('renders nothing and calls nothing for a managed account', () => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: account({ openai_vm_managed: true, codex_reset_credit_snapshot: { available_count: 3 } }) } })
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.text()).toBe('')
    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    expect(resetOpenAIQuota).not.toHaveBeenCalled()
  })

  it('still renders for an ordinary OAuth account, and for a mark that is not exactly true', () => {
    for (const extra of [{}, { openai_vm_managed: false }, { openai_vm_managed: 'true' }]) {
      const wrapper = mount(OpenAIQuotaResetCell, { props: { account: account(extra) } })
      expect(wrapper.findAll('button').length).toBeGreaterThan(0)
    }
  })
})
