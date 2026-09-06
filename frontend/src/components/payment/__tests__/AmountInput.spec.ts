import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AmountInput from '../AmountInput.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key} ${JSON.stringify(params || {})}` }),
}))

function mountAmounts() {
  return mount(AmountInput, {
    props: {
      modelValue: 10,
      amounts: [10, 30, 50, 5000],
      min: 10,
      max: 2000,
      currency: 'CNY',
      rechargeMultiplier: 0.5,
      bonusRules: [{ id: 1, recharge_amount: 30, bonus_amount: 1, validity_days: 7 }],
    },
  })
}

describe('AmountInput recharge tiers', () => {
  it('renders only configured, in-range tiers with accurate credit and bonus terms', () => {
    const wrapper = mountAmounts()
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(3)
    expect(buttons[0].attributes('aria-pressed')).toBe('true')
    expect(buttons[0].text()).toContain('payment.baseCreditPreview {"amount":"5.00"}')
    expect(buttons[0].text()).toContain('payment.noTierBonus')
    expect(buttons[1].text()).toContain('payment.baseCreditPreview {"amount":"15.00"}')
    expect(buttons[1].text()).toContain('payment.tierBonus {"amount":"1.00","days":7}')
    expect(wrapper.text()).toContain('payment.bonusExplanation')
  })

  it('keeps preset selection, custom decimals and clearing functional', async () => {
    const wrapper = mountAmounts()
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([30])
    const input = wrapper.get('input')
    await input.setValue('31.25')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([31.25])
    await input.setValue('31.259')
    expect(input.element.value).toBe('31.25')
    await input.setValue('')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([null])
  })
})
