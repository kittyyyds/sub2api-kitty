import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import Wizard from '../OpenAIVMManagedWizard.vue'
import { parseVMLogins, proxyUnusableReason, validateVMLogin, vmAccountName } from '../vmManagedWizard'
import type { Proxy } from '@/types'

const api = vi.hoisted(()=>({batchPreview:vi.fn(),batchStart:vi.fn(),batchStatus:vi.fn(),batchCancel:vi.fn()}))
vi.mock('@/api/admin/openaiVmManaged',()=>({openaiVmManagedAPI:api}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({user:{id:7}})}))
vi.mock('@/composables/useStepUp',()=>({useStepUp:()=>({run:(fn:()=>Promise<unknown>)=>fn()}),isStepUpCancelled:()=>false}))
vi.mock('vue-i18n',async importOriginal=>({...await importOriginal<typeof import('vue-i18n')>(),useI18n:()=>({t:(key:string,params?:unknown)=>key+(params?JSON.stringify(params):''),te:()=>true})}))
const secret='JBSWY3DPEHPK3PXP'
const proxies=[{id:1,name:'A',protocol:'socks5',status:'active'},{id:2,name:'B',protocol:'socks5h',status:'active'},{id:3,name:'HTTP',protocol:'http',status:'active'},{id:4,name:'Off',protocol:'socks5',status:'inactive'}] as Proxy[]
const settings={group_ids:[6],concurrency:7,priority:2,rate_multiplier:0.5,group_rate_multiplier:0.6,extra:{cost_multiplier:0.7},credentials:{model_mapping:{client:'upstream'}}}
const wrappers: ReturnType<typeof mount>[]=[]
function setup(){const wrapper=mount(Wizard,{props:{show:true,namePrefix:'Pool',settings,proxies},global:{stubs:{BaseDialog:{props:['show'],template:'<div v-if="show"><slot/><slot name="footer"/></div>'},TotpStepUpDialog:true,OpenAIVMManagedModal:true}}});wrappers.push(wrapper);return wrapper}
async function loginStep(wrapper:ReturnType<typeof setup>,n=2){await wrapper.get('[data-testid="vm-count"]').setValue(n);await wrapper.get('[data-testid="vm-proxy-1"]').setValue(true);await wrapper.get('[data-testid="vm-next"]').trigger('click')}
async function paste(wrapper:ReturnType<typeof setup>,text:string){await wrapper.get('[data-testid="vm-method-batch"]').trigger('click');await wrapper.get('[data-testid="vm-batch-text"]').setValue(text);await wrapper.get('[data-testid="vm-parse"]').trigger('click')}
beforeEach(()=>{localStorage.clear();Object.values(api).forEach(fn=>fn.mockReset());api.batchPreview.mockResolvedValue({importable:2,problems:[],lines:[{line_no:1,name:'Pool-01',email:'a@example.test',proxy_id:1},{line_no:2,name:'Pool-02',email:'b@example.test',proxy_id:1}],plan_token:'test-plan'})})
afterEach(()=>{wrappers.splice(0).forEach(w=>w.unmount());vi.useRealTimers()})

