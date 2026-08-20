<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'

import {
  GROK_VIDEO_ASPECT_RATIOS,
  GROK_VIDEO_15_MODEL,
  GROK_VIDEO_MODEL,
  GROK_VIDEO_MODELS,
  type GrokVideoResolution,
} from '@/api/videoGeneration'
import Icon from '@/components/icons/Icon.vue'
import ReferenceUpload from '../ReferenceUpload.vue'
import { catalogField, catalogStringOptions } from '../../catalogForm'
import type { ApplicationDefinition, RunnerInputPayload } from '../../types'

const props = withDefaults(defineProps<{
  disabled: boolean
  definition: ApplicationDefinition | null
  mediaPlatform?: string
}>(), {
  mediaPlatform: 'grok',
})
const emit = defineEmits<{ run: [payload: RunnerInputPayload] }>()
type VideoMode = 'text_to_video' | 'image_to_video' | 'reference_to_video' | 'video_edit' | 'video_extend'

const FALLBACK_MODES = [
  { label: '文生视频', value: 'text_to_video' },
  { label: '图生视频', value: 'image_to_video' },
  { label: '多图参考生视频', value: 'reference_to_video' },
  { label: '视频编辑', value: 'video_edit' },
  { label: '视频续写', value: 'video_extend' },
] as const
const FALLBACK_MODELS = [
  { label: 'Grok Imagine Video 1.5', value: GROK_VIDEO_15_MODEL },
  { label: 'Grok Imagine Video', value: GROK_VIDEO_MODEL },
] as const
const FALLBACK_RESOLUTIONS = [
  { label: '480p', value: '480p' },
  { label: '720p', value: '720p' },
  { label: '1080p', value: '1080p' },
] as const
const FALLBACK_RATIOS = [
  ...GROK_VIDEO_ASPECT_RATIOS.map((value) => ({ label: value, value })),
]

const SUPPORTED_MODES = new Set<VideoMode>(FALLBACK_MODES.map((option) => option.value))
const SUPPORTED_MODELS = new Set<string>(GROK_VIDEO_MODELS)
const SUPPORTED_RESOLUTIONS = new Set<GrokVideoResolution>(FALLBACK_RESOLUTIONS.map((option) => option.value))
const SUPPORTED_RATIOS = new Set<string>(GROK_VIDEO_ASPECT_RATIOS)

const form = reactive({
  brief: '',
  mode: 'text_to_video' as VideoMode,
  style: '电影叙事',
  subject_action: '',
  camera_movement: '稳定推进，运动自然连贯',
  pacing: '节奏清晰，开头快速建立主体，结尾完整收束',
  continuity: '主体身份、服装、比例、光线方向和空间关系保持一致',
  must_keep: '',
  avoid: '',
  aspect_ratio: '16:9',
  duration: 8,
  resolution: '720p',
  model: GROK_VIDEO_15_MODEL as string,
})
const files = shallowRef<File[]>([])
const localError = shallowRef('')
const modeOptions = computed(() => {
  const supported = new Set(props.definition?.supported_modes || [])
  const options = catalogStringOptions(props.definition, 'mode', FALLBACK_MODES)
    .filter((option) => SUPPORTED_MODES.has(option.value as VideoMode)
      && (!supported.size || supported.has(option.value)))
  return options.length
    ? options
    : [...FALLBACK_MODES]
})
const allModelOptions = computed(() => {
  const options = catalogStringOptions(props.definition, 'model', FALLBACK_MODELS)
    .filter((option) => SUPPORTED_MODELS.has(option.value))
  return options.length ? options : [...FALLBACK_MODELS]
})
const modelOptions = computed(() => {
  if (!['reference_to_video', 'video_edit', 'video_extend'].includes(form.mode)) return allModelOptions.value
  const options = allModelOptions.value.filter((option) => option.value === GROK_VIDEO_MODEL)
  return options.length ? options : [{ label: 'Grok Imagine Video', value: GROK_VIDEO_MODEL }]
})
const resolutionOptions = computed(() => {
  const options = catalogStringOptions(props.definition, 'resolution', FALLBACK_RESOLUTIONS)
    .filter((option) => SUPPORTED_RESOLUTIONS.has(option.value as GrokVideoResolution))
  return options.length ? options : [...FALLBACK_RESOLUTIONS]
})
const availableResolutions = computed(() => resolutionOptions.value.filter((option) => (
  option.value !== '1080p'
  || (['text_to_video', 'image_to_video'].includes(form.mode) && form.model === GROK_VIDEO_15_MODEL)
)))
const aspectRatioOptions = computed(() => {
  const options = catalogStringOptions(props.definition, 'aspect_ratio', FALLBACK_RATIOS)
    .filter((option) => SUPPORTED_RATIOS.has(option.value))
  return options.length ? options : [...FALLBACK_RATIOS]
})
const needsSource = computed(() => form.mode !== 'text_to_video')
const isVideoSource = computed(() => form.mode === 'video_edit' || form.mode === 'video_extend')
const acceptedFiles = computed(() => isVideoSource.value
  ? 'video/mp4,video/webm'
  : 'image/png,image/jpeg,image/webp')
