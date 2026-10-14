<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ admin ? '视频任务管理' : '我的视频任务' }}</h1>
          <p class="mt-2 text-sm text-gray-500">生成状态、分组计价与扣费记录集中查看。下载重试不会重新生成视频。</p>
        </div>
        <a :href="canvasUrl" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">无限画布</a>
      </div>
      <div class="card flex flex-wrap items-end gap-3 p-4">
        <label class="text-sm">开始日期<input v-model="startDate" type="date" class="input mt-1" /></label>
        <label class="text-sm">结束日期<input v-model="endDate" type="date" class="input mt-1" /></label>
        <label class="text-sm">任务状态
          <select v-model="status" class="input mt-1 min-w-36" @change="applyFilters">
            <option value="">全部状态</option>
            <option v-for="(label, value) in states" :key="value" :value="value">{{ label }}</option>
          </select>
        </label>
        <label class="text-sm">模型<input v-model="modelFilter" class="input mt-1 w-48" placeholder="全部 / 完整模型名" maxlength="200" @keyup.enter="applyFilters" /></label>
        <label class="text-sm">分组编号<input v-model="groupFilter" type="number" min="1" class="input mt-1 w-32" placeholder="全部" @keyup.enter="applyFilters" /></label>
        <label class="text-sm">密钥编号<input v-model="keyFilter" type="number" min="1" class="input mt-1 w-32" placeholder="全部" @keyup.enter="applyFilters" /></label>
        <label v-if="admin" class="text-sm">用户编号<input v-model="userFilter" type="number" min="1" class="input mt-1 w-32" placeholder="全部" @keyup.enter="applyFilters" /></label>
        <button class="btn btn-primary" :disabled="loading" @click="applyFilters">{{ loading ? '读取中…' : '查询 / 刷新' }}</button>
        <p class="w-full text-xs text-gray-500">按任务创建日期筛选，含开始与结束日期（本地时区）。默认近 30 天，可查询任意历史区间，每次最多 93 天；列表与统计使用相同筛选。</p>
      </div>
      <p v-if="error" role="alert" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-xl bg-blue-50 p-4 text-sm text-blue-700 dark:bg-blue-950/30 dark:text-blue-300">{{ notice }}</p>
      <section v-if="stats" aria-label="视频任务统计" class="space-y-3" :aria-busy="loading">
        <div class="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">
          <div v-for="card in statCards" :key="card.label" class="card p-4"><div class="text-xs text-gray-500">{{ card.label }}</div><div class="mt-2 break-words text-lg font-semibold text-gray-900 dark:text-white">{{ card.value }}</div></div>
        </div>
        <p class="text-xs leading-relaxed text-gray-500">成功率 = 已完成 ÷（已完成 + 失败），生成中与待核验不计入分母。成功秒数按任务记录的时长汇总；平均完成耗时包含排队、生成和结算。实扣仅汇总已有扣费凭据和有效用量记录的金额。</p>
        <p v-if="stats.unreconciled" class="text-sm text-amber-600">{{ stats.unreconciled }} 条任务的账单尚待核验，暂未计入实扣汇总；请查看对应任务记录。</p>
        <details class="card overflow-hidden"><summary class="cursor-pointer p-4 text-sm font-medium">按模型统计（{{ stats.models.length }}）</summary>
          <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead class="bg-gray-50 text-gray-500 dark:bg-dark-700"><tr><th class="p-3">模型</th><th class="p-3">任务</th><th class="p-3">成功 / 失败</th><th class="p-3">进行中 / 待核验</th><th class="p-3">成功率</th><th class="p-3">成功秒数</th><th class="p-3">实扣</th><th class="p-3">平均完成耗时</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-600"><tr v-for="model in stats.models" :key="model.model"><td class="max-w-64 break-words p-3">{{ model.model || '未记录模型' }}</td><td class="p-3">{{ model.total }}</td><td class="p-3">{{ model.succeeded }} / {{ model.failed }}</td><td class="p-3">{{ model.in_progress }} / {{ model.needs_review }}</td><td class="p-3">{{ percent(model.success_rate) }}</td><td class="p-3">{{ model.successful_seconds }} 秒</td><td class="p-3">{{ money(model.actual_cost) }}</td><td class="p-3">{{ duration(model.average_completion_seconds) }}</td></tr><tr v-if="!stats.models.length"><td colspan="8" class="p-6 text-center text-gray-500">当前筛选无视频任务</td></tr></tbody></table></div>
        </details>
      </section>
      <p class="text-xs leading-relaxed text-gray-500">{{ fileDownloadSupported ? '“保存到文件”可边下载边写入磁盘；本页暂停后可继续同一任务，续传取决于上游的分段与版本校验支持。' : '当前浏览器不支持流式写入文件，普通下载会先缓存完整视频再保存。' }} 普通下载仍可使用；重试只读取已有结果，不会重新生成或再次收取生成费用。</p>
      <div class="card overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="bg-gray-50 text-gray-500 dark:bg-dark-700"><tr>
            <th class="p-4">视频 / 任务</th><th class="p-4">{{ admin ? '用户 / 密钥 / 分组' : '密钥 / 分组' }}</th>
            <th class="p-4">状态</th><th class="p-4">计费与扣款</th><th class="p-4">创建时间</th><th class="p-4">操作</th>
          </tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
            <tr v-for="task in tasks" :key="task.id" class="align-top">
              <td class="max-w-72 p-4"><div class="break-words font-medium text-gray-900 dark:text-white">{{ task.model }}</div>
                <div class="mt-1 text-xs text-gray-500">{{ task.resolution || '—' }} · {{ task.seconds }} 秒</div>
                <div class="mt-2 break-all font-mono text-xs text-gray-400" :title="task.id">{{ task.task_id || '等待上游确认' }}</div>
              </td>
              <td class="p-4"><div v-if="admin" class="mb-1 text-xs text-gray-500">用户 #{{ task.user_id }}</div><div>{{ task.key_name || '已删除密钥' }} <span class="text-gray-400">#{{ task.api_key_id }}</span></div><div class="mt-1 text-xs text-gray-500">{{ task.group_name || '原分组' }}</div></td>
              <td class="p-4"><span class="rounded-full px-2.5 py-1 text-xs" :class="statusClass(task.status)">{{ states[task.status] || task.status }}</span>
                <p v-if="task.status === 'needs_review'" class="mt-2 max-w-40 text-xs text-amber-600">任务或账务需核验，请勿重复提交。</p>
                <p v-if="task.status === 'unknown'" class="mt-2 max-w-40 text-xs text-amber-600">提交结果未确认，请联系管理员核查。</p>
              </td>
              <td class="p-4"><div class="font-medium">{{ task.usage_recorded && task.actual_cost !== null ? money(task.actual_cost) : task.billed ? '账单核验中' : task.billing_state === 'refunded' ? '预扣已退回' : task.billing_state === 'refund_pending' ? '退款处理中' : task.billing_state === 'held' ? `已预扣 ${money(task.hold_amount || 0)}` : '待结算' }}</div>
                <div v-if="task.price_snapshot" class="mt-1 whitespace-nowrap text-xs text-gray-500">{{ money(task.price_snapshot.unit_price) }}/{{ task.price_snapshot.billing_mode === 'video' ? '秒' : '条' }}<span v-if="!task.price_snapshot.final_unit_price && task.price_snapshot.rate_multiplier !== 1"> × {{ task.price_snapshot.rate_multiplier }}</span></div>
                <router-link v-if="task.usage_id" class="mt-2 block text-xs text-primary-600" :to="usageLink(task)">用量记录 #{{ task.usage_id }}</router-link>
              </td>
              <td class="whitespace-nowrap p-4 text-xs text-gray-500">{{ date(task.created_at) }}</td>
              <td class="p-4"><div v-if="canOperate(task)" class="flex min-w-32 flex-col gap-2">
                <button v-if="task.task_id && task.status !== 'failed'" class="btn btn-secondary text-xs" :disabled="refreshing === task.id" @click="refreshTask(task)">{{ refreshing === task.id ? '查询中…' : '查询上游状态' }}</button>
                <button v-if="task.task_id && (task.status === 'done' || task.status === 'needs_review')" class="btn btn-primary text-xs" :disabled="!!downloading" @click="download(task)">下载视频</button>
                <button v-if="fileDownloadSupported && task.task_id && (task.status === 'done' || task.status === 'needs_review')" class="btn btn-secondary text-xs" :disabled="!!downloading" @click="saveToFile(task)">保存到文件 / 继续</button>
                <button v-if="downloading === task.id" class="text-xs text-red-500" @click="downloadController?.abort()">取消下载 {{ downloadProgress }}</button>
              </div><span v-else class="text-xs text-gray-400">由任务所属用户下载</span></td>
            </tr>
            <tr v-if="!tasks.length"><td colspan="6" class="p-12 text-center text-gray-500">{{ loading ? '正在读取视频任务…' : '暂无符合条件的视频任务' }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-between text-sm text-gray-500"><span>第 {{ page }} 页 · 下载后请保留本地副本，上游文件可能过期。</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="page === 1 || loading" @click="changePage(-1)">上一页</button><button class="btn btn-secondary" :disabled="!hasMore || loading" @click="changePage(1)">下一页</button></div></div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { getCreativeCanvasUrl } from '@/utils/creativeCanvas'
import AppLayout from '@/components/layout/AppLayout.vue'
import { downloadVideoTask, fetchVideoTaskContent, getVideoTaskStats, listVideoTasks, refreshVideoTask, type VideoTaskFilters, type VideoTaskStats, type VideoTaskSummary } from '@/api/videoTasks'

import { downloadVideoToFile, pickVideoDownloadFile, supportsVideoFileDownload, type VideoFileDownloadSession } from '@/utils/videoDownload'

const route = useRoute()
const auth = useAuthStore()
const appStore = useAppStore()
const canvasUrl = computed(() => getCreativeCanvasUrl(appStore.siteVersion))
const admin = computed(() => route.path.startsWith('/admin/'))
const states: Record<string, string> = { submitting: '提交中', unknown: '提交待核查', pending: '生成 / 结算中', done: '已完成', failed: '生成失败', needs_review: '待核验' }
const tasks = ref<VideoTaskSummary[]>([])
const stats = ref<VideoTaskStats | null>(null)
const page = ref(1), hasMore = ref(false), loading = ref(false)
const status = ref(''), keyFilter = ref(''), userFilter = ref(''), error = ref(''), notice = ref('')
const groupFilter = ref(''), modelFilter = ref('')
function localDate(value: Date) { return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}` }
const today = new Date(), firstDay = new Date(today)
firstDay.setDate(firstDay.getDate() - 29)
const startDate = ref(localDate(firstDay)), endDate = ref(localDate(today))
const refreshing = ref(''), downloading = ref(''), downloadProgress = ref('')
let listController: AbortController | undefined
let downloadController: AbortController | undefined
let generation = 0
const fileDownloadSupported = supportsVideoFileDownload()
const fileDownloads = new Map<string, VideoFileDownloadSession>()
const money = (value: number) => `$${Number(value).toLocaleString('en-US', { maximumFractionDigits: 8 })}`
const date = (value: string) => new Date(value).toLocaleString()
const percent = (value: number | null) => value === null ? '—' : `${value.toFixed(1)}%`
const duration = (value: number | null) => value === null ? '—' : value < 60 ? `${value.toFixed(1)} 秒` : `${(value / 60).toFixed(1)} 分钟`
const statCards = computed(() => stats.value ? [
  { label: '总任务', value: stats.value.total }, { label: '已完成 / 失败', value: `${stats.value.succeeded} / ${stats.value.failed}` },
  { label: '进行中 / 待核验', value: `${stats.value.in_progress} / ${stats.value.needs_review}` }, { label: '成功率', value: percent(stats.value.success_rate) },
  { label: '成功视频秒数', value: `${stats.value.successful_seconds} 秒` }, { label: '已确认实扣', value: money(stats.value.actual_cost) },
  { label: '平均完成耗时', value: duration(stats.value.average_completion_seconds) }, { label: '账单待核验', value: stats.value.unreconciled },
] : [])
function usageLink(task: VideoTaskSummary) {
  const localDate = (value: string) => {
    const at = new Date(value)
    return `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, '0')}-${String(at.getDate()).padStart(2, '0')}`
  }
  // Settlement can occur days after submission. Include both ends of the task
  // lifecycle so old receipts stay visible beyond the usage page's default day.
  return { path: admin.value ? '/admin/usage' : '/usage', query: {
    api_key_id: String(task.api_key_id),
    ...(admin.value ? { user_id: String(task.user_id) } : {}),
    start_date: localDate(task.created_at), end_date: localDate(task.updated_at),
  } }
}
const message = (err: unknown) => (err as { message?: string })?.message || '操作失败，请稍后重试。'
const canOperate = (task: VideoTaskSummary) => task.user_id === auth.user?.id
const statusClass = (state: string) => state === 'done' ? 'bg-green-50 text-green-700 dark:bg-green-950/30' : ['failed', 'needs_review', 'unknown'].includes(state) ? 'bg-amber-50 text-amber-700 dark:bg-amber-950/30' : 'bg-blue-50 text-blue-700 dark:bg-blue-950/30'

async function load() {
  const current = ++generation
  listController?.abort()
  listController = new AbortController()
  loading.value = true
  error.value = ''
  stats.value = null
  try {
    for (const raw of [keyFilter.value, groupFilter.value, ...(admin.value ? [userFilter.value] : [])]) {
      if (raw && (!Number.isSafeInteger(Number(raw)) || Number(raw) < 1)) throw new Error('请输入有效的正整数编号。')
    }
    const start = new Date(`${startDate.value}T00:00:00`), end = new Date(`${endDate.value}T00:00:00`)
    end.setDate(end.getDate() + 1)
    if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start || end.getTime() - start.getTime() > 93 * 86400000) throw new Error('请选择有效日期，单次查询范围最多 93 天。')
    if (new TextEncoder().encode(modelFilter.value.trim()).length > 200) throw new Error('模型名过长。')
    const filters: VideoTaskFilters = { status: status.value || undefined,
      api_key_id: keyFilter.value ? Number(keyFilter.value) : undefined,
      group_id: groupFilter.value ? Number(groupFilter.value) : undefined,
      model: modelFilter.value.trim() || undefined, start_at: start.toISOString(), end_at: end.toISOString(),
      user_id: admin.value && userFilter.value ? Number(userFilter.value) : undefined }
    const [data, summary] = await Promise.allSettled([
      listVideoTasks(admin.value, { ...filters, page: page.value, page_size: 20 }, listController.signal),
      getVideoTaskStats(admin.value, filters, listController.signal),
    ])
    if (current !== generation) return
    const errors: string[] = []
    if (data.status === 'fulfilled') {
      tasks.value = data.value.items
      hasMore.value = data.value.has_more
    } else {
      tasks.value = []; hasMore.value = false
      errors.push(`任务列表：${message(data.reason)}`)
    }
    if (summary.status === 'fulfilled') stats.value = summary.value
    else errors.push(`统计暂未读取成功，可缩小日期范围后重试。${message(summary.reason)}`)
    error.value = errors.join(' ')
  } catch (err) { if (!listController.signal.aborted && current === generation) { error.value = message(err); tasks.value = []; hasMore.value = false } }
  finally { if (current === generation) loading.value = false }
}
function applyFilters() { page.value = 1; void load() }
function changePage(delta: number) { page.value += delta; void load() }
async function refreshTask(task: VideoTaskSummary) {
  refreshing.value = task.id; notice.value = ''; error.value = ''
  try { await refreshVideoTask(task); notice.value = '已查询上游状态；账务信息将在任务结算后显示。'; await load() }
  catch (err) { error.value = message(err) }
  finally { refreshing.value = '' }
}
async function download(task: VideoTaskSummary) {
  downloadController = new AbortController(); downloading.value = task.id; error.value = ''; notice.value = ''; downloadProgress.value = ''
  try {
    const blob = await downloadVideoTask(task, downloadController.signal, (loaded, total) => {
      downloadProgress.value = total ? `${Math.round(loaded / total * 100)}%` : `${(loaded / 1024 / 1024).toFixed(1)} MB`
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a'); link.href = url; link.download = `kitty-video-${task.task_id.replace(/[^a-zA-Z0-9_-]/g, '_')}.mp4`
    document.body.appendChild(link); link.click(); link.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 60000)
    notice.value = '完整视频已交给浏览器下载，请查看下载列表。'
  } catch (err) { if (!downloadController.signal.aborted) error.value = message(err); else notice.value = '已取消下载，生成任务不受影响。' }
  finally { downloading.value = '' }
}
async function saveToFile(task: VideoTaskSummary) {
  if (!canOperate(task) || downloading.value) return
  const identity = `${task.user_id}:${task.api_key_id}:${task.task_id}`
  const controller = new AbortController()
  downloadController = controller; downloading.value = task.id; error.value = ''; notice.value = ''; downloadProgress.value = ''
  try {
    let session = fileDownloads.get(identity)
    if (!session || session.complete) {
      session = await pickVideoDownloadFile(identity, `kitty-video-${task.task_id.replace(/[^a-zA-Z0-9_-]/g, '_')}.mp4`)
      fileDownloads.set(identity, session)
    }
    const result = await downloadVideoToFile({ identity, session, signal: controller.signal,
      request: (headers, signal) => {
        if (!canOperate(task)) throw new Error('登录身份已变化，下载已停止。')
        return fetchVideoTaskContent(task, headers, signal)
      },
      onProgress: (state) => {
        if (!canOperate(task)) { controller.abort(); return }
        downloadProgress.value = `${state.resumed ? '续传 ' : state.restarted ? '从头下载 ' : ''}${(state.received / 1048576).toFixed(1)} MB${state.total ? ` / ${(state.total / 1048576).toFixed(1)} MB` : ''}`
      },
    })
    notice.value = `完整视频已保存为 ${result.fileName}，没有重新生成或再次扣生成费用。`
  } catch (err) {
    if (controller.signal.aborted || (err instanceof DOMException && err.name === 'AbortError')) notice.value = '下载已暂停，可在本页继续同一任务；生成任务不受影响。'
    else error.value = message(err)
  } finally { downloading.value = '' }
}
watch(() => auth.user?.id, () => { downloadController?.abort(); fileDownloads.clear() })
watch(admin, () => { tasks.value = []; page.value = 1; void load() })
onMounted(load)
onBeforeUnmount(() => { generation++; listController?.abort(); downloadController?.abort(); fileDownloads.clear() })
</script>
