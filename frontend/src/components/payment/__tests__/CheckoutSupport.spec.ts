import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CheckoutSupport from '../CheckoutSupport.vue'

const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('CheckoutSupport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true })
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn().mockReturnValue(true) })
  })

  afterEach(() => {
    vi.useRealTimers()
    Reflect.deleteProperty(document, 'execCommand')
  })

  it('copies the requested QQ and announces success only after copying', async () => {
    const wrapper = mount(CheckoutSupport)
    expect(wrapper.text()).toContain('1722345668')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('1722345668')
    expect(wrapper.text()).toContain('payment.copied')
    expect(showSuccess).toHaveBeenCalledOnce()
    vi.advanceTimersByTime(2000)
    await flushPromises()
    expect(wrapper.text()).toContain('payment.copyQQ')
    wrapper.unmount()
  })

  it('falls back after clipboard rejection and cleans up the temporary element', async () => {
    vi.mocked(navigator.clipboard.writeText).mockRejectedValue(new Error('denied'))
    const wrapper = mount(CheckoutSupport)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(document.execCommand).toHaveBeenCalledWith('copy')
    expect(document.querySelector('textarea')).toBeNull()
    expect(showSuccess).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('shows an error and removes the temporary element when both copy methods fail', async () => {
    vi.mocked(navigator.clipboard.writeText).mockRejectedValue(new Error('denied'))
    vi.mocked(document.execCommand).mockImplementation(() => { throw new Error('blocked') })
    const wrapper = mount(CheckoutSupport)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('common.copyFailed')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(document.querySelector('textarea')).toBeNull()
    wrapper.unmount()
  })
})
