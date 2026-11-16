import { describe, expect, it } from 'vitest'
import { userDisplayName } from '../userDisplayName'

describe('current user display name', () => {
  it('prefers the authenticated username over the account email', () => {
    expect(userDisplayName({ username: ' 张三 ', email: 'alice@example.com' })).toBe('张三')
    expect(userDisplayName({ username: '李四', email: 'bob@example.com' })).toBe('李四')
  })
  it('uses the same account email prefix when no username has been set', () => {
    expect(userDisplayName({ username: '', email: 'alice@example.com' })).toBe('alice')
    expect(userDisplayName({ username: ' ', email: 'bob@example.com' })).toBe('bob')
    expect(userDisplayName({ username: null, email: '用户乙@example.com' })).toBe('用户乙')
  })
  it('does not fabricate a brand or reuse a previous account name', () => {
    expect(userDisplayName(undefined)).toBe('')
    expect(userDisplayName(null)).toBe('')
    expect(userDisplayName({ username: '', email: '' })).toBe('')
    expect(userDisplayName({ username: 'Kitty' })).toBe('Kitty')
  })
})
