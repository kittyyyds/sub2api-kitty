import { describe, expect, it } from 'vitest'

import { redirectQuery, sanitizeRedirectPath } from '../authRedirect'

describe('sanitizeRedirectPath', () => {
  it('keeps same-site paths with their query', () => {
    expect(sanitizeRedirectPath('/purchase')).toBe('/purchase')
    expect(sanitizeRedirectPath('/oauth/mcp/authorize?client_id=a&state=b')).toBe('/oauth/mcp/authorize?client_id=a&state=b')
  })

  it.each([
    [undefined],
    [''],
    ['https://evil.example'],
    ['//evil.example'],
    ['/\\evil.example'],
    ['javascript:alert(1)'],
    ['/ok\nSet-Cookie: x'],
    [['/dashboard']],
  ])('falls back for %j', (value) => {
    expect(sanitizeRedirectPath(value)).toBe('/dashboard')
  })

  it('uses the given fallback', () => {
    expect(sanitizeRedirectPath('https://evil.example', '/home')).toBe('/home')
  })
})

describe('redirectQuery', () => {
  it('carries a safe redirect and drops an unsafe one', () => {
    expect(redirectQuery('/purchase')).toEqual({ redirect: '/purchase' })
    expect(redirectQuery('//evil.example')).toBeUndefined()
    expect(redirectQuery(undefined)).toBeUndefined()
  })
})
