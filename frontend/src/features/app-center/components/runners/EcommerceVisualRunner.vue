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
  { label: '参考商品图生成', value: 'image_edit' },
  { label: '纯文字生成', value: 'text_to_image' },
] as const
const FALLBACK_RESOLUTIONS = GROK_IMAGE_RESOLUTIONS.map((value) => ({ label: value.toUpperCase(), value }))
const SUPPORTED_MODES = new Set<ImageMode>(FALLBACK_MODES.map((option) => option.value))
const SUPPORTED_RESOLUTIONS = new Set<string>(GROK_IMAGE_RESOLUTIONS)
const form = reactive({
  product_name: '',
  selling_points: '',
  audience: '',
  placement: 'hero',
  marketing_goal: '突出核心卖点并提升点击与转化',
  composition: '商品主体突出，保留文案安全区',
  lighting: '商业棚拍柔光',
  palette: '',
  copy_requirements: '',
  must_keep: '保持商品外观、包装、商标和文字准确',
  avoid: '',
  aspect_ratio: '1:1',
  resolution: '2k',
  quality: 'auto',
  n: 1,
  mode: 'image_edit' as ImageMode,
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
  && !!form.product_name.trim()
  && !!form.selling_points.trim()
  && (!needsReference.value || files.value.length > 0))

function syncConstraints(): void {
  if (!modeOptions.value.some((option) => option.value === form.mode)) {
    form.mode = (modeOptions.value[0]?.value || 'image_edit') as ImageMode
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
      ? '参考商品图生成需要至少上传一张商品图。'
      : '请填写商品名称和核心卖点。'
    return
  }
  emit('run', {
    values: {
      product_name: form.product_name.trim(),
      selling_points: form.selling_points.trim(),
      audience: form.audience.trim(),
      placement: form.placement,
      marketing_goal: form.marketing_goal.trim(),
      composition: form.composition.trim(),
      lighting: form.lighting.trim(),
      palette: form.palette.trim(),
      copy_requirements: form.copy_requirements.trim(),
      must_keep: form.must_keep.trim(),
      avoid: form.avoid.trim(),
      ...(!usesSourceImageRatio.value ? { aspect_ratio: form.aspect_ratio } : {}),
      ...(isOpenAI.value
        ? { size: openAIImageSizeForRatio(form.aspect_ratio), quality: form.quality }
        : { resolution: form.resolution }),
      n: form.n,
      mode: form.mode,
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
      <div><h2>电商视觉策划</h2><p>按当前设置生成商品视觉，开启 AI 策划时会先提炼卖点与构图。</p></div>
      <code class="runner-code">COMMERCE / FLEX</code>
    </header>
    <fieldset class="runner-fields" :disabled="disabled">
      <label class="runner-field"><span>商品名称 <em>*</em></span><input v-model="form.product_name" maxlength="120" placeholder="例如：轻量防晒外套"></label>
      <label class="runner-field"><span>目标人群</span><input v-model="form.audience" maxlength="200" placeholder="例如：城市通勤女性"></label>
      <label class="runner-field runner-field-wide"><span>核心卖点 <em>*</em></span><textarea v-model="form.selling_points" maxlength="2000" placeholder="材质、功能、价格优势、必须出现的文字或元素" /></label>
      <label class="runner-field"><span>生成方式</span><select v-model="form.mode"><option v-for="option in modeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>执行模型</span><select v-model="form.model"><option v-for="option in modelOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>投放版位</span><select v-model="form.placement"><option value="hero">商品主图</option><option value="product_detail">详情页头图</option><option value="social">社交广告</option><option value="advertisement">广告素材</option></select></label>
      <label class="runner-field"><span>营销目标</span><input v-model="form.marketing_goal" maxlength="300" placeholder="提升点击、解释功能、建立高级感等"></label>
      <label class="runner-field"><span>构图要求</span><input v-model="form.composition" maxlength="300" placeholder="主体位置、景别、留白和文案区"></label>
      <label class="runner-field"><span>光线</span><input v-model="form.lighting" maxlength="200" placeholder="棚拍柔光、自然光、轮廓光等"></label>
      <label class="runner-field"><span>配色</span><input v-model="form.palette" maxlength="200" placeholder="品牌色、背景色和色温"></label>
      <label class="runner-field">
        <span>画面比例</span>
        <select v-if="usesSourceImageRatio" value="source" disabled><option value="source">跟随原图</option></select>
        <select v-else v-model="form.aspect_ratio"><option v-for="option in aspectRatioOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
      </label>
      <label v-if="!isOpenAI" class="runner-field"><span>清晰度</span><select v-model="form.resolution"><option v-for="option in resolutionOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label v-else class="runner-field"><span>图片质量</span><select v-model="form.quality"><option v-for="option in OPENAI_IMAGE_QUALITY_OPTIONS" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label class="runner-field"><span>生成数量</span><select v-model.number="form.n"><option v-for="count in countOptions" :key="count" :value="count">{{ count }} 张</option></select></label>
      <label class="runner-field runner-field-wide"><span>文案要求</span><textarea v-model="form.copy_requirements" maxlength="1000" placeholder="需要出现的短文案、语言、位置；不需要文字可留空" /></label>
      <label class="runner-field runner-field-wide"><span>必须保留</span><textarea v-model="form.must_keep" maxlength="1200" placeholder="包装、Logo、商品数量、比例和不能改变的结构" /></label>
      <label class="runner-field runner-field-wide"><span>禁止出现</span><textarea v-model="form.avoid" maxlength="1200" placeholder="错误文字、商品变形、额外配件或不适合的元素" /></label>
      <div v-if="needsReference" class="runner-field runner-field-wide"><span>商品参考图 <em>*</em></span><ReferenceUpload v-model="files" required :disabled="disabled" :max-files="5" /></div>
    </fieldset>
    <p v-if="localError" class="runner-error" role="alert">{{ localError }}</p>
    <div class="runner-actions"><button class="run-button" type="submit" :disabled="!canRun"><Icon name="sparkles" size="sm" />生成商品视觉</button></div>
  </form>
</template>

<style scoped src="./runner.css"></style>
