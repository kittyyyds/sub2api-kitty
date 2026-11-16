import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import RegisterView from '@/views/auth/RegisterView.vue'
import { validateInvitationCode, validatePromoCode } from '@/api/auth'

const {
  fetchPublicSettingsMock,
  registerMock,
  routerPushMock,
  showErrorMock,
  showSuccessMock,
  showWarningMock,
  verifyActionMock
} = vi.hoisted(() => ({
  fetchPublicSettingsMock: vi.fn(),
  registerMock: vi.fn(),
  routerPushMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showWarningMock: vi.fn(),
  verifyActionMock: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: routerPushMock })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' },
      t: (key: string) =>
        key === 'auth.emailDomainRegistrationLimit'
          ? '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
          : key === 'auth.emailAliasNotAllowed' ? '不支持使用邮箱别名注册，请去掉 Gmail 用户名中的点号、+标签或域名末尾的点号后重试。'
            : key === 'auth.errors.REGISTRATION_IP_LIMIT' ? '当前 IP 已注册过两个账号，如需更多账号请联系客服。' : key
    })
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ register: registerMock }),
  useAppStore: () => ({
    fetchPublicSettings: fetchPublicSettingsMock,
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showWarning: showWarningMock
  })
}))

vi.mock('@/api/auth', () => ({
  isWeChatWebOAuthEnabled: () => false,
  validateInvitationCode: vi.fn(),
  validatePromoCode: vi.fn()
}))

const enabledSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  promo_code_enabled: false,
  invitation_code_enabled: false,
  affiliate_enabled: true,
  turnstile_enabled: true,
  turnstile_site_key: 'registration-site-key',
  site_name: 'Kitty',
  linuxdo_oauth_enabled: false,
  wechat_oauth_enabled: false,
  oidc_oauth_enabled: false,
  oidc_oauth_provider_name: 'OIDC',
  github_oauth_enabled: false,
  google_oauth_enabled: false,
  registration_email_suffix_whitelist: [],
  registration_email_domain_quota_enabled: false,
  login_agreement_enabled: false
}

function mountView() {
  return mount(RegisterView, {
    global: {
      stubs: {
        AuthLayout: { template: '<main><slot /><slot name="footer" /></main>' },
        EmailOAuthButtons: true,
        Icon: true,
        LinuxDoOAuthSection: true,
        LoginAgreementPrompt: true,
        OidcOAuthSection: true,
        RouterLink: { template: '<a><slot /></a>' },
        TurnstileWidget: {
          name: 'TurnstileWidget',
          props: ['siteKey'],
          methods: { verifyAction: verifyActionMock, reset: vi.fn() },
          template: '<div data-testid="turnstile-widget" />'
        },
        WechatOAuthSection: true,
        transition: false
      }
    }
  })
}

