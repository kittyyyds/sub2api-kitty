import { apiClient } from './client'

export interface VideoTaskSummary {
	 billing_state?: string
	 hold_amount?: number
  id: string
  task_id: string
  user_id: number
  api_key_id: number
  group_id: number
  account_id?: number
  key_name: string
  group_name: string
  model: string
  status: string
  resolution: string
  seconds: number
  price_snapshot?: { billing_mode: string; unit_price: number; rate_multiplier: number; final_unit_price?: boolean; duration_seconds: number; prepaid_task_id?: string; reference_seconds?: number; reference_per_second?: number }
  usage_id: number | null
  actual_cost: number | null
  billed: boolean
  usage_recorded: boolean
  created_at: string
  updated_at: string
}

export interface VideoTaskFilters {
  status?: string
  user_id?: number
  api_key_id?: number
  group_id?: number
  model?: string
  start_at?: string
  end_at?: string
}

export interface VideoTaskMetrics {
  total: number
  succeeded: number
  failed: number
  in_progress: number
  needs_review: number
  success_rate: number | null
  successful_seconds: number
  actual_cost: number
  unreconciled: number
  average_completion_seconds: number | null
}

export interface VideoTaskStats extends VideoTaskMetrics {
  models: (VideoTaskMetrics & { model: string })[]
  start_at: string
  end_at: string
}

export async function getVideoTaskStats(admin: boolean, params: VideoTaskFilters, signal?: AbortSignal) {
  const { data } = await apiClient.get<VideoTaskStats>(admin ? '/admin/video-tasks/stats' : '/video-tasks/stats', { params, signal, timeout: 12000 })
  return data
}

export async function listVideoTasks(admin: boolean, params: VideoTaskFilters & { page: number; page_size: number }, signal?: AbortSignal) {
  const { data } = await apiClient.get<{ items: VideoTaskSummary[]; has_more: boolean }>(admin ? '/admin/video-tasks' : '/video-tasks', { params, signal })
  return data
}

export async function refreshVideoTask(task: VideoTaskSummary) {
  const { data } = await apiClient.get(`/canvas-api/v1/videos/${encodeURIComponent(task.task_id)}`, {
    baseURL: '/', headers: { 'X-Canvas-Key-ID': String(task.api_key_id) }, timeout: 90000,
  })
  return data
}

export async function downloadVideoTask(task: VideoTaskSummary, signal: AbortSignal, onProgress: (loaded: number, total?: number) => void): Promise<Blob> {
  if (task.price_snapshot?.prepaid_task_id) {
    const controller = new AbortController()
    const abort = () => controller.abort(signal.reason)
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) abort()
    let timer = setTimeout(() => controller.abort(new Error('下载读取超时，请重试已有任务')), 60_000)
    try {
    const response = await fetchVideoTaskContent(task,{},controller.signal)
    if (!response.ok) throw new Error(`上游视频下载失败（${response.status}）`)
    const type = response.headers.get('content-type') || 'video/mp4'
    if (/json|text\//i.test(type)) throw new Error('上游未返回视频文件')
    if (response.status === 206) throw new Error('上游只返回部分内容，请使用保存到文件续传')
    const total = Number(response.headers.get('content-length')) || undefined
    if (total && total > 512 * 1048576) { await response.body?.cancel(); throw new Error('视频超过512MB，请使用保存到文件') }
    const reader = response.body?.getReader(); if (!reader) throw new Error('无法读取视频流')
    const chunks: Uint8Array[] = []; let loaded = 0
    try {
      for (;;) { const {done,value}=await reader.read(); clearTimeout(timer); if(done) break; timer=setTimeout(()=>controller.abort(new Error('下载读取超时，请重试已有任务')),60_000); loaded+=value.byteLength; if(loaded>512*1048576) throw new Error('视频超过512MB，请使用保存到文件'); chunks.push(new Uint8Array(value)); onProgress(loaded,total) }
    } catch(e) { await reader.cancel().catch(()=>{}); throw e }
    if (!loaded || (total && loaded !== total)) throw new Error('视频下载不完整，请重试已有任务')
    return new Blob(chunks,{type})
    } finally { clearTimeout(timer); signal.removeEventListener('abort', abort) }
  }
  const response = await apiClient.get<Blob>(`/canvas-api/v1/videos/${encodeURIComponent(task.task_id)}/content`, {
    baseURL: '/', headers: { 'X-Canvas-Key-ID': String(task.api_key_id) },
    responseType: 'blob', timeout: 15 * 60 * 1000, signal,
    onDownloadProgress: (event) => onProgress(event.loaded, event.total),
  })
  const { data } = response
  if (response.status === 206) {
    const range = String(response.headers['content-range'] || '').match(/^bytes 0-(\d+)\/(\d+)$/)
    if (!range || Number(range[1]) + 1 !== Number(range[2])) throw new Error('视频接口只返回部分内容，请重新下载完整文件。')
  }
  const expected = Number(response.headers?.['content-length'])
  if (expected > 0 && !response.headers?.['content-encoding'] && data.size !== expected) {
    throw new Error('视频下载不完整，请重新下载已有任务。')
  }
  if (!(data instanceof Blob) || data.size === 0 || /json|text\/|html/i.test(data.type)) {
    throw new Error('未取得完整视频文件，请稍后重试下载。')
  }
  return data
}

/** Native streaming response for optional file saving; reads an existing task only. */
export async function fetchVideoTaskContent(task: VideoTaskSummary, rangeHeaders: Record<string, string>, signal: AbortSignal): Promise<Response> {
  const token = localStorage.getItem('auth_token')
  if (!token) throw new Error('请先登录，再下载已有视频任务。')
  if (task.price_snapshot?.prepaid_task_id) {
    const status = await fetch(`/canvas-api/v1/videos/${encodeURIComponent(task.task_id)}`, { headers: { Authorization: `Bearer ${token}`, 'X-Canvas-Key-ID': String(task.api_key_id) },signal,cache:'no-store' })
    const data = await status.json()
    if (!status.ok) throw new Error(data.error?.message || '读取视频交付状态失败')
    if (data.delivery_status === 'expired') throw new Error('上游视频已过期')
    if (data.delivery_mode !== 'upstream_direct' || data.status !== 'completed' || typeof data.url !== 'string') throw new Error('上游暂未提供下载链接，请稍后查询')
    const url = new URL(data.url)
    if(url.protocol !== 'https:' || url.username || url.password) throw new Error('上游下载链接无效')
    const headers: Record<string,string> = {}
    for(const [k,v] of Object.entries(rangeHeaders)) if(/^(range|if-range)$/i.test(k)) headers[k]=v
    return fetch(url.href,{headers,signal,cache:'no-store',credentials:'omit',referrerPolicy:'no-referrer'})
  }
  return fetch(`/canvas-api/v1/videos/${encodeURIComponent(task.task_id)}/content`, {
    headers: { ...rangeHeaders, Authorization: `Bearer ${token}`, 'X-Canvas-Key-ID': String(task.api_key_id) },
    signal, cache: 'no-store',
  })
}
