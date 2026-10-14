import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import OpenAIRequestTimezoneField from '../OpenAIRequestTimezoneField.vue'
import { readOpenAIRequestTimezone, withOpenAIRequestTimezone } from '@/utils/openAIRequestTimezone'
import { withAccountTierRouting } from '@/utils/accountTierRouting'

const { catalog, probe } = vi.hoisted(() => ({ catalog: vi.fn(), probe: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getOpenAIRequestTimezones: catalog, probeOpenAIProxyTimezone: probe } } }))

const SelectStub = defineComponent({
  props: ['modelValue', 'options'], emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{option.label}}</option></select>'
})

describe('OpenAI request timezone account settings', () => {
  beforeEach(() => {
    catalog.mockReset()
    probe.mockReset()
    probe.mockResolvedValue({ success: true, timezone: 'America/Los_Angeles', ip_address: '192.0.2.1', country: 'US', city: 'Los Angeles', checked_at: '2026-10-14T10:00:00Z' })
    catalog.mockResolvedValue({ default: 'Asia/Singapore', timezones: ['Asia/Singapore', 'Asia/Tokyo', 'America/New_York'] })
  })

  it('defaults legacy accounts off, allows independent search sync and retains the selection when disabled', async () => {
    const wrapper = mount(defineComponent({
      components: { OpenAIRequestTimezoneField },
      setup: () => ({ settings: ref(readOpenAIRequestTimezone()) }),
      template: '<OpenAIRequestTimezoneField v-model="settings" />'
    }), { global: { stubs: { Select: SelectStub } } })
    expect(wrapper.find('select').exists()).toBe(false)
    expect(catalog).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="request-timezone-enabled"]').setValue(true)
    await flushPromises()
    await wrapper.get('[data-testid="request-timezone-target"]').setValue('Asia/Tokyo')
    await wrapper.get('[data-testid="request-timezone-search"]').setValue(true)
    await wrapper.get('[data-testid="request-timezone-enabled"]').setValue(false)
    expect(wrapper.vm.settings).toEqual({ enabled: false, timezone: 'Asia/Tokyo', syncSearch: true })
    expect(wrapper.find('select').exists()).toBe(false)
    await wrapper.get('[data-testid="request-timezone-enabled"]').setValue(true)
    expect((wrapper.get('[data-testid="request-timezone-target"]').element as HTMLSelectElement).value).toBe('Asia/Tokyo')
    expect(catalog).toHaveBeenCalledTimes(1)
  })

  it('preserves tier routing, model mappings and custom account fields in both save orders', () => {
    const original = { pool: 'premium', tiered_routing_enabled: true, quota_daily_limit: 50, custom_feature: { keep: true }, openai_passthrough: true }
    const settings = { enabled: true, timezone: 'America/New_York', syncSearch: false }
    const withTimezoneFirst = withAccountTierRouting(withOpenAIRequestTimezone(original, settings), true, 'premium')
    const withTierFirst = withOpenAIRequestTimezone(withAccountTierRouting(original, true, 'premium'), settings)
    expect(withTimezoneFirst).toEqual(withTierFirst)
    expect(withTierFirst).toMatchObject(original)
    expect(original).not.toHaveProperty('openai_request_timezone')
    expect(readOpenAIRequestTimezone({ openai_request_timezone: 'Asia/Tokyo' }).enabled).toBe(false)
  })

  it('does not silently change a stored policy when the catalog cannot load', async () => {
    catalog.mockRejectedValue(new Error('offline'))
    const settings = { enabled: true, timezone: 'America/New_York', syncSearch: true }
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: settings }, global: { stubs: { Select: SelectStub } } })
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('probes the selected proxy without overwriting the manual timezone, and supports switching back', async () => {
    const wrapper = mount(defineComponent({
      components: { OpenAIRequestTimezoneField },
      setup: () => ({ settings: ref({ enabled: true, timezone: 'Asia/Tokyo', syncSearch: false }) }),
      template: '<OpenAIRequestTimezoneField v-model="settings" :proxy-id="7" />'
    }), { global: { stubs: { Select: SelectStub } } })
    await wrapper.get('[data-testid="request-timezone-mode"]').setValue('proxy')
    await flushPromises()
    expect(probe).toHaveBeenCalledWith(7, false)
    expect(wrapper.get('[data-testid="request-timezone-result"]').text()).toContain('America/Los_Angeles')
    expect(wrapper.vm.settings.timezone).toBe('Asia/Tokyo')
    expect(wrapper.find('[data-testid="request-timezone-target"]').exists()).toBe(false)
    await wrapper.get('[data-testid="request-timezone-detect"]').trigger('click')
    await flushPromises()
    expect(probe).toHaveBeenLastCalledWith(7, true)
    await wrapper.get('[data-testid="request-timezone-mode"]').setValue('manual')
    expect(wrapper.find('[data-testid="request-timezone-result"]').exists()).toBe(false)
    expect(withOpenAIRequestTimezone({ openai_request_timezone_mode: 'proxy' }, wrapper.vm.settings).openai_request_timezone_mode).toBe('manual')
    expect((wrapper.get('[data-testid="request-timezone-target"]').element as HTMLSelectElement).value).toBe('Asia/Tokyo')
  })

  it('requires a proxy and reports detection failures without changing the stored policy', async () => {
    const settings = { enabled: true, mode: 'proxy' as const, timezone: 'Asia/Tokyo', syncSearch: true }
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: settings }, global: { stubs: { Select: SelectStub } } })
    await flushPromises()
    expect(probe).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="request-timezone-detect"]').attributes('disabled')).toBeDefined()
    probe.mockResolvedValueOnce({ success: false, reason: 'timezone_unknown' })
    await wrapper.setProps({ proxyId: 8 })
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toContain('detectFailed')
    probe.mockRejectedValueOnce(new Error('offline'))
    await wrapper.get('[data-testid="request-timezone-detect"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toContain('detectFailed')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('discards a stale result when the proxy changes or the feature is disabled', async () => {
    let finishOld!: (value: unknown) => void
    probe.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    const settings = { enabled: true, mode: 'proxy' as const, timezone: 'Asia/Tokyo', syncSearch: false }
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: settings, proxyId: 1 }, global: { stubs: { Select: SelectStub } } })
    await wrapper.setProps({ proxyId: 2 })
    await flushPromises()
    finishOld({ success: true, timezone: 'Asia/Tokyo' })
    await flushPromises()
    expect(wrapper.get('[data-testid="request-timezone-result"]').text()).toContain('America/Los_Angeles')
    await wrapper.setProps({ modelValue: { ...settings, enabled: false } })
    expect(wrapper.find('[data-testid="request-timezone-result"]').exists()).toBe(false)
  })

  it('round trips the opt-in mode independently of other account extras', () => {
    const original = { openai_request_timezone_mode: 'proxy', custom: 'keep' }
    const settings = readOpenAIRequestTimezone(original)
    expect(settings.mode).toBe('proxy')
    expect(withOpenAIRequestTimezone(original, settings)).toMatchObject(original)
    expect(readOpenAIRequestTimezone({ openai_request_timezone_mode: 'invalid' }).mode).toBeUndefined()
  })
})