describe('RegisterView security settings bootstrap', () => {
  beforeEach(() => {
    fetchPublicSettingsMock.mockReset()
    registerMock.mockReset()
    routerPushMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    showWarningMock.mockReset()
    verifyActionMock.mockReset()
    verifyActionMock.mockResolvedValue({ token: 'ticket', randstr: 'randstr' })
    sessionStorage.removeItem('register_data')
    fetchPublicSettingsMock.mockResolvedValue(enabledSettings)
    registerMock.mockResolvedValue({})
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not flash a disabled promo code while public settings are loading', async () => {
    let resolveSettings!: (settings: typeof enabledSettings) => void
    fetchPublicSettingsMock.mockReturnValueOnce(new Promise<typeof enabledSettings>(resolve => {
      resolveSettings = resolve
    }))
    const wrapper = mountView()
    expect(wrapper.find('#promo_code').exists()).toBe(false)
    resolveSettings(enabledSettings)
    await flushPromises()
    expect(wrapper.find('#promo_code').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows an enabled promo code after the trusted settings bootstrap finishes', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({ ...enabledSettings, promo_code_enabled: true })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('#promo_code').exists()).toBe(true)
    wrapper.unmount()
  })
  it('keeps registration closed while a transient settings failure is retried', async () => {
    vi.useFakeTimers()
    fetchPublicSettingsMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(enabledSettings)

    const wrapper = mountView()
    await flushPromises()

    expect(fetchPublicSettingsMock).toHaveBeenCalledWith(false)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('[data-testid="registration-settings-loading"]').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()

    expect(fetchPublicSettingsMock).toHaveBeenLastCalledWith(true)
    expect(wrapper.find('form').exists()).toBe(true)
    expect(wrapper.get('[data-testid="turnstile-widget"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('fails closed when registration security settings cannot be loaded', async () => {
    vi.useFakeTimers()
    fetchPublicSettingsMock.mockResolvedValue(null)

    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_200)
    await flushPromises()

    expect(fetchPublicSettingsMock).toHaveBeenCalledTimes(3)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('[data-testid="registration-settings-error"]').exists()).toBe(true)
    expect(registerMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    ['', 'auth.confirmPasswordRequired'],
    ['different-password', 'auth.passwordsDoNotMatch']
  ])('blocks invalid confirmation %j before captcha and allows correction', async (confirmation, error) => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'app-id'
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue(confirmation)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(error)
    expect(wrapper.get('#confirmPassword').classes()).toContain('input-error')
    expect(registerMock).not.toHaveBeenCalled()
    expect(verifyActionMock).not.toHaveBeenCalled()
    expect(routerPushMock).not.toHaveBeenCalled()
    expect(sessionStorage.getItem('register_data')).toBeNull()

    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('#confirmPassword').classes()).not.toContain('input-error')
    expect(verifyActionMock).toHaveBeenCalledOnce()
    expect(registerMock).toHaveBeenCalledWith({
      email: 'user@example.com',
      password: 'secret-123',
      turnstile_token: undefined,
      tencent_captcha_ticket: 'ticket',
      tencent_captcha_randstr: 'randstr',
      promo_code: undefined,
      invitation_code: undefined
    })
    expect(routerPushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('requires matching confirmation before storing only the registration fields for email verification', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      email_verify_enabled: true
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('different-password')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(sessionStorage.getItem('register_data')).toBeNull()
    expect(routerPushMock).not.toHaveBeenCalled()

    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(JSON.parse(sessionStorage.getItem('register_data')!)).toEqual({
      email: 'user@example.com',
      password: 'secret-123'
    })
    expect(routerPushMock).toHaveBeenCalledWith('/email-verify')
    expect(registerMock).not.toHaveBeenCalled()
  })

  it('keeps the optional affiliate invitation field before Turnstile', async () => {
    fetchPublicSettingsMock.mockResolvedValue(enabledSettings)
    const wrapper = mountView()
    await flushPromises()

    const invitationField = wrapper.get('[data-testid="affiliate-invitation-field"]')
    const turnstile = wrapper.get('[data-testid="registration-turnstile"]')

    expect(invitationField.get('input').attributes('id')).toBe('affiliate_code')
    expect(invitationField.text()).toContain('common.optional')
    expect(
      invitationField.element.compareDocumentPosition(turnstile.element) &
      Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    wrapper.unmount()
  })

  it('keeps registration fields editable and checks the agreement only on submit', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      login_agreement_enabled: true,
      login_agreement_mode: 'checkbox',
      login_agreement_revision: 'agreement-v1',
      login_agreement_documents: [{ id: 'terms', title: 'Terms of service' }]
    })

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
      'legal.loginAgreementPrompt.registerRequiredWarning'
    )
    expect(registerMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('uses the mandatory invitation field without duplicating the affiliate field', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      invitation_code_enabled: true
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="affiliate-invitation-field"]').exists()).toBe(false)
    expect(wrapper.get('#invitation_code').exists()).toBe(true)
    wrapper.unmount()
  })

  it('submits a non-whitelist email domain so the backend can enforce its registration quota', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('first@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'first@custom.example' })
    )
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('shows the localized registration domain quota message returned by the backend', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })
    registerMock.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message'
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('second@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  it('translates a backend reason instead of showing the generic English-pattern guess', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({ ...enabledSettings, turnstile_enabled: false })
    // The API client already turned "this IP address has already registered..." into this guess.
    registerMock.mockRejectedValueOnce({ reason: 'REGISTRATION_IP_LIMIT', message: '该内容已存在。' })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('third@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenLastCalledWith('当前 IP 已注册过两个账号，如需更多账号请联系客服。')
  })

  it('tells the visitor why submit is blocked by an invalid promo code', async () => {
    vi.useFakeTimers()
    fetchPublicSettingsMock.mockResolvedValueOnce({ ...enabledSettings, turnstile_enabled: false, promo_code_enabled: true })
    vi.mocked(validatePromoCode).mockResolvedValueOnce({ valid: false, error_code: 'PROMO_CODE_NOT_FOUND' } as never)

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#promo_code').setValue('NOPE')
    await vi.advanceTimersByTimeAsync(600)
    await flushPromises()
    await wrapper.get('#email').setValue('new@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    showErrorMock.mockClear()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenLastCalledWith('auth.promoCodeInvalidCannotRegister')
  })

  it('trims the invitation code it submits, matching what the validate endpoint accepted', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({ ...enabledSettings, turnstile_enabled: false, invitation_code_enabled: true })
    vi.mocked(validateInvitationCode).mockResolvedValue({ valid: true } as never)

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('new@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('#invitation_code').setValue('  INV-123  ')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({ invitation_code: 'INV-123' }))
  })

  it('repeats the validation message when the same invalid form is submitted again', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({ ...enabledSettings, turnstile_enabled: false })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock.mock.calls.filter(([message]) => message === 'auth.emailRequired')).toHaveLength(2)
  })

  it('shows the localized email alias rejection message returned by the backend', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false
    })
    registerMock.mockRejectedValueOnce({
      reason: 'EMAIL_ALIAS_NOT_ALLOWED',
      message: 'raw backend message'
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('sa.m.ple@gmail.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(
      '不支持使用邮箱别名注册，请去掉 Gmail 用户名中的点号、+标签或域名末尾的点号后重试。'
    )
  })

  // 域名限量注册开关默认关闭：恢复 PR5423 之前的客户端白名单预检，非白名单域名不发起注册请求。
  it('rejects a non-whitelist email domain locally when the domain quota switch is disabled', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('first@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).not.toHaveBeenCalled()
    // 校验失败通过 validationToastMessage watcher 弹 toast
    expect(showErrorMock).toHaveBeenCalledWith('auth.emailSuffixNotAllowedWithAllowed')
    expect(wrapper.get('#email').classes()).toContain('input-error')
  })

  it('still submits whitelisted email domains when the domain quota switch is disabled', async () => {
    fetchPublicSettingsMock.mockResolvedValueOnce({
      ...enabledSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('#email').setValue('user@allowed.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'user@allowed.com' })
    )
    expect(showErrorMock).not.toHaveBeenCalled()
  })
})
