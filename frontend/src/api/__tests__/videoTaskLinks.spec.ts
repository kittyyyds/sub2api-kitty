import {beforeEach,describe,expect,it,vi} from 'vitest'
const {get}=vi.hoisted(()=>({get:vi.fn()}))
vi.mock('@/api/client',()=>({apiClient:{get}}))
import {getVideoTaskDownloadURL,type VideoTaskSummary} from '../videoTasks'
const task={task_id:'task/one',api_key_id:42} as VideoTaskSummary
describe('task upstream link metadata',()=>{
 beforeEach(()=>get.mockReset())
 it('uses authenticated status and original key, without fetching content',async()=>{
  get.mockResolvedValue({data:{status:'completed',url:'https://cdn.example/video.mp4?signature=abc'}});const signal=new AbortController().signal
  expect(await getVideoTaskDownloadURL(task,signal)).toBe('https://cdn.example/video.mp4?signature=abc')
  expect(get).toHaveBeenCalledTimes(1);expect(get).toHaveBeenCalledWith('/canvas-api/v1/videos/task%2Fone',{baseURL:'/',headers:{'X-Canvas-Key-ID':'42'},timeout:90000,signal})
 })
 it('supports nested video URL for completed legacy tasks',async()=>{get.mockResolvedValue({data:{status:'done',video:{url:'https://cdn.example/old.mp4'}}});expect(await getVideoTaskDownloadURL(task)).toBe('https://cdn.example/old.mp4')})
 it.each(['pending','failed','in_progress','submission_unknown'])('does not expose links for %s',async status=>{get.mockResolvedValue({data:{status,url:'https://cdn.example/v.mp4'}});await expect(getVideoTaskDownloadURL(task)).rejects.toThrow('尚未交付')})
 it.each([undefined,'javascript:alert(1)','http://cdn.example/v.mp4','https://u:p@cdn.example/v.mp4','https://localhost/a','https://127.0.0.1/a','https://10.0.0.1/a','https://169.254.169.254/a','https://172.20.0.1/a','https://192.168.0.1/a','https://100.64.0.1/a','https://[::1]/a','https://cdn.example:8443/a','https://cdn.example/a#x'])('rejects unsafe URLs %s',async url=>{get.mockResolvedValue({data:{status:'completed',url}});await expect(getVideoTaskDownloadURL(task)).rejects.toThrow()})
 it('rejects expired links with completed status',async()=>{get.mockResolvedValue({data:{status:'completed',delivery_status:'expired',url:'https://cdn.example/v.mp4'}});await expect(getVideoTaskDownloadURL(task)).rejects.toThrow('过期')})
})
