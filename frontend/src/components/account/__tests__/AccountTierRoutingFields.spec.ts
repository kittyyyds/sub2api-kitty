import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import AccountTierRoutingFields from '../AccountTierRoutingFields.vue'
import { readAccountTierRouting, withAccountTierRouting, type AccountTierPool } from '@/utils/accountTierRouting'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('account tier routing', () => {
  it('retains the selected pool while disabled and preserves unrelated account fields', async () => {
    const extra = { pool: 'premium', tiered_routing_enabled: false, usage_counter: 42 }
    const wrapper = mount(defineComponent({
      components: { AccountTierRoutingFields },
      setup() {
        const initial = readAccountTierRouting(extra)
        return { enabled: ref(initial.enabled), pool: ref<AccountTierPool>(initial.pool) }
      },
      template: '<AccountTierRoutingFields v-model:enabled="enabled" v-model:pool="pool" />'
    }))
    expect(wrapper.find('select').exists()).toBe(false)
    await wrapper.get('input').setValue(true)
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('premium')
    await wrapper.get('select').setValue('standard')
    await wrapper.get('input').setValue(false)
    expect(wrapper.find('select').exists()).toBe(false)
    expect(withAccountTierRouting(extra, wrapper.vm.enabled, wrapper.vm.pool)).toEqual({
      pool: 'standard', tiered_routing_enabled: false, usage_counter: 42
    })
    expect(extra.pool).toBe('premium')
  })

  it('preserves explicitly configured legacy pools and defaults new accounts to off', () => {
    expect(readAccountTierRouting({ pool: ' Premium ' })).toEqual({ enabled: true, pool: 'premium' })
    expect(readAccountTierRouting({ pool: 'premium', tiered_routing_enabled: false }).enabled).toBe(false)
    expect(readAccountTierRouting({})).toEqual({ enabled: false, pool: 'standard' })
  })
})
