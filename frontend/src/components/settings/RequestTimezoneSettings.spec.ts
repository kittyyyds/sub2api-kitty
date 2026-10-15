import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import RequestTimezoneSettings from './RequestTimezoneSettings.vue'

function render(enabled = false) {
  return mount(RequestTimezoneSettings, {
    props: { modelValue: enabled, auto: true },
    global: { plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false })] },
  })
}

describe('RequestTimezoneSettings', () => {
  it('keeps the source controls hidden while disabled', async () => {
    const wrapper = render()
    expect(wrapper.find('#request-timezone-mode').exists()).toBe(false)
    await wrapper.find('#request-timezone-enabled').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
    expect(wrapper.emitted('update:auto')).toBeUndefined()
  })

  it('selects automatic and manual sources independently of the main switch', async () => {
    const wrapper = render(true)
    expect((wrapper.get('#request-timezone-mode').element as HTMLSelectElement).value).toBe('auto')
    await wrapper.get('#request-timezone-mode').setValue('manual')
    expect(wrapper.emitted('update:auto')).toEqual([[false]])
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
