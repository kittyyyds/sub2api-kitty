<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { videoGenerationAPI, buildVideoContentURL } from '@/api/videoGeneration'
import { uploadVideoReference, validateVideoReference } from '@/api/videoReferences'
import { downloadVideoToFile, pickVideoDownloadFile, supportsVideoFileDownload, type VideoFileDownloadSession } from '@/utils/videoDownload'
import { buildSuanliVideoRequest, splitReferenceURLs, SUANLI_VIDEO_MODELS, SUANLI_VIDEO_RATIOS, type SuanliVideoRatio } from '../../suanliVideo'

const props = defineProps<{ apiKey: string; modelIds: string[]; disabled: boolean }>()
const emit = defineEmits<{ busy: [value: boolean] }>()
const availableModels = computed(() => SUANLI_VIDEO_MODELS.filter((model) => props.modelIds.includes(model.id)))
const form = reactive({ modelID: '', prompt: '', ratio: '16:9' as SuanliVideoRatio, duration: 5, images: '', audio: '' })
const imageFiles = ref<File[]>([])
const audioFiles = ref<File[]>([])
const fileProgress = ref('')
let fileSession: VideoFileDownloadSession | null = null
let keyEpoch = 0
let diskDownloadActive = false
const diskDownloadSupported = supportsVideoFileDownload()
const model = computed(() => availableModels.value.find((item) => item.id === form.modelID))
const activity = ref<'idle' | 'submitting' | 'querying' | 'downloading'>('idle')
const busy = computed(() => activity.value !== 'idle')
const taskID = ref('')
const status = ref('')
const error = ref('')
const downloadFailed = ref(false)
const videoURL = ref('')
const completed = computed(() => ['done', 'completed', 'succeeded', 'success'].includes(status.value.toLowerCase()))
let controller: AbortController | null = null
let assigningSubmittedID = false

function clearPreview(): void {
  if (videoURL.value) URL.revokeObjectURL(videoURL.value)
  videoURL.value = ''
}

function stopQuerying(): void {
  const active = controller
  // Keep the operation busy until the partial file has been closed on disk.
  if (diskDownloadActive) { active?.abort(); return }
  controller = null
  active?.abort()
  activity.value = 'idle'
}

function resetResult(): void {
  stopQuerying()
  status.value = ''
  error.value = ''
  downloadFailed.value = false
  clearPreview()
}

watch(busy, (value) => emit('busy', value))
watch(() => props.apiKey, () => {
  keyEpoch++
  fileSession = null
  fileProgress.value = ''
  resetResult()
  taskID.value = ''
}, { flush: 'sync' })
watch(taskID, () => {
  fileSession = null
  fileProgress.value = ''
  if (!assigningSubmittedID) resetResult()
}, { flush: 'sync' })
watch(availableModels, (models) => {
  if (!models.some((item) => item.id === form.modelID)) form.modelID = models[0]?.id || ''
}, { immediate: true })
watch(() => form.modelID, () => {
  imageFiles.value = []
  audioFiles.value = []
  form.images = ''
  form.audio = ''
  form.duration = model.value?.family === 'sd25' ? 30 : 5
}, { immediate: true })

function selectFiles(event: Event, kind: 'image' | 'audio'): void {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  try {
    const max = kind === 'image' ? model.value?.maxImages || 1 : model.value?.maxAudio || 0
    if (files.length > max) throw new Error(`当前型号最多支持 ${max} 个此类素材。`)
    files.forEach((file) => validateVideoReference(file, kind))
    if (kind === 'image') imageFiles.value = files
    else audioFiles.value = files
    error.value = ''
  } catch (caught) { error.value = requestError(caught, props.apiKey) }
}

function abortError(): DOMException {
  return new DOMException('Aborted', 'AbortError')
}

function delay(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.reject(abortError())
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer)
      reject(abortError())
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, 12000)
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

