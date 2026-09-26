import { afterEach, describe, it, expect, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
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

enableAutoUnmount(afterEach)

function mountInput(value: number | null = null) {
  return mount(AmountInput, { props: { modelValue: value } })
}

describe('recharge amount input', () => {
  it.each(['10abc', '10.555', '-10', '1e2'])('restores the accepted amount after rejecting %s', async (value) => {
    const wrapper = mountInput(10)
    const input = wrapper.get('input')
    await input.setValue(value)
    expect((input.element as HTMLInputElement).value).toBe('10')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('restores the last typed amount rather than a stale prop', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    await input.setValue('12.50')
    await input.setValue('12.500')
    expect((input.element as HTMLInputElement).value).toBe('12.50')
    expect(wrapper.emitted('update:modelValue')).toEqual([[12.5]])
  })

  it('preserves decimal editing and allows clearing the amount', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    for (const value of ['0', '0.', '0.5', '0.50', '']) await input.setValue(value)
    expect(wrapper.emitted('update:modelValue')).toEqual([[null], [null], [0.5], [0.5], [null]])
    expect((input.element as HTMLInputElement).value).toBe('')
  })
})