const maxFiles = computed(() => form.mode === 'reference_to_video'
  ? Math.min(8, Math.max(2, catalogField(props.definition, 'reference_files')?.max_items || 5))
  : 1)
const durationMin = computed(() => catalogField(props.definition, 'duration')?.min ?? 1)
const durationMax = computed(() => catalogField(props.definition, 'duration')?.max ?? 15)
const validDuration = computed(() => Number.isInteger(form.duration)
  && form.duration >= durationMin.value
  && form.duration <= durationMax.value)

function sourceValidationError(): string {
  if (form.mode === 'text_to_video') return ''
  if (isVideoSource.value) {
    return files.value.length === 1 && files.value[0]?.type.startsWith('video/')
      ? ''
      : '视频编辑和续写需要上传一个 MP4 或 WebM 视频。'
  }
  const images = files.value.filter((file) => file.type.startsWith('image/'))
  if (form.mode === 'reference_to_video') {
    return images.length >= 2 && images.length <= maxFiles.value && images.length === files.value.length
      ? ''
      : `多图参考生视频需要上传 2-${maxFiles.value} 张图片。`
  }
  return images.length === 1 && files.value.length === 1
    ? ''
    : '图生视频需要上传一张起始图片。'
}

const canRun = computed(() => props.mediaPlatform === 'grok' && !props.disabled
  && form.brief.trim().length >= 4
  && validDuration.value
  && !sourceValidationError())

function syncConstraints(): void {
  if (!modeOptions.value.some((option) => option.value === form.mode)) {
    form.mode = (modeOptions.value[0]?.value || 'text_to_video') as VideoMode
  }
  if (!modelOptions.value.some((option) => option.value === form.model)) {
    form.model = modelOptions.value[0]?.value || GROK_VIDEO_15_MODEL
  }
  if (!availableResolutions.value.some((option) => option.value === form.resolution)) {
    form.resolution = availableResolutions.value.find((option) => option.value === '720p')?.value
      || availableResolutions.value[0]?.value
      || '720p'
  }
  if (!aspectRatioOptions.value.some((option) => option.value === form.aspect_ratio)) {
    form.aspect_ratio = aspectRatioOptions.value.find((option) => option.value === '16:9')?.value
      || aspectRatioOptions.value[0]?.value
      || '16:9'
  }
}

watch(() => form.mode, (mode, previousMode) => {
  if (previousMode && mode !== previousMode) files.value = []
  syncConstraints()
}, { immediate: true })
watch(() => form.model, syncConstraints)
watch(() => props.definition, syncConstraints)
watch(() => props.mediaPlatform, syncConstraints)