describe('VM2 login parsing',()=>{
  it.each(['---','----'])('preserves all characters inside passwords with %s delimiters',sep=>{const [r]=parseVMLogins('a@example.test'+sep+'p----and---dash'+sep+secret);expect(r.password).toBe('p----and---dash');expect(r.problem).toBe('')})
  it('uses separate fields without changing leading password dashes',()=>{expect(validateVMLogin({email:'a@example.test',password:'----pw---',mfa_secret:secret})).toBe('')})
  it('rejects temporary codes and malformed rows without echoing their content',()=>{expect(parseVMLogins('a@example.test----pw----123456')[0].problem).toBe('invalid_mfa_secret');expect(parseVMLogins('secret-malformed')[0].email).toBe('')})
})
describe('VM2 creation wizard',()=>{
  it('automatically parses pasted rows into separate editable fields',async()=>{
    const w=setup();await loginStep(w,1);await w.get('[data-testid="vm-method-batch"]').trigger('click')
    await w.get('[data-testid="vm-batch-text"]').trigger('paste',{clipboardData:{getData:()=>`a@example.test----pw----${secret}`}})
    expect((w.get('input[type="email"]').element as HTMLInputElement).value).toBe('a@example.test')
    expect((w.get('[data-testid="vm-batch-text"]').element as HTMLTextAreaElement).value).toBe('')
  })
  it('clears unsubmitted secrets and resets review when closed then reopened',async()=>{
    const w=setup();await loginStep(w);await paste(w,`a@example.test----pw----${secret}\nb@example.test----pw----${secret}`)
    await w.get('[data-testid="vm-preview"]').trigger('click');await flushPromises()
    await w.setProps({show:false});await w.setProps({show:true})
    expect(w.find('[data-testid="vm-count"]').exists()).toBe(true)
    expect(w.find('[data-testid="vm-start"]').exists()).toBe(false)
    await w.get('[data-testid="vm-next"]').trigger('click')
    expect(w.findAll('input[type="email"]')).toHaveLength(0)
  })
  it('retains all three original login modes and adds bulk 2FA, selected by default',async()=>{const w=setup();await loginStep(w);for(const mode of ['password','oauth','file','batch'])expect(w.find(`[data-testid="vm-method-${mode}"]`).exists()).toBe(true);expect(w.get('[data-testid="vm-method-batch"]').attributes('aria-selected')).toBe('true')})
  it('blocks insufficient, extra and duplicate selected rows',async()=>{const w=setup();await loginStep(w);await paste(w,`a@example.test----pw----${secret}`);expect(w.get('[data-testid="vm-preview"]').attributes('disabled')).toBeDefined();await paste(w,`a@example.test----pw----${secret}\nA@example.test----pw----${secret}`);expect(w.get('[data-testid="vm-preview"]').attributes('disabled')).toBeDefined();await paste(w,['a','b','c'].map(x=>`${x}@example.test----pw----${secret}`).join('\n'));expect(w.get('[data-testid="vm-preview"]').attributes('disabled')).toBeDefined();await w.findAll('input[type="checkbox"]')[2].setValue(false);expect(w.get('[data-testid="vm-preview"]').attributes('disabled')).toBeUndefined()})
  it('passes shared settings and parsed rows through preview and start',async()=>{
    const w=setup();await loginStep(w);await paste(w,`a@example.test----pw----${secret}\nb@example.test----pw----${secret}`)
    await w.get('[data-testid="vm-preview"]').trigger('click');await flushPromises()
    expect(api.batchPreview).toHaveBeenCalledWith(expect.objectContaining({count:2,settings,proxy_ids:[1],login_mode:'batch_2fa',entries:expect.arrayContaining([expect.objectContaining({email:'a@example.test',password:'pw'})])}))
    api.batchStart.mockResolvedValue({id:19,status:'finished',total:2,items:[]})
    await w.get('[data-testid="vm-start"]').trigger('click');await flushPromises()
    expect(api.batchStart).toHaveBeenCalledWith(expect.objectContaining({plan_token:'test-plan'}));expect(localStorage.getItem('vm-managed-last-batch:7')).toBe('19');expect(JSON.stringify(localStorage)).not.toContain(secret)
  })
  it.each(['password','oauth','file'])('creates awaiting-login slots for existing %s mode without requiring 2FA rows',async mode=>{const w=setup();await loginStep(w);await w.get(`[data-testid="vm-method-${mode}"]`).trigger('click');await w.get('[data-testid="vm-preview"]').trigger('click');await flushPromises();expect(api.batchPreview).toHaveBeenCalledWith(expect.objectContaining({login_mode:'manual',entries:[],count:2}))})
  it('does not allow a short-capacity preview to start a partial batch',async()=>{api.batchPreview.mockResolvedValue({importable:1,problems:[{line_no:2,code:'no_proxy_available'}],lines:[]});const w=setup();await loginStep(w);await w.get('[data-testid="vm-method-password"]').trigger('click');await w.get('[data-testid="vm-preview"]').trigger('click');await flushPromises();expect(w.get('[data-testid="vm-start"]').attributes('disabled')).toBeDefined()})
})

describe('VM2 wizard naming and proxy list',()=>{
  it('names one account exactly as the prefix and numbers a batch',()=>{expect(vmAccountName(' Pool ',1,1)).toBe('Pool');expect(vmAccountName('Pool',2,3)).toBe('Pool-02');expect(vmAccountName('',1,1)).toBe('')})
  it('explains why a proxy cannot carry a managed slot',()=>{
    expect(proxyUnusableReason({protocol:'socks5',status:'active',expires_at:null})).toBeNull()
    expect(proxyUnusableReason({protocol:'http',status:'active',expires_at:null})).toBe('protocol')
    expect(proxyUnusableReason({protocol:'socks5',status:'inactive',expires_at:null})).toBe('inactive')
    expect(proxyUnusableReason({protocol:'socks5h',status:'active',expires_at:'2000-01-01T00:00:00Z'})).toBe('expired')
  })
  it('lists HTTP and disabled proxies apart with their reason, and never lets them be picked',async()=>{
    const w=setup()
    expect(w.find('[data-testid="vm-proxy-3"]').exists()).toBe(false)
    expect(w.find('[data-testid="vm-proxy-4"]').exists()).toBe(false)
    const unusable=w.get('[data-testid="vm-unusable-proxies"]').text()
    expect(unusable).toContain('HTTP');expect(unusable).toContain('unusable.protocol');expect(unusable).toContain('Off');expect(unusable).toContain('unusable.inactive')
  })
  it('shows the single account name without a number and step 2 as current',async()=>{
    const w=setup()
    expect(w.get('[data-testid="vm-settings-summary"]').text()).toContain('Pool')
    expect(w.get('[data-testid="vm-settings-summary"]').text()).not.toContain('Pool-01')
    await w.get('[data-testid="vm-count-plus"]').trigger('click')
    expect(w.get('[data-testid="vm-settings-summary"]').text()).toContain('Pool-01 … Pool-02')
    expect(w.get('[data-testid="vm-steps"] [aria-current="step"]').attributes('data-state')).toBe('current')
    expect(w.findAll('[data-testid="vm-steps"] li[data-state="done"]')).toHaveLength(1)
  })
  it('switching to multiple proxies keeps the proxy already chosen',async()=>{
    const w=setup();await w.get('[data-testid="vm-proxy-2"]').setValue(true);await w.get('[data-testid="vm-proxy-mode-multiple"]').trigger('click')
    expect((w.get('[data-testid="vm-proxy-2"]').element as HTMLInputElement).checked).toBe(true)
    expect(w.get('[data-testid="vm-next"]').attributes('disabled')).toBeUndefined()
  })
})
