
vi.mock('@/api/admin/credentialEncryption', () => ({
  getCredentialEncryption: vi.fn().mockResolvedValue({ configured: true, source: 'server_config' }),
  initializeCredentialEncryption: vi.fn(),
}))
import { getCredentialEncryption, initializeCredentialEncryption } from '@/api/admin/credentialEncryption'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OpenAITwoFAImport from '../OpenAITwoFAImport.vue'
import { deleteTwoFALogin, getTwoFALogin, startTwoFALogin } from '@/api/admin/accountTokenGuard'

vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/accountTokenGuard', async importOriginal => ({
  ...await importOriginal<typeof import('@/api/admin/accountTokenGuard')>(),
  startTwoFALogin: vi.fn(), getTwoFALogin: vi.fn(), deleteTwoFALogin: vi.fn(),
}))
enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(getCredentialEncryption).mockResolvedValue({ configured: true, source: 'server_config' })
  vi.mocked(startTwoFALogin).mockResolvedValue({ id: 'job-1', status: 'running' })
  vi.mocked(getTwoFALogin).mockResolvedValue({ id: 'job-1', status: 'succeeded', credential: { access_token: 'mock-access' } })
  vi.mocked(deleteTwoFALogin).mockResolvedValue()
})

async function start(wrapper: ReturnType<typeof mount>, input: string) {
  await flushPromises()
  await wrapper.get('textarea').setValue(input)
  await wrapper.get('button').trigger('click')
  await flushPromises()
}