// End the UI wait even if an aborted network request settles late. Reclaim any late Blob URL.
function timedRequest<T>(
  signal: AbortSignal,
  timeoutMs: number,
  timeoutMessage: string,
  request: (signal: AbortSignal) => Promise<T>,
  discard?: (value: T) => void,
): Promise<T> {
  if (signal.aborted) return Promise.reject(abortError())
  return new Promise((resolve, reject) => {
    const active = new AbortController()
    let settled = false
    const cleanup = () => {
      clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
    }
    const fail = (reason: unknown) => {
      if (settled) return
      settled = true
      cleanup()
      reject(reason)
    }
    const onAbort = () => {
      fail(abortError())
      active.abort()
    }
    const timer = setTimeout(() => {
      fail(new Error(timeoutMessage))
      active.abort()
    }, timeoutMs)
    signal.addEventListener('abort', onAbort, { once: true })
    Promise.resolve().then(() => {
      if (active.signal.aborted) throw abortError()
      return request(active.signal)
    }).then((value) => {
      if (settled) {
        discard?.(value)
        return
      }
      settled = true
      cleanup()
      resolve(value)
    }, fail)
  })
}

function isCurrent(active: AbortController, apiKey: string, id?: string): boolean {
  return controller === active && !active.signal.aborted && props.apiKey === apiKey
    && (id === undefined || taskID.value.trim() === id)
}

function requestError(caught: unknown, apiKey: string): string {
  const message = caught instanceof Error ? caught.message : '视频请求失败。'
  return apiKey ? message.split(apiKey).join('[已隐藏]') : message
}

async function poll(apiKey: string, id: string, active: AbortController): Promise<void> {
  const startedAt = Date.now()
  while (isCurrent(active, apiKey, id)) {
    const result = await timedRequest(active.signal, 30000, '状态查询超时，请点击“继续查询”重试。',
      (signal) => videoGenerationAPI.status(apiKey, id, signal))
    if (!isCurrent(active, apiKey, id)) return
    status.value = result.status
    if (completed.value) return
    if (['failed', 'error', 'expired', 'cancelled', 'canceled'].includes(result.status.toLowerCase())) {
      throw new Error(result.errorMessage || '视频生成失败。')
    }
    if (Date.now() - startedAt > 20 * 60 * 1000) {
      status.value = '生成中；可稍后继续查询'
      return
    }
    await delay(active.signal)
  }
}

async function run(resume = false): Promise<void> {
  if (busy.value || props.disabled || !props.apiKey) return
  error.value = ''
  downloadFailed.value = false
  let request: Record<string, unknown> | null = null
  try {
    if (!resume) {
      if (!availableModels.value.some((item) => item.id === form.modelID)) throw new Error('当前密钥不支持所选型号。')
      if ([...imageFiles.value, ...audioFiles.value].reduce((sum, file) => sum + file.size, 0) > 100 * 1024 * 1024) throw new Error('本次参考素材合计最多 100MB，请减少素材大小或使用公开链接。')
      request = buildSuanliVideoRequest({
        modelID: form.modelID, prompt: form.prompt, duration: form.duration, ratio: form.ratio,
        imageURLs: [...splitReferenceURLs(form.images), ...imageFiles.value.map(() => 'https://reference.invalid/pending')],
        audioURLs: [...splitReferenceURLs(form.audio), ...audioFiles.value.map(() => 'https://reference.invalid/pending')],
      })
    } else if (!taskID.value.trim()) throw new Error('请输入需要查询的任务 ID。')
  } catch (caught) {
    error.value = requestError(caught, props.apiKey)
    return
  }
  if (request) {
    taskID.value = ''
    clearPreview()
  }
  const active = new AbortController()
  const apiKey = props.apiKey
  controller = active
  activity.value = request ? 'submitting' : 'querying'
  try {
    if (request) {
      const images = splitReferenceURLs(form.images)
      const audio = splitReferenceURLs(form.audio)
      for (const [kind, files, target] of [['image', imageFiles.value, images], ['audio', audioFiles.value, audio]] as const) {
        for (const file of files) {
          status.value = `正在上传参考素材：${file.name}`
          const uploaded = await uploadVideoReference(apiKey, file, kind, active.signal)
          if (!isCurrent(active, apiKey)) return
          target.push(uploaded.url)
        }
      }
      request = buildSuanliVideoRequest({ modelID: form.modelID, prompt: form.prompt, duration: form.duration, ratio: form.ratio, imageURLs: images, audioURLs: audio })
      status.value = '提交中'
      const submitted = await videoGenerationAPI.submitGateway(apiKey, '/v1/videos/generations', request, { signal: active.signal })
      if (!isCurrent(active, apiKey)) return
      assigningSubmittedID = true
      taskID.value = submitted.requestId
      assigningSubmittedID = false
      activity.value = 'querying'
    }
    await poll(apiKey, taskID.value.trim(), active)
  } catch (caught) {
    if (isCurrent(active, apiKey)) error.value = requestError(caught, apiKey)
  } finally {
    if (controller === active) {
      controller = null
      activity.value = 'idle'
    }
  }
}

