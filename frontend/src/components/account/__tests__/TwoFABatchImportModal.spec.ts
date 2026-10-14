import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import TwoFABatchImportModal from '../TwoFABatchImportModal.vue'
import OpenAITwoFAImport from '../OpenAITwoFAImport.vue'
import type { Group, Proxy } from '@/types'
const api=vi.hoisted(()=>({importCodexSession:vi.fn(),syncUpstreamModels:vi.fn(),getById:vi.fn(),update:vi.fn(),guard:vi.fn()}))
vi.mock('@/api/admin',()=>({adminAPI:{accounts:api}}))
vi.mock('@/api/admin/accountTokenGuardV2',()=>({createTokenGuardV2Account:api.guard}))
enableAutoUnmount(afterEach)
beforeEach(()=>{
 vi.resetAllMocks()
 api.importCodexSession.mockResolvedValue({created:1,skipped:0,failed:0,items:[{action:'created',account_id:42}]})
 api.guard.mockResolvedValue({});api.syncUpstreamModels.mockResolvedValue({models:['gpt-fixture'],warnings:[]})
 api.getById.mockResolvedValue({credentials:{access_token:'fixture'}});api.update.mockResolvedValue({})
})
function fixture(){return mount(TwoFABatchImportModal,{props:{proxies:[{id:7,name:'Fixture LA',status:'active'} as Proxy],groups:[{id:9,name:'Fixture Group',status:'active',platform:'openai'} as Group]},global:{stubs:{BaseDialog:{template:'<div><slot /></div>'},OpenAIRequestTimezoneField:true,OpenAITwoFAImport:true}}})}
const credential={access_token:'fixture'}
const login={email:'fixture@example.invalid',password:'fixture-password',mfa_secret:'fixture-totp'}
it('freezes proxy, group, websocket and fingerprint settings then synchronizes models',async()=>{
 const w=fixture(),selects=w.findAll('select')
 await selects[0].setValue('7');await selects[1].setValue('ctx_pool');await selects[2].setValue('full')
 await w.findAll('input[type="checkbox"]')[1].setValue(true)
 const importer=w.getComponent(OpenAITwoFAImport);await importer.props('prepare')?.();await flushPromises()
 expect(w.get('fieldset').attributes('disabled')).toBeDefined()
 await expect(importer.props('importCredential')(credential,login.email,login)).resolves.toBe('created')
 expect(api.importCodexSession).toHaveBeenCalledWith(expect.objectContaining({proxy_id:7,group_ids:[9],skip_default_group_bind:true,skip_existing:true,update_existing:false,extra:expect.objectContaining({openai_oauth_responses_websockets_v2_mode:'ctx_pool',codex_fingerprint_mode:'full'})}))
 expect(api.guard).toHaveBeenCalledWith(expect.objectContaining({account_id:42,engine:'local_worker',proxy_source:'account'}))
 expect(api.update).toHaveBeenCalledWith(42,{credentials:{access_token:'fixture',model_mapping:{'gpt-fixture':'gpt-fixture'}}})
})
it('never enrolls or modifies existing skipped accounts',async()=>{
 api.importCodexSession.mockResolvedValue({created:0,skipped:1,failed:0,items:[{action:'skipped',account_id:42}]})
 const w=fixture(),importer=w.getComponent(OpenAITwoFAImport);await importer.props('prepare')?.()
 await expect(importer.props('importCredential')(credential,login.email,login)).resolves.toBe('skipped')
 expect(api.guard).not.toHaveBeenCalled();expect(api.update).not.toHaveBeenCalled();expect(api.syncUpstreamModels).not.toHaveBeenCalled()
})
it('retries partial import without creating a duplicate or logging in again',async()=>{
 api.guard.mockRejectedValueOnce(new Error('fixture failure'))
 const w=fixture(),importer=w.getComponent(OpenAITwoFAImport);await importer.props('prepare')?.()
 await expect(importer.props('importCredential')(credential,login.email,login)).rejects.toThrow()
 await expect(importer.props('importCredential')(credential,login.email,login)).resolves.toBe('created')
 expect(api.importCodexSession).toHaveBeenCalledOnce();expect(api.guard).toHaveBeenCalledTimes(2)
})
