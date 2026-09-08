import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import KittyRiskControlView from '../KittyRiskControlView.vue'

const api = vi.hoisted(() => ({ getConfig: vi.fn(), updateConfig: vi.fn(), getStatus: vi.fn(), listLogs: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin/kittyRiskControl', () => ({ riskControlAPI: api }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

const ToggleStub = defineComponent({
  props: { modelValue: Boolean }, emits: ['update:modelValue'],
  setup(props, { emit }) { return () => h('button', { role: 'switch', 'aria-checked': String(props.modelValue), onClick: () => emit('update:modelValue', !props.modelValue) }) }
})

function render() {
  return mount(KittyRiskControlView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' }, BaseDialog: true, Icon: true, Pagination: true, Toggle: ToggleStub,
  } } })
}

describe('Kitty risk release switches', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getConfig.mockResolvedValue({ deepseek_channels: [], remote_reviewers: [] })
    api.getStatus.mockResolvedValue(null)
    api.listLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    api.updateConfig.mockImplementation(async (input) => input)
  })
  it('renders missing switches as off and saves evidence independently', async () => {
    const wrapper = render()
    await flushPromises()
    expect(api.showError).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="risk-control-enabled"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.get('[data-test="kitty-risk-evidence-enabled"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('[data-test="kitty-risk-evidence-enabled"]').trigger('click')
    await wrapper.get('[data-test="save-risk-control"]').trigger('click')
    await flushPromises()
    expect(api.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, evidence_enabled: true }))
    wrapper.unmount()
  })
  it('does not offer saving when the persisted configuration cannot be loaded', async () => {
    api.getConfig.mockRejectedValue(new Error('offline'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-test="risk-config-load-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="save-risk-control"]').exists()).toBe(false)
    expect(api.updateConfig).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
