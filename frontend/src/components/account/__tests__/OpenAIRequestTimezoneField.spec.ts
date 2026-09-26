import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import OpenAIRequestTimezoneField from '../OpenAIRequestTimezoneField.vue'
import { readOpenAIRequestTimezone, withOpenAIRequestTimezone } from '@/utils/openAIRequestTimezone'
import { withAccountTierRouting } from '@/utils/accountTierRouting'

const { catalog } = vi.hoisted(() => ({ catalog: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getOpenAIRequestTimezones: catalog } } }))

const SelectStub = defineComponent({
  props: ['modelValue', 'options'], emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{option.label}}</option></select>'
})

describe('OpenAI request timezone account settings', () => {
  beforeEach(() => {
    catalog.mockReset()
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
    await wrapper.get('select').setValue('Asia/Tokyo')
    await wrapper.get('[data-testid="request-timezone-search"]').setValue(true)
    await wrapper.get('[data-testid="request-timezone-enabled"]').setValue(false)
    expect(wrapper.vm.settings).toEqual({ enabled: false, timezone: 'Asia/Tokyo', syncSearch: true })
    expect(wrapper.find('select').exists()).toBe(false)
    await wrapper.get('[data-testid="request-timezone-enabled"]').setValue(true)
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('Asia/Tokyo')
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
})
