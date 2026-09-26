<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'

import { GROK_IMAGE_RESOLUTIONS } from '@/api/imageGeneration'
import Icon from '@/components/icons/Icon.vue'
import ReferenceUpload from '../ReferenceUpload.vue'
import { catalogStringOptions } from '../../catalogForm'
import type { ApplicationDefinition, RunnerInputPayload } from '../../types'
import {
  GROK_IMAGE_RUNNER_MODELS,
  GROK_IMAGE_RUNNER_RATIOS,
  OPENAI_IMAGE_QUALITY_OPTIONS,
  OPENAI_IMAGE_RUNNER_MODELS,
  OPENAI_IMAGE_RUNNER_RATIOS,
  imageGenerationCountOptions,
  normalizeImageMediaPlatform,
  normalizeImageGenerationCount,
  openAIImageSizeForRatio,
} from './imageRunnerContract'

const props = withDefaults(defineProps<{
  disabled: boolean
  definition: ApplicationDefinition | null
  mediaPlatform?: string
}>(), {
  mediaPlatform: 'grok',
})
const emit = defineEmits<{ run: [payload: RunnerInputPayload] }>()
type ImageMode = 'text_to_image' | 'image_edit'

const FALLBACK_MODES = [
  { label: '文生图', value: 'text_to_image' },
  { label: '参考图精修', value: 'image_edit' },
] as const
const FALLBACK_RESOLUTIONS = GROK_IMAGE_RESOLUTIONS.map((value) => ({ label: value.toUpperCase(), value }))
const SUPPORTED_MODES = new Set<ImageMode>(FALLBACK_MODES.map((option) => option.value))
const SUPPORTED_RESOLUTIONS = new Set<string>(GROK_IMAGE_RESOLUTIONS)

const form = reactive({
  brief: '',
  mode: 'text_to_image' as ImageMode,
  purpose: '内容配图',
  style: '写实摄影',
  composition: '主体清晰，视觉层级明确',
  lighting: '自然柔光',
  palette: '协调中性色',
  must_keep: '',
  avoid: '',
  aspect_ratio: '1:1',
  resolution: '2k',
  quality: 'auto',
  n: 1,
  model: 'grok-imagine-image-quality',
})
const files = shallowRef<File[]>([])
const localError = shallowRef('')
const mediaPlatform = computed(() => normalizeImageMediaPlatform(props.mediaPlatform))
const isOpenAI = computed(() => mediaPlatform.value === 'openai')
const countOptions = computed(() => imageGenerationCountOptions(mediaPlatform.value))
const modeOptions = computed(() => {
  const supported = new Set(props.definition?.supported_modes || [])
  const options = catalogStringOptions(props.definition, 'mode', FALLBACK_MODES)
    .filter((option) => SUPPORTED_MODES.has(option.value as ImageMode)
      && (!supported.size || supported.has(option.value)))
  return options.length ? options : [...FALLBACK_MODES]
})
const modelOptions = computed(() => {
  const fallback = isOpenAI.value ? OPENAI_IMAGE_RUNNER_MODELS : GROK_IMAGE_RUNNER_MODELS
  const allowed = new Set(fallback.map((option) => option.value))
  const options = catalogStringOptions(props.definition, 'model', fallback)
    .filter((option) => allowed.has(option.value))
  return options.length ? options : [...fallback]
})
const aspectRatioOptions = computed(() => {
  const fallback = isOpenAI.value ? OPENAI_IMAGE_RUNNER_RATIOS : GROK_IMAGE_RUNNER_RATIOS
  const allowed = new Set(fallback.map((option) => option.value))
  const options = catalogStringOptions(props.definition, 'aspect_ratio', fallback)
    .filter((option) => allowed.has(option.value))
  if (isOpenAI.value) return options.length ? options : [...fallback]
  const configured = new Map(options.map((option) => [option.value, option]))
  return fallback.map((option) => configured.get(option.value) || option)
})
const resolutionOptions = computed(() => {
  const options = catalogStringOptions(props.definition, 'resolution', FALLBACK_RESOLUTIONS)
    .filter((option) => SUPPORTED_RESOLUTIONS.has(option.value.toLowerCase()))
  return options.length ? options : [...FALLBACK_RESOLUTIONS]
})
const needsReference = computed(() => form.mode === 'image_edit')
const usesSourceImageRatio = computed(() => !isOpenAI.value
  && needsReference.value
  && files.value.length === 1)
const canRun = computed(() => !props.disabled
  && form.brief.trim().length >= 4
  && (!needsReference.value || files.value.length > 0))

