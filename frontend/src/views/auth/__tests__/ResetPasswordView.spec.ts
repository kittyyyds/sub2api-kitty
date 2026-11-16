import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ResetPasswordView from '@/views/auth/ResetPasswordView.vue'

const { resetPasswordMock, showErrorMock, showSuccessMock } = vi.hoisted(() => ({
  resetPasswordMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
}))

const translations: Record<string, string> = {
  'auth.invalidOrExpiredToken': '密码重置链接无效或已过期。请重新请求一个新链接。',
  'auth.resetPasswordFailed': '重置密码失败，请重试。',
  'auth.errors.PASSWORD_RESET_DISABLED': '暂未开放找回密码，请联系管理员。',
}

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: { email: 'user@example.com', token: 'reset-token' } }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => translations[key] ?? key }) }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: showErrorMock, showSuccess: showSuccessMock }),
}))

vi.mock('@/api/auth', () => ({ resetPassword: resetPasswordMock }))

async function submitNewPassword() {
  const wrapper = mount(ResetPasswordView, {
    global: {
      stubs: {
        AuthLayout: { template: '<main><slot /><slot name="footer" /></main>' },
        Icon: true,
        RouterLink: { template: '<a><slot /></a>' },
        transition: false,
      },
    },
  })
  await flushPromises()
  await wrapper.get('#password').setValue('new-secret-1')
  await wrapper.get('#confirmPassword').setValue('new-secret-1')
  await wrapper.get('form').trigger('submit.prevent')
  await flushPromises()
  return wrapper
}

describe('ResetPasswordView backend errors', () => {
  beforeEach(() => {
    resetPasswordMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
  })

  it('explains an expired or used reset link instead of blaming the new password', async () => {
    // The API client rejects with a flat object and has already guessed "输入内容无效。" from the English text.
    resetPasswordMock.mockRejectedValueOnce({ status: 400, reason: 'INVALID_RESET_TOKEN', message: '输入内容无效。' })

    await submitNewPassword()

    expect(showErrorMock).toHaveBeenLastCalledWith('密码重置链接无效或已过期。请重新请求一个新链接。')
  })

  it('translates other backend reasons', async () => {
    resetPasswordMock.mockRejectedValueOnce({ status: 403, reason: 'PASSWORD_RESET_DISABLED', message: '没有权限执行此操作。' })

    await submitNewPassword()

    expect(showErrorMock).toHaveBeenLastCalledWith('暂未开放找回密码，请联系管理员。')
  })
})