function submit(): void {
  localError.value = ''
  syncConstraints()
  if (!canRun.value) {
    localError.value = props.mediaPlatform !== 'grok'
      ? '视频功能必须选择 Grok 媒体密钥。'
      : sourceValidationError()
      || (!validDuration.value ? `视频时长必须是 ${durationMin.value}-${durationMax.value} 秒的整数。` : '请填写至少 4 个字的视频需求。')
    return
  }
  emit('run', {
    values: {
      brief: form.brief.trim(),
      mode: form.mode,
      model: form.model,
      style: form.style,
      subject_action: form.subject_action.trim(),
      camera_movement: form.camera_movement.trim(),
      pacing: form.pacing.trim(),
      continuity: form.continuity.trim(),
      must_keep: form.must_keep.trim(),
      avoid: form.avoid.trim(),
      aspect_ratio: form.aspect_ratio,
      duration: form.duration,
      resolution: form.resolution,
      reference_count: files.value.length,
      source_type: files.value.some((file) => file.type.startsWith('video/')) ? 'video' : files.value.length ? 'image' : 'none',
    },
    files: [...files.value],
  })
}
</script>

<template>
  <form class="runner-form" @submit.prevent="submit">
    <header class="runner-heading">
      <div><h2>Grok 视频导演</h2><p>先确定镜头、动作与节奏，再提交对应的视频任务。</p></div>
      <code class="runner-code">GROK VIDEO / 2-STAGE</code>
    </header>
    <fieldset class="runner-fields" :disabled="disabled">
      <label class="runner-field runner-field-wide"><span>视频需求 <em>*</em></span><textarea v-model="form.brief" maxlength="4000" placeholder="主体、环境、用途、情绪，以及这段视频要表达什么" /></label>
      <label class="runner-field"><span>任务类型</span><select v-model="form.mode"><option v-for="option in modeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>执行模型</span><select v-model="form.model"><option v-for="option in modelOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>视觉风格</span><select v-model="form.style"><option>电影叙事</option><option>商业广告</option><option>纪实镜头</option><option>动画短片</option></select></label>
      <label class="runner-field"><span>主体动作</span><input v-model="form.subject_action" maxlength="400" placeholder="动作起点、过程和结束状态"></label>
      <label class="runner-field"><span>镜头运动</span><input v-model="form.camera_movement" maxlength="400" placeholder="推进、横移、环绕、手持或固定镜头"></label>
      <label class="runner-field"><span>节奏</span><input v-model="form.pacing" maxlength="400" placeholder="开头、转折、高潮和收束"></label>
      <label class="runner-field runner-field-wide"><span>连续性要求</span><textarea v-model="form.continuity" maxlength="1200" placeholder="人物、商品、服装、空间和光线需要如何保持一致" /></label>
      <label class="runner-field runner-field-wide"><span>禁止改动项</span><textarea v-model="form.must_keep" maxlength="1200" placeholder="人物身份、商品结构、Logo、文字或必须保持不变的细节" /></label>
      <label class="runner-field runner-field-wide"><span>禁止出现</span><textarea v-model="form.avoid" maxlength="1200" placeholder="畸变、闪烁、身份漂移、错误文字、多余肢体或不希望出现的元素" /></label>
      <label class="runner-field"><span>比例</span><select v-model="form.aspect_ratio"><option v-for="option in aspectRatioOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>清晰度</span><select v-model="form.resolution"><option v-for="option in availableResolutions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>时长</span><input v-model.number="form.duration" type="number" :min="durationMin" :max="durationMax" step="1"></label>
      <div v-if="needsSource" class="runner-field runner-field-wide"><span>{{ isVideoSource ? '源视频' : '参考图片' }} <em>*</em></span><ReferenceUpload v-model="files" required :disabled="disabled" :multiple="maxFiles > 1" :max-files="maxFiles" :accept="acceptedFiles" /></div>
    </fieldset>
    <p v-if="localError" class="runner-error" role="alert">{{ localError }}</p>
    <div class="runner-actions"><button class="run-button" type="submit" :disabled="!canRun"><Icon name="play" size="sm" />开始导演</button></div>
  </form>
</template>

<style scoped src="./runner.css"></style>