function saveVideo(): void {
  if (!videoURL.value.startsWith('blob:')) return
  const link = document.createElement('a')
  link.href = videoURL.value
  link.download = 'generated-video.mp4'
  document.body.appendChild(link)
  link.click()
  link.remove()
}

async function saveToFile(): Promise<void> {
  const id = taskID.value.trim()
  if (!id || !props.apiKey || props.disabled || busy.value) return
  const apiKey = props.apiKey
  const identity = `suanli-${keyEpoch}-${id}`
  const active = new AbortController()
  controller = active
  diskDownloadActive = true
  activity.value = 'downloading'
  error.value = ''
  try {
    const picked = fileSession || await pickVideoDownloadFile(identity, `video-${id.replace(/[^\w-]/g, '_').slice(0, 80)}.mp4`)
    if (!isCurrent(active, apiKey, id)) return
    fileSession = picked
    await downloadVideoToFile({ identity, session: fileSession, signal: active.signal,
      request: (headers, signal) => fetch(buildVideoContentURL(id), { headers: { ...headers, Authorization: `Bearer ${apiKey}` }, signal }),
      onProgress: ({ received, total, resumed, restarted }) => {
        if (isCurrent(active, apiKey, id)) fileProgress.value = `${resumed ? '继续下载' : restarted ? '上游不支持续传，重新下载' : '正在保存'}：${(received / 1048576).toFixed(1)}${total ? ` / ${(total / 1048576).toFixed(1)}` : ''} MB`
      },
    })
    if (isCurrent(active, apiKey, id)) fileProgress.value = '已保存到所选文件。'
  } catch (caught) {
    if (isCurrent(active, apiKey, id)) error.value = requestError(caught, apiKey)
  } finally {
    if (controller === active) {
      if (active.signal.aborted && props.apiKey === apiKey && taskID.value.trim() === id && fileSession?.bytes) fileProgress.value = '已暂停并保存下载进度，可继续保存到文件。'
      controller = null; diskDownloadActive = false; activity.value = 'idle'
    }
  }
}

async function downloadVideo(): Promise<void> {
  const id = taskID.value.trim()
  if (!id || !props.apiKey || props.disabled || activity.value === 'submitting' || activity.value === 'downloading') return
  if (videoURL.value) {
    saveVideo()
    return
  }
  stopQuerying()
  error.value = ''
  downloadFailed.value = false
  const active = new AbortController()
  const apiKey = props.apiKey
  controller = active
  activity.value = 'downloading'
  const discard = (url: string) => { if (url.startsWith('blob:')) URL.revokeObjectURL(url) }
  try {
    const url = await timedRequest(active.signal, 90000, '获取视频超时，请点击“重试下载”。已提交的生成任务不受影响。',
      (signal) => videoGenerationAPI.createContentURL(apiKey, id, signal), discard)
    if (!isCurrent(active, apiKey, id)) {
      discard(url)
      return
    }
    if (!url.startsWith('blob:')) throw new Error('视频文件响应无效，请重试下载。')
    videoURL.value = url
    saveVideo()
  } catch (caught) {
    if (isCurrent(active, apiKey, id)) {
      downloadFailed.value = true
      error.value = requestError(caught, apiKey)
    }
  } finally {
    if (controller === active) {
      controller = null
      activity.value = 'idle'
    }
  }
}

