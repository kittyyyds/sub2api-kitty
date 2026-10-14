import { describe, expect, it, vi } from 'vitest'
import { apiClient } from '../../client'
import {
  formatTokenGuardReloginText,
  parseTwoFALoginText,
  parseTokenGuardReloginText,
  startTwoFALogin
} from '../accountTokenGuard'

vi.mock('../../client', () => ({ apiClient: { post: vi.fn() } }))

describe('token guard re-login credential format', () => {
  it('parses the account----password----2FA format', () => {
    expect(parseTokenGuardReloginText(
      'user@example.com----p,a,s,s----JBSWY3DPEHPK3PXP\nsecond@example.com----secret----'
    )).toEqual([
      { email: 'user@example.com', password: 'p,a,s,s', mfa_secret: 'JBSWY3DPEHPK3PXP' },
      { email: 'second@example.com', password: 'secret', mfa_secret: '' }
    ])
  })

  it('keeps reading legacy comma-separated entries and formats them with the new separator', () => {
    const accounts = parseTokenGuardReloginText('user@example.com,password,JBSWY3DPEHPK3PXP')
    expect(formatTokenGuardReloginText(accounts)).toBe('user@example.com----password----JBSWY3DPEHPK3PXP')
  })
})


describe('initial 2FA login input', () => {
  it('defaults new UI logins to credential operations', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: { id: 'job', status: 'running' } })
    const entry = { email: 'user@example.com', password: 'test-password', mfa_secret: 'test-secret' }
    await startTwoFALogin(entry)
    expect(apiClient.post).toHaveBeenCalledWith('/admin/account-ops/token-guard/two-fa-login', {
      ...entry, credential_target: 'operations',
    })
  })
  it('supports both separators and passwords containing commas', () => {
    expect(parseTwoFALoginText('USER@example.com----p,a!ss----SECRET\nsecond@example.com,password,SECRET')).toEqual([
      { email: 'user@example.com', password: 'p,a!ss', mfa_secret: 'SECRET' },
      { email: 'second@example.com', password: 'password', mfa_secret: 'SECRET' }
    ])
  })
  it.each(['', 'bad', 'u@example.com----p----', 'u@example.com----p----s\nU@example.com----p----s'])('rejects invalid or duplicate entries', input => {
    expect(() => parseTwoFALoginText(input)).toThrow('invalid_batch')
  })
})

it('supports desktop separators, embedded password separators, and sends the selected proxy',async()=>{
 expect(parseTwoFALoginText('u@example.com--p--inside--KEY\nv@example.com\tpass\tKEY')).toEqual([
 {email:'u@example.com',password:'p--inside',mfa_secret:'KEY'}, {email:'v@example.com',password:'pass',mfa_secret:'KEY'}])
 const entry={email:'u@example.com',password:'pass',mfa_secret:'KEY'}
 vi.mocked(apiClient.post).mockResolvedValue({data:{id:'job',status:'running'}})
 await startTwoFALogin(entry,17)
 expect(apiClient.post).toHaveBeenLastCalledWith('/admin/account-ops/token-guard/two-fa-login',{...entry,credential_target:'operations',proxy_id:17})
 expect(()=>parseTwoFALoginText(Array.from({length:201},(_,i)=>`u${i}@example.com--p--KEY`).join('\n'))).toThrow()
})