describe('OpenAI initial 2FA import', () => {
  it('imports each account, skips completed rows, and retries creation without logging in again', async () => {
    const importCredential = vi.fn().mockResolvedValueOnce('created').mockRejectedValueOnce(new Error('do not display credential'))
    const wrapper = mount(OpenAITwoFAImport, { props: { importCredential } })
    await start(wrapper, 'a@example.com----p1----s1\nb@example.com----p2----s2')
    expect(importCredential).toHaveBeenCalledTimes(2)
    expect(startTwoFALogin).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('tokenGuard.twoFA.states.created')
    expect(wrapper.text()).toContain('tokenGuard.twoFA.states.importFailed')
    expect(wrapper.text()).not.toContain('do not display credential')
    expect(wrapper.find('textarea').exists()).toBe(false)
    importCredential.mockResolvedValueOnce('created')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(importCredential).toHaveBeenCalledTimes(3)
    expect(startTwoFALogin).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('tokenGuard.twoFA.retry')
    expect(importCredential.mock.calls[1]?.[2]).toEqual({ email: 'b@example.com', password: 'p2', mfa_secret: 's2' })
    expect(importCredential.mock.calls[2]?.[2]).toEqual({ email: 'b@example.com', password: 'p2', mfa_secret: 's2' })
    expect(wrapper.emitted('busy')).toEqual([[true], [false], [true], [false]])
  })

  it('never imports when login fails, and rejects malformed batch lines before sending anything', async () => {
    const importCredential = vi.fn()
    const wrapper = mount(OpenAITwoFAImport, { props: { importCredential } })
    await start(wrapper, 'a@example.com----p----s\ninvalid')
    expect(startTwoFALogin).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('invalid')
    vi.mocked(getTwoFALogin).mockResolvedValue({ id: 'job-1', status: 'failed' })
    await start(wrapper, 'a@example.com----p----s')
    expect(importCredential).not.toHaveBeenCalled()
    expect(deleteTwoFALogin).not.toHaveBeenCalled()
  })

  it('resumes a job after a polling failure instead of starting another login', async () => {
    const importCredential = vi.fn().mockResolvedValue('created')
    const wrapper = mount(OpenAITwoFAImport, { props: { importCredential } })
    vi.mocked(getTwoFALogin).mockRejectedValueOnce(new Error('network failure'))
    await start(wrapper, 'a@example.com----p----s')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(startTwoFALogin).toHaveBeenCalledTimes(1)
    expect(importCredential).toHaveBeenCalledTimes(1)
  })

  it('stops an active login without importing or starting the next row', async () => {
    vi.useFakeTimers()
    try {
      vi.mocked(getTwoFALogin).mockResolvedValue({ id: 'job-1', status: 'running' })
      const importCredential = vi.fn()
      const wrapper = mount(OpenAITwoFAImport, { props: { importCredential } })
      await start(wrapper, 'a@example.com----p----s\nb@example.com----p----s')
      await wrapper.get('button').trigger('click')
      await vi.advanceTimersByTimeAsync(1500)
      await flushPromises()
      expect(deleteTwoFALogin).toHaveBeenCalledWith('job-1')
      expect(importCredential).not.toHaveBeenCalled()
      expect(startTwoFALogin).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })
})


it('blocks 2FA login and account creation until encryption initialization succeeds', async () => {
  vi.mocked(getCredentialEncryption).mockResolvedValue({ configured: false, source: 'unconfigured' })
  vi.mocked(initializeCredentialEncryption).mockResolvedValue({ configured: true, source: 'local_file' })
  const importCredential = vi.fn().mockResolvedValue('created')
  const wrapper = mount(OpenAITwoFAImport, { props: { importCredential } })
  await flushPromises()
  await wrapper.get('textarea').setValue('a@example.com----password----secret')
  expect(wrapper.get('[data-testid="two-fa-start"]').attributes('disabled')).toBeDefined()
  await wrapper.get('[data-testid="two-fa-start"]').trigger('click')
  expect(startTwoFALogin).not.toHaveBeenCalled()
  expect(importCredential).not.toHaveBeenCalled()
  await wrapper.get('[data-testid="initialize-credential-encryption"]').trigger('click')
  await flushPromises()
  expect(wrapper.get('[data-testid="two-fa-start"]').attributes('disabled')).toBeUndefined()
  await wrapper.get('[data-testid="two-fa-start"]').trigger('click')
  await flushPromises()
  expect(importCredential).toHaveBeenCalledTimes(1)
})

it('preserves the post-import proxy selection across retries',async()=>{
 const prepare=vi.fn(),importCredential=vi.fn().mockResolvedValue('created')
 vi.mocked(startTwoFALogin).mockRejectedValueOnce(new Error('proxy unavailable'))
 const wrapper=mount(OpenAITwoFAImport,{props:{proxyId:17,prepare,importCredential}})
 await start(wrapper,'u@example.com----pass----KEY')
 expect(prepare).toHaveBeenCalledOnce()
 await wrapper.setProps({proxyId:99})
 await wrapper.get('button').trigger('click');await flushPromises()
 expect(startTwoFALogin).toHaveBeenLastCalledWith(expect.anything(),17)
 expect(prepare).toHaveBeenCalledOnce()
})

it('uses the configured service while preserving the assigned proxy and ticket for account creation', async () => {
 const importer = vi.fn().mockResolvedValue('created')
 vi.mocked(getTwoFALogin).mockResolvedValue({id:'job-1',status:'succeeded',login_backend:'configured_service',proxy_id:17,import_ticket:'ticket-1',credential:{access_token:'fixture-access'}})
 const wrapper = mount(OpenAITwoFAImport,{props:{autoProxy:true,importCredential:importer}})
 await start(wrapper,'a@example.com----fixture-password----FIXTUREMFA')
 expect(wrapper.text()).toContain('原版后台登录服务')
 expect(wrapper.text()).toContain('密码及 2FA 密钥会提交给凭证守护中配置的重登服务')
 expect(wrapper.text()).toContain('导入后绑定代理：#17')
 expect(wrapper.text()).not.toContain('登录与保存使用同一代理')
 expect(importer).toHaveBeenCalledWith({access_token:'fixture-access',_import_ticket:'ticket-1',_import_proxy_id:17},'a@example.com',expect.anything())
 expect(wrapper.text()).not.toContain('fixture-password')
 expect(wrapper.text()).not.toContain('FIXTUREMFA')
})

it('explains exhausted idle proxies before login and retries after capacity is added', async () => {
 const importCredential=vi.fn().mockResolvedValue('created')
 vi.mocked(startTwoFALogin).mockRejectedValueOnce({status:400,message:'没有可分配的 IP：须出口已验证、当前空闲、历史未满且未被预留；未切换直连'})
 const wrapper=mount(OpenAITwoFAImport,{props:{autoProxy:true,importCredential}})
 await start(wrapper,'a@example.com----example-password----EXAMPLEKEY')
 expect(wrapper.text()).toContain('没有符合条件的空闲代理')
 expect(wrapper.text()).toContain('未验证密码或 2FA')
 expect(getTwoFALogin).not.toHaveBeenCalled()
 expect(importCredential).not.toHaveBeenCalled()
 expect(wrapper.text()).not.toContain('example-password')
 await wrapper.get('[data-testid="two-fa-start"]').trigger('click');await flushPromises()
 expect(importCredential).toHaveBeenCalledOnce()
 expect(wrapper.text()).not.toContain('没有符合条件的空闲代理')
})

it('keeps arbitrary server failures and secrets out of preparation diagnostics', async () => {
 vi.mocked(startTwoFALogin).mockRejectedValueOnce({status:500,message:'password=EXAMPLE-PRIVATE-PASSWORD token=EXAMPLE-PRIVATE-TOKEN'})
 const wrapper=mount(OpenAITwoFAImport,{props:{importCredential:vi.fn()}})
 await start(wrapper,'a@example.com----example-password----EXAMPLEKEY')
 expect(wrapper.text()).toContain('登录任务未能启动')
 expect(wrapper.text()).not.toContain('EXAMPLE-PRIVATE')
})

it('preserves the safe terminal login stage and HTTP status', async () => {
 vi.mocked(getTwoFALogin).mockResolvedValueOnce({id:'job-1',status:'failed',stage:'security_verification',http_status:403,error_code:'security_verification_required',message:'需要人工验证'})
 const wrapper=mount(OpenAITwoFAImport,{props:{importCredential:vi.fn()}})
 await start(wrapper,'a@example.com----example-password----EXAMPLEKEY')
 expect(wrapper.text()).toContain('HTTP 403')
 expect(wrapper.text()).toContain('等待安全验证')
 expect(wrapper.text()).toContain('需要人工验证')
 expect(wrapper.text()).not.toContain('登录任务未能启动')
})

it('locks before asynchronous preparation and does not start after unmount',async()=>{
 let resolve!:()=>void
 const prepare=vi.fn(()=>new Promise<void>(r=>{resolve=r}))
 const importer=vi.fn()
 const wrapper=mount(OpenAITwoFAImport,{props:{prepare,importCredential:importer,proxyId:17}})
 await flushPromises()
 await wrapper.get('textarea').setValue('u@example.com----p----s')
 const startButton=wrapper.get('[data-testid="two-fa-start"]')
 await startButton.trigger('click')
 await startButton.trigger('click')
 expect(prepare).toHaveBeenCalledOnce()
 wrapper.unmount();resolve();await flushPromises()
 expect(startTwoFALogin).not.toHaveBeenCalled()
 expect(importer).not.toHaveBeenCalled()
})

it('validates proxy-timezone selection before starting a login',async()=>{
 const prepare=vi.fn(()=>{throw new Error('select_proxy')})
 const wrapper=mount(OpenAITwoFAImport,{props:{prepare,importCredential:vi.fn()}})
 await start(wrapper,'u@example.com----p----s')
 expect(wrapper.get('[role="alert"]').text()).toContain('先选择一个可用代理')
 expect(startTwoFALogin).not.toHaveBeenCalled()
 expect(wrapper.emitted('busy')).toEqual([[true],[false]])
})

it('shows a readable protocol failure without opening a browser and keeps the proxy for retry',async()=>{
 vi.useFakeTimers()
 const importCredential=vi.fn().mockResolvedValue('created')
 const environment={enabled:true,mode:'proxy',timezone:'Asia/Tokyo'}
 const w=mount(OpenAITwoFAImport,{props:{proxyId:17,loginEnvironment:environment,importCredential}})
 try{
  vi.mocked(getTwoFALogin).mockResolvedValue({id:'job-1',status:'waiting_interaction',stage:'security_verification',error_code:'security_verification_required',http_status:403,timezone:'America/Los_Angeles',browser_available:true,message:'需要人工验证'})
  await start(w,'u@example.com----password----SECRET')
  expect(startTwoFALogin).toHaveBeenCalledWith(expect.anything(),17)
  expect(w.text()).toContain('America/Los_Angeles');expect(w.text()).toContain('HTTP 403')
  expect(w.find('[data-testid="two-fa-browser"]').exists()).toBe(false)
  expect(importCredential).not.toHaveBeenCalled();expect(w.text()).not.toContain('SECRET')
  await w.setProps({proxyId:99,loginEnvironment:{enabled:true,mode:'manual',timezone:'UTC'}})
  vi.mocked(getTwoFALogin).mockResolvedValueOnce({id:'job-1',status:'failed',stage:'security_verification',error_code:'login_timeout'})
  await vi.advanceTimersByTimeAsync(1500);await flushPromises()
  expect(importCredential).not.toHaveBeenCalled();expect(deleteTwoFALogin).not.toHaveBeenCalled()
  vi.mocked(getTwoFALogin).mockResolvedValue({id:'job-2',status:'succeeded',credential:{access_token:'fixture'}})
  await w.get('[data-testid="two-fa-start"]').trigger('click');await flushPromises()
  expect(startTwoFALogin).toHaveBeenLastCalledWith(expect.anything(),17)
  expect(importCredential).toHaveBeenCalledOnce()
 }finally{w.unmount();vi.useRealTimers()}
})
