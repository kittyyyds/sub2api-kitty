import {mount,flushPromises} from '@vue/test-utils'
import {beforeEach,describe,expect,it,vi} from 'vitest'
import {createI18n} from 'vue-i18n'
import Modal from '../AccountIntelligenceScheduleModal.vue'
import {accountIntelligence,type AccountIntelligenceSchedule} from '@/api/admin/accountIntelligence'
import type {AccountListItem} from '@/types'
vi.mock('@/api/admin/accountIntelligence',()=>({accountIntelligence:{schedule:vi.fn(),saveSchedule:vi.fn()}}))
const plan:AccountIntelligenceSchedule={account_id:27,revision:0,enabled:false,kind:'candy',model:'gpt-6-astra',reasoning_effort:'low',prompt:'',cron_expression:'*/30 * * * *',max_results:12,timezone:'Asia/Shanghai',next_run_at:null,last_attempt_at:null,last_error:''}
const account={id:27,name:'fixed account',platform:'openai',credentials:{}} as AccountListItem
const options={props:{account},global:{stubs:{BaseDialog:{template:'<div><slot/></div>'}},plugins:[createI18n({legacy:false,locale:'en',messages:{en:{}},missingWarn:false,fallbackWarn:false})]}}
describe('automatic account intelligence schedule',()=>{
 beforeEach(()=>{vi.resetAllMocks();vi.mocked(accountIntelligence.schedule).mockResolvedValue({...plan});vi.mocked(accountIntelligence.saveSchedule).mockImplementation(async(id,p)=>({...p,revision:p.revision+1,next_run_at:p.enabled?'2026-11-16T12:30:00Z':null}))})
 it('does not start testing or save on open; explicit save uses the selected account, model and full Cron',async()=>{
  const w=mount(Modal,options);await flushPromises()
  expect(accountIntelligence.saveSchedule).not.toHaveBeenCalled()
  await w.get('[data-testid=schedule-enabled]').setValue(true)
  await w.get('form').trigger('submit');await flushPromises()
  expect(accountIntelligence.saveSchedule).toHaveBeenCalledWith(27,expect.objectContaining({revision:0,enabled:true,model:'gpt-6-astra',reasoning_effort:'low',cron_expression:'*/30 * * * *',max_results:12}))
  expect(w.text()).toContain('2026');expect(w.emitted('updated')).toHaveLength(1)
  await w.get('[data-testid=schedule-enabled]').setValue(false);await w.get('form').trigger('submit');await flushPromises()
  expect(vi.mocked(accountIntelligence.saveSchedule).mock.calls[1][1]).toMatchObject({revision:1,enabled:false})
  w.unmount()
 })
 it('rejects the screenshot Cron with Chinese explanation without sending a request',async()=>{
  const w=mount(Modal,options);await flushPromises()
  await w.get('[data-testid=schedule-period]').setValue('custom')
  await w.get('[data-testid=schedule-cron]').setValue('*/30:每30分钟运行一次')
  await w.get('form').trigger('submit');await flushPromises()
  expect(accountIntelligence.saveSchedule).not.toHaveBeenCalled();expect(w.get('[role=alert]').text()).toContain('invalidCron');w.unmount()
 })
 it('saves the one-minute preset only after explicit confirmation',async()=>{
  const w=mount(Modal,options);await flushPromises()
  expect(w.get<HTMLSelectElement>('[data-testid=schedule-period]').element.value).toBe('*/30 * * * *')
  await w.get('[data-testid=schedule-period]').setValue('* * * * *')
  await w.get('[data-testid=schedule-enabled]').setValue(true)
  expect(accountIntelligence.saveSchedule).not.toHaveBeenCalled()
  await w.get('form').trigger('submit');await flushPromises()
  expect(accountIntelligence.saveSchedule).toHaveBeenCalledWith(27,expect.objectContaining({enabled:true,cron_expression:'* * * * *'}))
  w.unmount()
 })
 it('loads a saved one-minute plan as a preset and preserves it when saving',async()=>{
  vi.mocked(accountIntelligence.schedule).mockResolvedValue({...plan,revision:3,enabled:true,cron_expression:'* * * * *'})
  const w=mount(Modal,options);await flushPromises()
  expect(w.get<HTMLSelectElement>('[data-testid=schedule-period]').element.value).toBe('* * * * *')
  expect(w.find('[data-testid=schedule-cron]').exists()).toBe(false)
  await w.get('form').trigger('submit');await flushPromises()
  expect(accountIntelligence.saveSchedule).toHaveBeenCalledWith(27,expect.objectContaining({revision:3,enabled:true,cron_expression:'* * * * *'}))
  w.unmount()
 })
 it('keeps a server error visible and never claims a failed save succeeded',async()=>{
  vi.mocked(accountIntelligence.saveSchedule).mockRejectedValue({response:{data:{message:'Schedule conflict: reload before saving'}}})
  const w=mount(Modal,options);await flushPromises();await w.get('form').trigger('submit');await flushPromises()
  expect(w.get('[role=alert]').text()).toContain('Schedule conflict');expect(w.emitted('updated')).toBeUndefined();w.unmount()
 })
 it('does not save after a failed load or accept a late response after leaving',async()=>{
  let resolve!:(p:AccountIntelligenceSchedule)=>void
  vi.mocked(accountIntelligence.schedule).mockImplementation(()=>new Promise(r=>{resolve=r}))
  const w=mount(Modal,options);await w.get('form').trigger('submit');expect(accountIntelligence.saveSchedule).not.toHaveBeenCalled();w.unmount();resolve({...plan});await flushPromises();expect(w.emitted('updated')).toBeUndefined()
 })
})
