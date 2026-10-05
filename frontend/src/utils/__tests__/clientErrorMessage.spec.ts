import { beforeEach, describe, expect, it, vi } from 'vitest'

import { localizeClientErrorMessage } from '../clientErrorMessage'

describe('localizeClientErrorMessage', () => {
  beforeEach(() => {
    document.documentElement.lang = 'zh'
  })

  it('translates common backend errors into Chinese', () => {
    expect(localizeClientErrorMessage('Invalid credentials')).toBe('账号或密码错误。')
    expect(localizeClientErrorMessage('Internal server error')).toBe('服务器内部错误，请稍后重试。')
    expect(localizeClientErrorMessage('API key is invalid')).toBe('API Key 无效。')
  })

  it('translates network, auth, and rate-limit errors', () => {
    expect(localizeClientErrorMessage('Network error. Please check your connection.')).toBe(
      '网络错误，请检查网络连接。',
    )
    expect(localizeClientErrorMessage('Session expired. Please log in again.')).toBe(
      '登录状态已过期，请重新登录。',
    )
    expect(localizeClientErrorMessage('Too many requests')).toBe('请求过于频繁，请稍后再试。')
  })

  it('uses error codes and falls back without exposing raw English', () => {
    expect(localizeClientErrorMessage('', { code: 'INSUFFICIENT_BALANCE' })).toBe('余额不足。')
    expect(
      localizeClientErrorMessage('Provider returned an unexpected failure', {
        status: 500,
        fallback: '操作失败，请稍后重试。',
      }),
    ).toBe('操作失败，请稍后重试。')
  })

  it('keeps existing Chinese messages and keeps English when locale is English', () => {
    expect(localizeClientErrorMessage('余额不足，请充值')).toBe('余额不足，请充值')

    document.documentElement.lang = 'en'
    expect(localizeClientErrorMessage('Invalid credentials')).toBe('Invalid credentials')
  })
})
