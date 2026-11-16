import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import LoginView from '@/views/auth/LoginView.vue'

const {
  getPublicSettingsMock,
  loginMock,
  loginWithPasskeyMock,
  routerPushMock,
  showErrorMock,
  showSuccessMock,
  showWarningMock
} = vi.hoisted(() => ({
  getPublicSettingsMock: vi.fn(),
  loginMock: vi.fn(),
  loginWithPasskeyMock: vi.fn(),
  routerPushMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showWarningMock: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute: { value: { query: {} } },
    push: routerPushMock
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    login: loginMock,
    loginWithPasskey: loginWithPasskeyMock
  }),
  useAppStore: () => ({
    // The login page reads the app's cached settings; serve the same values the test configures.
    fetchPublicSettings: () => getPublicSettingsMock(),
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showWarning: showWarningMock
  })
}))

vi.mock('@/api/auth', () => ({
  buildOAuthLoginStartURL: vi.fn(() => '/api/v1/auth/oauth/start'),
  getPublicSettings: getPublicSettingsMock,
  isTotp2FARequired: vi.fn(() => false),
  isWeChatWebOAuthEnabled: vi.fn(() => false),
  startOAuthLogin: vi.fn()
}))

const agreementSettings = {
  registration_enabled: true,
  turnstile_enabled: false,
  tencent_captcha_enabled: false,
  aliyun_captcha_enabled: false,
  linuxdo_oauth_enabled: false,
  dingtalk_oauth_enabled: false,
  wechat_oauth_enabled: false,
  backend_mode_enabled: false,
  oidc_oauth_enabled: false,
  github_oauth_enabled: false,
  google_oauth_enabled: false,
  password_reset_enabled: true,
  passkey_enabled: false,
  login_agreement_enabled: true,
  login_agreement_mode: 'checkbox',
  login_agreement_revision: 'agreement-v1',
  login_agreement_documents: [{ id: 'terms', title: 'Terms of service' }]
}

function mountView() {
  return mount(LoginView, {
    global: {
      stubs: {
        AuthLayout: { template: '<main><slot /><slot name="footer" /></main>' },
        DingTalkOAuthSection: true,
        EmailOAuthButtons: true,
        Icon: true,
        LinuxDoOAuthSection: true,
        LoginAgreementPrompt: true,
        OidcOAuthSection: true,
        RouterLink: { template: '<a><slot /></a>' },
        TotpLoginModal: true,
        TurnstileWidget: true,
        WechatOAuthSection: true,
        transition: false
      }
    }
  })
}

describe('LoginView agreement gate UX', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    getPublicSettingsMock.mockReset()
    loginMock.mockReset()
    loginWithPasskeyMock.mockReset()
    routerPushMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    showWarningMock.mockReset()
    getPublicSettingsMock.mockResolvedValue(agreementSettings)
  })

  it('keeps login fields editable and checks the agreement only on submit', async () => {
    const wrapper = mountView()
    await flushPromises()

    const email = wrapper.get('#email')
    const password = wrapper.get('#password')
    const submit = wrapper.get('button[type="submit"]')
    expect(email.attributes('disabled')).toBeUndefined()
    expect(password.attributes('disabled')).toBeUndefined()
    expect(submit.attributes('disabled')).toBeUndefined()

    await email.setValue('user@example.com')
    await password.setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showWarningMock).toHaveBeenCalledWith(
      'legal.loginAgreementPrompt.loginRequiredWarning'
    )
    expect(loginMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([true, false])('honors registration_enabled=%s without enabling login Turnstile', async (enabled) => {
    getPublicSettingsMock.mockResolvedValueOnce({
      ...agreementSettings,
      registration_enabled: enabled,
      turnstile_enabled: true,
      turnstile_site_key: 'test-site-key'
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text().includes('auth.signUp')).toBe(enabled)
    expect(wrapper.findComponent({ name: 'TurnstileWidget' }).exists()).toBe(false)
    wrapper.unmount()
  })
})