onBeforeUnmount(() => { stopQuerying(); clearPreview(); emit('busy', false) })
</script>

<template>
  <form class="suanli-video-form" data-testid="suanli-special-video-runner" @submit.prevent="run()">
    <header><h2>算力 AI 视频测试</h2><p>使用当前专用分组的密钥提交，结果在本页查询和下载。</p></header>
    <section class="task-result" aria-label="任务查询与下载" data-testid="suanli-task-result">
      <div class="task-query">
        <label>任务 ID<input v-model="taskID" :disabled="activity === 'submitting'" placeholder="提交后自动填写，也可粘贴已有任务 ID" data-testid="suanli-task-id"></label>
        <button type="button" :disabled="disabled || busy || !apiKey || !taskID.trim()" data-testid="suanli-query" @click="run(true)">继续查询</button>
        <button type="button" :disabled="disabled || !apiKey || !taskID.trim() || activity === 'submitting' || activity === 'downloading'" data-testid="suanli-download" @click="downloadVideo">{{ activity === 'downloading' ? '正在获取视频…' : downloadFailed ? '重试下载' : '下载视频' }}</button>
        <button v-if="diskDownloadSupported" type="button" :disabled="disabled || busy || !apiKey || !taskID.trim()" data-testid="suanli-save-file" @click="saveToFile">{{ fileSession && !fileSession.complete && fileSession.bytes ? '继续保存到文件' : '保存到文件（适合大视频）' }}</button>
        <button v-if="activity === 'querying'" type="button" data-testid="suanli-stop" @click="stopQuerying">停止查询</button>
        <button v-if="activity === 'downloading'" type="button" data-testid="suanli-stop" @click="stopQuerying">取消下载</button>
      </div>
      <p v-if="activity === 'submitting'" class="hint">任务正在提交，请等待返回任务 ID。</p>
      <p v-else-if="activity === 'downloading'" class="hint" role="status">正在获取视频文件，完成后可保存和预览。取消下载不会取消生成任务。</p>
      <p v-else-if="activity === 'querying'" class="hint">每 12 秒查询一次。停止查询或离开页面不会取消已经提交的任务，请保存任务 ID。</p>
      <p v-else-if="videoURL" class="hint">视频文件已就绪，可预览或再次保存。</p>
      <p v-else-if="completed" class="hint">生成完成，点击“下载视频”获取文件并预览。</p>
      <p v-else class="hint">已有任务 ID 可直接查询或下载，无需重新填写生成参数。视频仍在生成时请稍后重试。</p>
      <p v-if="status" role="status">状态：{{ status }}</p>
      <p v-if="fileProgress" class="hint" role="status">{{ fileProgress }}</p>
      <p v-if="diskDownloadSupported" class="hint">保存到文件不缓存完整视频；保留本页可暂停后继续。上游不支持续传时会从头下载。</p>
      <p v-else class="hint">当前浏览器使用普通下载，会先缓存完整视频；大文件建议使用支持“保存到文件”的桌面 Chrome 或 Edge。</p>
      <p v-if="error" role="alert" class="error">{{ error }}</p>
      <video v-if="videoURL" :src="videoURL" controls preload="metadata" />
      <a v-if="videoURL" :href="videoURL" download="generated-video.mp4">保存视频到本地</a>
    </section>
    <fieldset :disabled="disabled || busy">
      <label>视频型号<select v-model="form.modelID" data-testid="suanli-model"><option v-for="item in availableModels" :key="item.id" :value="item.id">{{ item.id }}</option></select></label>
      <label>提示词<textarea v-model="form.prompt" rows="4" maxlength="6000" placeholder="描述主体、动作、镜头和画面要求" /></label>
      <div class="parameter-row">
        <label>分辨率<input :value="model?.resolution || ''" readonly aria-readonly="true"></label>
        <label>画面比例<select v-model="form.ratio"><option v-for="ratio in SUANLI_VIDEO_RATIOS" :key="ratio" :value="ratio">{{ ratio }}</option></select></label>
        <label>时长（秒）<input v-model.number="form.duration" type="number" :min="model?.family === 'sd25' ? 30 : 5" :max="model?.family === 'sd25' ? 30 : 15" :readonly="model?.family === 'sd25'" step="1"></label>
      </div>
      <p v-if="model?.family === 'seedance'" class="hint">首批测试开放单图、5–15 秒；这是本站测试范围，完整素材和时长能力尚待逐项验证。</p>
      <p v-else-if="model?.family === 'sd25'" class="hint">固定 30 秒、720p，支持最多 10 张图片，不支持参考视频或音频。</p>
      <p v-else class="hint">5–15 秒，最多 9 张图片、3 条音频，不支持参考视频。</p>
      <label>参考图片链接（1–{{ model?.maxImages || 1 }} 张，每行一条）<textarea v-model="form.images" rows="3" placeholder="https://…/reference.jpg" data-testid="suanli-images" /></label>
      <label>或选择本地参考图片<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple data-testid="suanli-image-files" @change="selectFiles($event, 'image')"></label>
      <p v-if="imageFiles.length" class="hint">已选：{{ imageFiles.map(file => file.name).join('、') }} <button type="button" @click="imageFiles = []">清除文件</button></p>
      <label v-if="model?.maxAudio">参考音频链接（可选，最多 3 条，每行一条）<textarea v-model="form.audio" rows="2" placeholder="https://…/reference.mp3" /></label>
      <label v-if="model?.maxAudio">或选择本地参考音频<input type="file" accept="audio/mpeg,audio/wav,audio/ogg,audio/flac,audio/mp4,.m4a" multiple @change="selectFiles($event, 'audio')"></label>
      <p v-if="audioFiles.length" class="hint">已选：{{ audioFiles.map(file => file.name).join('、') }} <button type="button" @click="audioFiles = []">清除文件</button></p>
      <p class="hint">本地素材在提交时临时上传，供视频平台读取，24 小时后自动清理；每个文件最多 25MB，每人临时素材最多 100MB。同页重复使用会复用未过期素材。文件与链接合计不得超过型号限制。上传不扣生成费，提交生成后按当前分组价格计费。</p>
      <button class="submit-button" type="submit" :disabled="!model || !form.prompt.trim() || (!form.images.trim() && !imageFiles.length)">提交视频任务</button>
    </fieldset>

  </form>
