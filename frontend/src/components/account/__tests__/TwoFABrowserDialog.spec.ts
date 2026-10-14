import {mount,flushPromises} from '@vue/test-utils'
import {afterEach,expect,it,vi} from 'vitest'
import TwoFABrowserDialog from '../TwoFABrowserDialog.vue'
const api=vi.hoisted(()=>({frame:vi.fn(),input:vi.fn(),resume:vi.fn()}))
vi.mock('@/api/admin/accountTokenGuard',()=>({getTwoFABrowserFrame:api.frame,sendTwoFABrowserInput:api.input,resumeTwoFABrowser:api.resume}))
afterEach(()=>{vi.useRealTimers();vi.restoreAllMocks()})
it('uses authenticated frame transport, maps user input and revokes images on close',async()=>{
 vi.useFakeTimers();api.frame.mockResolvedValue(new Blob(['fixture'],{type:'image/jpeg'}));api.input.mockResolvedValue(undefined);api.resume.mockResolvedValue(undefined)
 const create=vi.fn().mockReturnValueOnce('blob:fixture-one').mockReturnValue('blob:fixture-two'),revoke=vi.fn()
 vi.stubGlobal('URL',class extends URL {static createObjectURL=create;static revokeObjectURL=revoke})
 const w=mount(TwoFABrowserDialog,{props:{jobId:'public-job-id'},global:{stubs:{BaseDialog:{template:'<div><slot /></div>'}}}})
 try{
  await flushPromises();expect(api.frame).toHaveBeenCalledWith('public-job-id');expect(w.get('img').attributes('src')).toBe('blob:fixture-one')
  await w.get('input').setValue('fixture-text');await w.findAll('button')[0].trigger('click');await flushPromises()
  expect(api.input).toHaveBeenCalledWith('public-job-id',{type:'text',text:'fixture-text'});expect((w.get('input').element as HTMLInputElement).value).toBe('')
  await vi.advanceTimersByTimeAsync(1200);await flushPromises();expect(revoke).toHaveBeenCalledWith('blob:fixture-one')
  const close=w.findAll('button').find(b=>b.text()==='关闭窗口')!;await close.trigger('click');expect(w.emitted('close')).toHaveLength(1)
 }finally{w.unmount();vi.unstubAllGlobals()}
 expect(revoke).toHaveBeenCalledWith('blob:fixture-two')
})
