import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import VideoTasksView from '../VideoTasksView.vue'
const mocks = vi.hoisted(() => ({ list:vi.fn(), stats:vi.fn(), link:vi.fn(), auth:null as any }))
vi.mock('vue-router',()=>({ useRoute:()=>({path:'/video-tasks'}) }))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>mocks.auth}))
vi.mock('@/stores/app',()=>({useAppStore:()=>({siteVersion:'test'})}))
vi.mock('@/api/videoTasks',()=>({listVideoTasks:mocks.list,getVideoTaskStats:mocks.stats,getVideoTaskDownloadURL:mocks.link,refreshVideoTask:vi.fn()}))
const task={id:'local-1',task_id:'task-1',user_id:2,api_key_id:166,model:'video',status:'done',seconds:5,actual_cost:1,created_at:'2026-10-26T00:00:00Z',updated_at:'2026-10-26T00:00:00Z'}
const setup=()=>mount(VideoTasksView,{global:{stubs:{AppLayout:{template:'<div><slot /></div>'},RouterLink:true}}})
const buttons=(w:ReturnType<typeof setup>)=>w.findAll('button').filter(b=>b.text()==='获取上游下载链接')
describe('task-specific upstream downloads',()=>{
 beforeEach(()=>{vi.clearAllMocks();mocks.auth=reactive({user:{id:2}});mocks.list.mockResolvedValue({items:[task],has_more:false});mocks.stats.mockResolvedValue(null);mocks.link.mockResolvedValue('https://cdn.example/task-1.mp4?sig=fixture')})
 it('only prepares the exact task link without site credentials or referrer',async()=>{
  const w=setup();await flushPromises();expect(mocks.link).not.toHaveBeenCalled()
  await buttons(w)[0]!.trigger('click');await flushPromises()
  expect(mocks.link).toHaveBeenCalledWith(expect.objectContaining({task_id:'task-1',api_key_id:166}),expect.any(AbortSignal))
  expect(w.find('a[href="https://cdn.example/task-1.mp4?sig=fixture"]').attributes()).toMatchObject({target:'_blank',rel:'noopener noreferrer',referrerpolicy:'no-referrer'})
  expect(w.text()).not.toContain('保存到文件 / 继续');expect(w.text()).toContain('视频文件不经本站服务器中转');w.unmount()
 })
 it('keeps different tasks mapped to their own returned URLs',async()=>{
  mocks.list.mockResolvedValue({items:[task,{...task,id:'local-2',task_id:'task-2',api_key_id:199}],has_more:false})
  mocks.link.mockImplementation(async(t:any)=>'https://cdn.example/'+t.task_id+'.mp4')
  const w=setup();await flushPromises();await buttons(w)[0]!.trigger('click');await flushPromises();await buttons(w)[1]!.trigger('click');await flushPromises()
  const rows=w.findAll('tbody tr');expect(rows[0]!.find('a[target="_blank"]').attributes('href')).toBe('https://cdn.example/task-1.mp4')
  expect(rows[1]!.find('a[target="_blank"]').attributes('href')).toBe('https://cdn.example/task-2.mp4');w.unmount()
 })
 it('clears old links on expired results without relay fallback',async()=>{
  const w=setup();await flushPromises();await buttons(w)[0]!.trigger('click');await flushPromises();mocks.link.mockRejectedValueOnce(new Error('上游视频文件已过期'))
  await buttons(w)[0]!.trigger('click');await flushPromises();expect(w.find('a[href^="https://cdn.example"]').exists()).toBe(false);expect(w.text()).toContain('上游视频文件已过期');w.unmount()
 })
 it('does not expose another owner task or stale responses after identity changes',async()=>{
  let resolve!:(x:string)=>void;mocks.link.mockImplementationOnce(()=>new Promise(r=>{resolve=r}))
  const w=setup();await flushPromises();await buttons(w)[0]!.trigger('click');mocks.auth.user.id=9;await flushPromises();resolve('https://cdn.example/private.mp4');await flushPromises()
  expect(w.text()).toContain('由任务所属用户下载');expect(w.find('a[href^="https://cdn.example"]').exists()).toBe(false);w.unmount()
 })
 it('aborts pending link lookup on unmount',async()=>{
  mocks.link.mockImplementation(()=>new Promise(()=>{}));const w=setup();await flushPromises();await buttons(w)[0]!.trigger('click');const signal=mocks.link.mock.calls[0]![1];w.unmount();expect(signal.aborted).toBe(true)
 })
})
