import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OAuthCallbackView from '@/views/auth/OAuthCallbackView.vue'

const {
  routeState,
  locationState,
  routerReplaceMock,
  showErrorMock,
  showSuccessMock,
  setTokenMock,
  copyToClipboardMock,
  exchangePendingOAuthCompletionMock,
  apiPostMock,
} = vi.hoisted(() => ({
  routeState: {
    path: '/auth/callback',
    query: {} as Record<string, unknown>,
  },
  locationState: {
    current: {
      href: 'http://localhost/auth/callback',
      hash: '',
    } as { href: string; hash: string },
  },
  routerReplaceMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  setTokenMock: vi.fn(),
  copyToClipboardMock: vi.fn(),
  exchangePendingOAuthCompletionMock: vi.fn(),
  apiPostMock: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace: (...args: any[]) => routerReplaceMock(...args),
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key === 'auth.errors.REGISTRATION_IP_LIMIT' ? '当前 IP 已注册过两个账号，如需更多账号请联系客服。' : key,
    te: (key: string) => key === 'auth.errors.REGISTRATION_IP_LIMIT',
  }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    setToken: (...args: any[]) => setTokenMock(...args),
  }),
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
    showSuccess: (...args: any[]) => showSuccessMock(...args),
  }),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiPostMock(...args),
  },
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    exchangePendingOAuthCompletion: (...args: any[]) => exchangePendingOAuthCompletionMock(...args),
    persistOAuthTokenContext: vi.fn(),
  }
})

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: (...args: any[]) => copyToClipboardMock(...args),
  }),
}))

describe('OAuthCallbackView', () => {
  beforeEach(() => {
    routeState.path = '/auth/callback'
    routeState.query = {}
    locationState.current = {
      href: 'http://localhost/auth/callback',
      hash: '',
    }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    })
    routerReplaceMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    setTokenMock.mockReset()
    copyToClipboardMock.mockReset()
    exchangePendingOAuthCompletionMock.mockReset()
    apiPostMock.mockReset()
    window.sessionStorage.clear()
  })

  it('renders localized callback copy actions', () => {
    routeState.query = {
      code: 'oauth-code',
      state: 'oauth-state',
    }

    const wrapper = mount(OAuthCallbackView)

    expect(wrapper.text()).toContain('auth.oauth.callbackTitle')
    expect(wrapper.text()).toContain('auth.oauth.callbackHint')
    expect(wrapper.text()).toContain('common.copy')
    expect(wrapper.find('input[value="oauth-code"]').exists()).toBe(true)
    expect(wrapper.find('input[value="oauth-state"]').exists()).toBe(true)
  })

  it('sends callback errors to toast instead of rendering inline red text', () => {
    routeState.query = {
      error: 'oauth failed',
    }

    const wrapper = mount(OAuthCallbackView)

    expect(showErrorMock).toHaveBeenCalledWith('oauth failed')
    expect(wrapper.text()).not.toContain('oauth failed')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })

  it('shows a translated reason and the back-to-login panel when the provider sign-in failed', async () => {
    routeState.path = '/auth/oauth/callback'
    locationState.current.hash = '#error=REGISTRATION_IP_LIMIT&error_message=this+IP+address+has+already+registered+two+accounts'

    const wrapper = mount(OAuthCallbackView)
    await Promise.resolve()

    expect(showErrorMock).toHaveBeenCalledWith('当前 IP 已注册过两个账号，如需更多账号请联系客服。')
    expect(wrapper.text()).toContain('auth.oauth.invalidCallbackTitle')
    expect(wrapper.text()).toContain('auth.backToLogin')
    expect(wrapper.text()).not.toContain('auth.oauth.callbackTitle')
  })

  it('does not render manual copy fields for direct email oauth callback visits', async () => {
    routeState.path = '/auth/oauth/callback'
    exchangePendingOAuthCompletionMock.mockRejectedValue(new Error('pending session not found'))

    const wrapper = mount(OAuthCallbackView)
    await vi.dynamicImportSettled()

    expect(exchangePendingOAuthCompletionMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('auth.oauth.invalidCallbackTitle')
    expect(wrapper.text()).toContain('auth.oauth.invalidCallbackHint')
    expect(wrapper.find('input[readonly]').exists()).toBe(false)
  })

  it('forwards frontend email oauth provider callbacks back to the backend callback endpoint', async () => {
    routeState.path = '/auth/oauth/callback'
    routeState.query = {
      code: 'provider-code',
      state: 'provider-state',
    }
    window.sessionStorage.setItem('email_oauth_pending_provider', 'google')

    mount(OAuthCallbackView)
    await vi.dynamicImportSettled()

    expect(locationState.current.href).toBe(
      '/api/v1/auth/oauth/google/callback?code=provider-code&state=provider-state'
    )
    expect(exchangePendingOAuthCompletionMock).not.toHaveBeenCalled()
  })

  it('submits stored affiliate code when completing invited email oauth registration', async () => {
    routeState.path = '/auth/oauth/callback'
    exchangePendingOAuthCompletionMock.mockResolvedValue({
      error: 'invitation_required',
      provider: 'google',
      redirect: '/dashboard',
      resolved_email: 'pending@example.com',
      invitation_required: true,
    })
    apiPostMock.mockResolvedValue({
      data: {
        access_token: 'token-1',
      },
    })
    window.sessionStorage.setItem('oauth_aff_code', 'AFF456')

    const wrapper = mount(OAuthCallbackView)
    await vi.dynamicImportSettled()
    const passwordInputs = wrapper.findAll('input[type="password"]')
    await passwordInputs[0].setValue('secret-123')
    await passwordInputs[1].setValue('secret-123')
    const invitationInput = wrapper.find('input[type="text"]')
    await invitationInput.setValue('INVITE456')
    await wrapper.findAll('button').at(0)?.trigger('click')

    expect(apiPostMock).toHaveBeenCalledWith('/auth/oauth/google/complete-registration', {
      password: 'secret-123',
      invitation_code: 'INVITE456',
      aff_code: 'AFF456',
    })
    expect(setTokenMock).toHaveBeenCalledWith('token-1')
  })

  it('completes email oauth registration with readonly email and without posting email', async () => {
    routeState.path = '/auth/oauth/callback'
    exchangePendingOAuthCompletionMock.mockResolvedValue({
      error: 'registration_completion_required',
      provider: 'github',
      redirect: '/dashboard',
      resolved_email: 'verified@example.com',
      invitation_required: false,
    })
    apiPostMock.mockResolvedValue({
      data: {
        access_token: 'token-2',
      },
    })

    const wrapper = mount(OAuthCallbackView)
    await vi.dynamicImportSettled()

    const emailInput = wrapper.find('input[type="email"]')
    expect(emailInput.exists()).toBe(true)
    expect((emailInput.element as HTMLInputElement).value).toBe('verified@example.com')
    expect(emailInput.attributes('readonly')).toBeDefined()
    expect(emailInput.attributes('disabled')).toBeDefined()

    const passwordInputs = wrapper.findAll('input[type="password"]')
    await passwordInputs[0].setValue('secret-456')
    await passwordInputs[1].setValue('secret-456')
    await wrapper.findAll('button').at(0)?.trigger('click')

    expect(apiPostMock).toHaveBeenCalledWith('/auth/oauth/github/complete-registration', {
      password: 'secret-456',
    })
    expect(apiPostMock.mock.calls[0][1]).not.toHaveProperty('email')
    expect(setTokenMock).toHaveBeenCalledWith('token-2')
  })
})