function syncConstraints(): void {
  if (!modeOptions.value.some((option) => option.value === form.mode)) {
    form.mode = (modeOptions.value[0]?.value || 'text_to_image') as ImageMode
  }
  if (!modelOptions.value.some((option) => option.value === form.model)) {
    form.model = modelOptions.value[0]?.value || 'grok-imagine-image-quality'
  }
  if (!resolutionOptions.value.some((option) => option.value === form.resolution)) {
    form.resolution = resolutionOptions.value.find((option) => option.value === '2k')?.value
      || resolutionOptions.value[0]?.value
      || '2k'
  }
  if (!aspectRatioOptions.value.some((option) => option.value === form.aspect_ratio)) {
    form.aspect_ratio = aspectRatioOptions.value.find((option) => option.value === '1:1')?.value
      || aspectRatioOptions.value[0]?.value
      || '1:1'
  }
  form.n = normalizeImageGenerationCount(form.n, mediaPlatform.value)
}

watch(() => form.mode, (mode, previousMode) => {
  if (previousMode && mode !== previousMode) files.value = []
  syncConstraints()
}, { immediate: true })
watch(() => props.definition, syncConstraints)
watch(() => props.mediaPlatform, syncConstraints)

function submit(): void {
  localError.value = ''
  syncConstraints()
  if (!canRun.value) {
    localError.value = needsReference.value && !files.value.length
      ? '参考图精修需要至少上传一张图片。'
      : '请填写至少 4 个字的创作需求。'
    return
  }
  emit('run', {
    values: {
      brief: form.brief.trim(),
      mode: form.mode,
      purpose: form.purpose,
      style: form.style,
      composition: form.composition.trim(),
      lighting: form.lighting.trim(),
      palette: form.palette.trim(),
      must_keep: form.must_keep.trim(),
      avoid: form.avoid.trim(),
      ...(!usesSourceImageRatio.value ? { aspect_ratio: form.aspect_ratio } : {}),
      ...(isOpenAI.value
        ? { size: openAIImageSizeForRatio(form.aspect_ratio), quality: form.quality }
        : { resolution: form.resolution }),
      n: form.n,
      model: form.model,
      reference_count: files.value.length,
    },
    files: [...files.value],
  })
}
</script>

<template>
  <form class="runner-form" @submit.prevent="submit">
    <header class="runner-heading">
      <div><h2>灵感精修生图</h2><p>按当前设置直接生成，开启 AI 策划时会先整理画面方案。</p></div>
      <code class="runner-code">IMAGE / FLEX</code>
    </header>
    <fieldset class="runner-fields" :disabled="disabled">
      <label class="runner-field runner-field-wide">
        <span>创作需求 <em>*</em></span>
        <textarea v-model="form.brief" maxlength="4000" placeholder="主体、场景、用途、情绪，以及希望画面表达什么" />
      </label>
      <label class="runner-field"><span>生成方式</span><select v-model="form.mode"><option v-for="option in modeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>执行模型</span><select v-model="form.model"><option v-for="option in modelOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>使用场景</span><select v-model="form.purpose"><option>内容配图</option><option>产品展示</option><option>社交海报</option><option>品牌视觉</option></select></label>
      <label class="runner-field"><span>视觉方向</span><select v-model="form.style"><option>写实摄影</option><option>商业棚拍</option><option>电影质感</option><option>插画设计</option></select></label>
      <label class="runner-field"><span>构图要求</span><input v-model="form.composition" maxlength="300" placeholder="主体位置、景别、留白和视觉层级"></label>
      <label class="runner-field"><span>光线</span><input v-model="form.lighting" maxlength="200" placeholder="自然柔光、棚拍硬光、逆光等"></label>
      <label class="runner-field"><span>配色</span><input v-model="form.palette" maxlength="200" placeholder="主色、辅色和整体色温"></label>
      <label class="runner-field">
        <span>画面比例</span>
        <select v-if="usesSourceImageRatio" value="source" disabled><option value="source">跟随原图</option></select>
        <select v-else v-model="form.aspect_ratio"><option v-for="option in aspectRatioOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
      </label>
      <label v-if="!isOpenAI" class="runner-field"><span>清晰度</span><select v-model="form.resolution"><option v-for="option in resolutionOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label v-else class="runner-field"><span>图片质量</span><select v-model="form.quality"><option v-for="option in OPENAI_IMAGE_QUALITY_OPTIONS" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>生成数量</span><select v-model.number="form.n"><option v-for="count in countOptions" :key="count" :value="count">{{ count }} 张</option></select></label>
      <label class="runner-field runner-field-wide"><span>必须保留</span><textarea v-model="form.must_keep" maxlength="1200" placeholder="人物特征、商品结构、文字、Logo 或不能改变的细节" /></label>
      <label class="runner-field runner-field-wide"><span>禁止出现</span><textarea v-model="form.avoid" maxlength="1200" placeholder="不希望出现的元素、变形、文字错误或风格偏差" /></label>
      <div v-if="needsReference" class="runner-field runner-field-wide"><span>参考图片 <em>*</em></span><ReferenceUpload v-model="files" required :disabled="disabled" :max-files="5" /></div>
    </fieldset>
    <p v-if="localError" class="runner-error" role="alert">{{ localError }}</p>
    <div class="runner-actions"><button class="run-button" type="submit" :disabled="!canRun"><Icon name="sparkles" size="sm" />开始运行</button></div>
  </form>
</template>

<style scoped src="./runner.css"></style>