</template>

<style scoped>
.suanli-video-form, fieldset { display: grid; gap: 12px; min-width: 0; }
fieldset { margin: 0; padding: 0; border: 0; }
h2 { margin: 0; font-size: 16px; color: var(--customer-ink); }
header p, .hint { margin: 5px 0; color: var(--customer-muted); font-size: 12px; line-height: 1.6; }
label { display: grid; min-width: 0; gap: 5px; color: var(--customer-ink); font-size: 12px; }
input, select, textarea { width: 100%; min-width: 0; padding: 9px; border: 1px solid var(--customer-line); border-radius: 5px; color: var(--customer-ink); background: var(--customer-surface); }
textarea { resize: vertical; }
.parameter-row { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; }
.task-result { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--customer-line); border-radius: 6px; }
.task-result p { margin: 0; }
.task-query { display: flex; align-items: end; flex-wrap: wrap; gap: 8px; }
.task-query label { flex: 1 1 180px; }
button { padding: 9px 12px; border: 1px solid var(--customer-line); border-radius: 5px; }
.submit-button { color: white; background: var(--customer-cyan); border-color: var(--customer-cyan); }
button:disabled { opacity: .5; cursor: not-allowed; }
.error { color: #dc2626; font-size: 12px; overflow-wrap: anywhere; }
video { width: 100%; max-height: 420px; border-radius: 6px; }
a { color: var(--customer-cyan); }
</style>
