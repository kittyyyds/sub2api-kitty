<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { videoModels, videoQuote, videoDelivery, fetchVideoContent, type VideoModel, type VideoQuote } from '@/api/videoDirect'
import { videoGenerationAPI } from '@/api/videoGeneration'
import { uploadVideoReference } from '@/api/videoReferences'
import { downloadVideoToFile, pickVideoDownloadFile, supportsVideoFileDownload, type VideoFileDownloadSession } from '@/utils/videoDownload'
import { splitReferenceURLs } from '../../suanliVideo'
import SuanliSpecialVideoRunner from './SuanliSpecialVideoRunner.vue'

const props = defineProps<{ apiKey: string; modelIds: string[]; disabled: boolean }>()
const emit = defineEmits<{ busy: [boolean] }>()
const catalog = ref<VideoModel[]>([])
const selected = ref('')
const model = computed(() => catalog.value.find(m => m.id === selected.value))
const cap = computed(() => model.value?.capabilities)
const legacy = computed(() => !!model.value && cap.value?.family !== 'unified')
const form = reactive({ prompt: '', seconds: 5, ratio: '16:9', resolution: '720p', images: '', videos: '', audios: '', generateAudio: true, face: false })
const files = reactive<Record<'image' | 'video' | 'audio', File[]>>({ image: [], video: [], audio: [] })
const quote = ref<VideoQuote>()
const task = ref('')
const status = ref('')
const error = ref('')
const progress = ref('')
const busy = ref(false)
const uncertain = ref(false)
const directURL = ref('')
const canSave = supportsVideoFileDownload()
let prepared: Record<string, unknown> | null = null
let idem = ''
let requestKey = ''
let controller: AbortController | null = null
let fileSession: VideoFileDownloadSession | null = null
watch(busy, value => emit('busy', value))
watch(() => [props.apiKey, props.modelIds.join('\n')], async () => {
  controller?.abort(); busy.value = false; catalog.value = []; prepared = null; error.value = ''; progress.value = ''; const key = props.apiKey; quote.value = undefined; task.value = ''; uncertain.value = false; idem = ''; fileSession = null
  if (!key) return
  const active = new AbortController(); controller = active
  try { const result = await videoModels(key, active.signal); if (props.apiKey !== key || active.signal.aborted) return; catalog.value = result.filter(m => props.modelIds.includes(m.id)); selected.value = catalog.value[0]?.id || '' }
  catch (e) { if (!active.signal.aborted) error.value = e instanceof Error ? e.message : '目录读取失败' }
}, { immediate: true })
watch(selected, () => {
  const c = cap.value; form.seconds = c?.second_options?.[0] || c?.seconds?.min || 5
  form.resolution = c?.resolutions?.[0] || ''; form.ratio = c?.aspect_ratios?.[0] || '16:9'
  form.images = ''; form.videos = ''; form.audios = ''; files.image = []; files.video = []; files.audio = []
})
watch(task, () => { directURL.value = ''; fileSession = null })
watch([form, files, selected], () => { quote.value = undefined; prepared = null }, { deep: true })
function selectFiles(event: Event, kind: 'image' | 'video' | 'audio') { const input = event.target as HTMLInputElement; files[kind] = Array.from(input.files || []); input.value = '' }
async function execute(action: (signal: AbortSignal, key: string) => Promise<void>) {
  if (busy.value || props.disabled || !props.apiKey) return
  busy.value = true; error.value = ''; requestKey = props.apiKey
  const active = new AbortController(); controller = active
  try { await action(active.signal, requestKey) } catch (e) { if (!active.signal.aborted) error.value = e instanceof Error ? e.message : '操作失败，请稍后重试' }
  finally { if (controller === active) { busy.value = false; controller = null } }
}
function prepareQuote() { return execute(async (signal, key) => {
  const c = cap.value
  if (!c || c.family !== 'unified') throw new Error('请选择已适配的型号')
  if (!form.prompt.trim()) throw new Error('请输入视频提示词')
  const refs = { images: splitReferenceURLs(form.images), videos: splitReferenceURLs(form.videos), audios: splitReferenceURLs(form.audios) }
  for (const [kind, field, max] of [['image','images', c.reference_images?.max || 0],['video','videos',c.reference_videos?.max || 0],['audio','audios',c.reference_audios?.max || 0]] as const) {
    if (refs[field].length + files[kind].length > max) throw new Error(`当前型号的${kind === 'image' ? '图片' : kind === 'video' ? '视频' : '音频'}素材最多 ${max} 个`)
    for (const file of files[kind]) { progress.value = `上传参考素材：${file.name}`; refs[field].push((await uploadVideoReference(key,file,kind,signal)).url) }
  }
  prepared = { model: selected.value, prompt: form.prompt.trim(), seconds: form.seconds, aspect_ratio: form.ratio, resolution: form.resolution, ...refs,
    ...(c.generate_audio ? { generate_audio: form.generateAudio } : {}), ...(c.face ? { face: form.face } : {}) }
  const result = await videoQuote(key,prepared,signal)
  if (result.reference_videos?.length) prepared.videos = result.reference_videos
  if (props.apiKey !== key || signal.aborted) return
  quote.value = result; progress.value = ''; idem = crypto.randomUUID(); uncertain.value = false
}) }
async function poll(signal: AbortSignal, key: string) {
  const until = Date.now() + 20 * 60 * 1000
  while (!signal.aborted && props.apiKey === key) {
    const result = await videoGenerationAPI.status(key,task.value,signal)
    status.value = result.status
    if (['completed','done','succeeded','success','failed','expired','error'].includes(status.value)) return
    if (Date.now() > until) { progress.value = '任务仍在进行，可稍后继续查询'; return }
    await new Promise<void>((resolve,reject) => {
      const abort = () => { clearTimeout(timer); reject(new DOMException('已停止查询','AbortError')) }
      const timer = setTimeout(() => { signal.removeEventListener('abort',abort); resolve() },5000)
      signal.addEventListener('abort',abort,{ once:true })
    })
  }
}
function submit() { return execute(async (signal,key) => {
  if (!prepared || !quote.value || uncertain.value) throw new Error('请先核对报价和原任务状态')
  const latest = await videoQuote(key,prepared,signal)
  if (latest.estimated_cost !== quote.value.estimated_cost || latest.unit_price !== quote.value.unit_price) { quote.value = latest; throw new Error('报价已变化，请核对后重新确认') }
  uncertain.value = true
  const result = await videoGenerationAPI.submitGateway(key,'/v1/videos/generations',prepared,{ signal, idempotencyKey: idem })
  if (props.apiKey !== key) return
  task.value = result.requestId; fileSession = null; uncertain.value = false; quote.value = undefined
  await poll(signal,key)
}) }
function query() { return execute((signal,key) => poll(signal,key)) }
function openDirect() { return execute(async(signal,key) => { const delivery = await videoDelivery(key,task.value,signal); if (!delivery.direct) throw new Error('该任务使用旧交付协议'); if (!signal.aborted && props.apiKey === key) directURL.value = delivery.url }) }
function save() { return execute(async(signal,key) => {
  const identity = `${task.value}:${idem}`
  fileSession ||= await pickVideoDownloadFile(identity,`video-${task.value.replace(/[^\w-]/g,'_').slice(0,60)}.mp4`)
  await downloadVideoToFile({ identity, session:fileSession, signal, request:(headers,signal)=>fetchVideoContent(key,task.value,headers,signal),onProgress:p=>{progress.value=`已保存 ${(p.received/1048576).toFixed(1)} MB`} })
}) }
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section data-testid="suanli-special-video-runner" class="space-y-5 rounded-2xl border border-gray-200 bg-white p-5 text-gray-900 dark:border-dark-500 dark:bg-dark-800 dark:text-gray-100">
    <div class="flex flex-wrap items-end gap-4"><label class="min-w-0 flex-1 text-sm">视频型号<select v-model="selected" class="input mt-2" :disabled="busy || disabled"><option v-for="m in catalog" :key="m.id" :value="m.id">{{ m.id }}</option></select></label><RouterLink to="/video-tasks" class="text-sm text-primary-600">历史视频任务</RouterLink></div>
    <SuanliSpecialVideoRunner v-if="legacy" :api-key="apiKey" :model-ids="[selected]" :disabled="disabled" @busy="emit('busy',$event)" />
    <template v-else-if="cap">
      <p class="text-xs text-gray-500">按型号显示支持的输入参数；生成成片从上游下载，建议完成后及时保存。</p>
      <fieldset :disabled="busy || disabled || uncertain" class="space-y-4 disabled:opacity-60">
        <label class="block text-sm">视频描述<textarea v-model="form.prompt" rows="4" class="input mt-2" placeholder="描述画面、动作、镜头和声音" /></label>
        <div class="grid gap-4 sm:grid-cols-3">
          <label class="text-sm">时长（秒）<select v-if="cap.second_options?.length" v-model.number="form.seconds" class="input mt-2"><option v-for="s in cap.second_options" :key="s" :value="s">{{ s }}</option></select><input v-else v-model.number="form.seconds" type="number" :min="cap.seconds?.min" :max="cap.seconds?.max" step="1" class="input mt-2"></label>
          <label class="text-sm">分辨率<select v-model="form.resolution" class="input mt-2"><option v-for="r in cap.resolutions" :key="r">{{ r }}</option></select></label>
          <label class="text-sm">画面比例<select v-model="form.ratio" class="input mt-2"><option v-for="r in cap.aspect_ratios" :key="r">{{ r }}</option></select></label>
        </div>
        <div class="grid gap-4 lg:grid-cols-3">
          <label v-if="cap.reference_images?.max" class="text-sm">参考图片（{{ cap.reference_images.min }}–{{ cap.reference_images.max }} 张）<textarea v-model="form.images" rows="3" class="input my-2" placeholder="公开图片链接，每行一个" /><input type="file" multiple accept="image/png,image/jpeg,image/webp,image/gif" @change="selectFiles($event,'image')"><p>已选 {{ files.image.length }} 个本地文件</p></label>
          <label v-if="cap.reference_videos?.max" class="text-sm">参考视频（最多 {{ cap.reference_videos.max }} 条）<textarea v-model="form.videos" rows="3" class="input my-2" placeholder="公开 MP4 链接，每行一个" /><input type="file" multiple accept="video/mp4" @change="selectFiles($event,'video')"><p>已选 {{ files.video.length }} 个本地文件</p></label>
          <label v-if="cap.reference_audios?.max" class="text-sm">参考音频（最多 {{ cap.reference_audios.max }} 条）<textarea v-model="form.audios" rows="3" class="input my-2" placeholder="公开音频链接，每行一个" /><input type="file" multiple accept="audio/*" @change="selectFiles($event,'audio')"><p>已选 {{ files.audio.length }} 个本地文件</p></label>
        </div>
        <div class="flex gap-5 text-sm"><label v-if="cap.generate_audio"><input v-model="form.generateAudio" type="checkbox"> 生成声音</label><label v-if="cap.face"><input v-model="form.face" type="checkbox"> 使用模型人脸参数</label></div>
        <button class="btn btn-secondary" @click="prepareQuote">上传参考素材并计算费用</button>
      </fieldset>
      <div v-if="quote" class="space-y-2 rounded-xl bg-primary-50 p-4 text-sm dark:bg-dark-700"><p>生成费用：{{ quote.output_cost }} · 参考费用：{{ quote.reference_cost }}（{{ quote.reference_seconds }} 秒 × {{ quote.reference_per_second }}）</p><p class="font-semibold">预计预扣：{{ quote.estimated_cost }}</p><p>{{ quote.note }}</p><button class="btn btn-primary" :disabled="busy || disabled || uncertain" @click="submit">确认费用并生成</button></div>
      <p v-if="uncertain && !task" class="text-sm text-amber-600">提交结果尚未确认。请先到“视频任务”核对，不要再次生成。</p>
      <div class="flex flex-wrap items-center gap-3"><input v-model="task" class="input flex-1" :disabled="busy" placeholder="任务 ID，可继续查询之前的任务"><button class="btn btn-secondary" :disabled="busy || !task" @click="query">继续查询</button><button v-if="busy" class="btn btn-secondary" @click="controller?.abort()">停止等待</button><button v-if="canSave" class="btn btn-secondary" :disabled="busy || !task" @click="save">保存到本地文件</button><button class="btn btn-secondary" :disabled="busy || !task" @click="openDirect">获取上游下载链接</button><a v-if="directURL" :href="directURL" target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer" class="btn btn-primary">点击直接下载</a></div>
      <p v-if="status || progress" role="status" class="text-sm">任务状态：{{ status }} {{ progress }}</p>
    </template>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="!catalog.length" class="text-sm text-gray-500">当前 Key 尚无已适配的视频型号，请检查分组价格与上游账号白名单。</p>
  </section>
</template>
