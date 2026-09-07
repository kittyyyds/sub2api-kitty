import { computed, onBeforeUnmount, reactive, shallowRef, watch } from 'vue'

import {
  GROK_IMAGE_ASPECT_RATIOS,
  GROK_IMAGE_MODEL,
  GROK_IMAGE_QUALITY_MODEL,
  IMAGE_REFERENCE_LIMIT,
  KITTY_IMAGE_MODEL,
  imageGenerationCountOptions,
  normalizeImageGenerationCount,
  type GrokImageAspectRatio,
  type KittyImageModel,
  type KittyImageQuality,
} from '@/api/imageGeneration'
import {
  GROK_VIDEO_ASPECT_RATIOS,
  GROK_VIDEO_15_MODEL,
  GROK_VIDEO_MODEL,
  type GrokVideoAspectRatio,
  type GrokVideoModel,
  type GrokVideoResolution,
  type VideoGenerationStatus,
} from '@/api/videoGeneration'

import type { RunnerInputPayload } from '../types'

const MAX_REFERENCE_BYTES = 20 * 1024 * 1024
const ALLOWED_REFERENCE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])
const GROK_IMAGE_REFERENCE_LIMIT = 3
const VIDEO_REFERENCE_LIMIT = 1

export type ImageStudioMode = 'text' | 'image' | 'text-video' | 'image-video'

export interface MediaModelOption<TModel extends string = KittyImageModel> {
  value: TModel
  model: TModel
  labelSuffix: string
  descriptionSuffix: string
}

export interface ImageRatioOption {
  value: string
  label: string
  hint: string
  size: string | null
}

export interface ImageReference {
  id: string
  file: File
  previewUrl: string
}

export interface VideoStudioResult extends VideoGenerationStatus {
  contentUrl: string
}

interface ImageStudioForm {
  mode: ImageStudioMode
  prompt: string
  imageModel: KittyImageModel
  imageResolution: '1k' | '2k'
  ratio: string
  count: number
  quality: KittyImageQuality
  videoModel: GrokVideoModel
  videoResolution: GrokVideoResolution
  videoDuration: number
  customWidth: number
  customHeight: number
}

type Translate = (key: string, params?: Record<string, unknown>) => string

export interface MediaStudioOptions {
  mediaPlatform: () => string
  isRunning: () => boolean
}

const studioFallbackMessages: Record<string, string> = {
  'native.studio.error.sizeInteger': '宽度和高度必须填写整数。',
  'native.studio.error.sizePositive': '宽度和高度必须大于 0。',
  'native.studio.error.sizeMax': '最大边长不能超过 3840px。',
  'native.studio.error.sizeMultiple': '宽和高都必须是 16 的倍数。',
  'native.studio.error.sizeRatio': '长边 / 短边比例不能超过 3:1。',
  'native.studio.error.sizePixels': '总像素需在 655360 到 8294400 之间。',
  'native.studio.error.referenceSize': '单张参考图不能超过 20MB。',
  'native.studio.error.referenceType': '参考图仅支持 PNG、JPG 和 WebP。',
  'native.studio.error.referenceLimited': '当前模式仅使用前 {limit} 张参考图，其余图片已保留。',
  'native.studio.error.prompt': '请先输入提示词。',
  'native.studio.error.key': '请选择一个可用的用户 Key。',
  'native.studio.error.videoKey': '视频生成需要选择 Grok 媒体 Key。',
  'native.studio.error.referenceRequired': '图生图模式需要至少上传一张参考图。',
  'native.studio.error.videoReferenceRequired': '图生视频模式需要上传一张起始图片。',
  'native.studio.error.copy': '复制失败，请手动复制。',
}

function fallbackTranslate(key: string, params?: Record<string, unknown>): string {
  if (key === 'native.studio.error.referenceLimit') return `最多上传 ${params?.limit ?? ''} 张参考图。`
  if (key === 'native.studio.error.referenceTrimmed') return `只保留前 ${params?.limit ?? ''} 张参考图。`
  if (key === 'native.studio.error.referenceLimited') return `当前模式仅使用前 ${params?.limit ?? ''} 张参考图，其余图片已保留。`
  return studioFallbackMessages[key] ?? key
}

export const IMAGE_RATIO_OPTIONS: readonly ImageRatioOption[] = [
  { value: '1:1', label: '1:1 方图', hint: '头像、社媒配图、商品图', size: '1024x1024' },
  { value: '5:4', label: '5:4 横图', hint: '产品展示、封面', size: '1280x1024' },
  { value: '9:16', label: '9:16 竖版', hint: '短视频封面、手机海报', size: '864x1536' },
  { value: '21:9', label: '21:9 宽银幕', hint: '电影感宽幅画面', size: '1792x768' },
  { value: '16:9', label: '16:9 横版', hint: '海报、封面、展示图', size: '1536x864' },
  { value: '3:2', label: '3:2 摄影横图', hint: '摄影、场景图', size: '1536x1024' },
  { value: '4:3', label: '4:3 横图', hint: '通用内容配图', size: '1024x768' },
  { value: '4:5', label: '4:5 竖图', hint: '商品详情、人物', size: '1024x1280' },
  { value: '3:4', label: '3:4 竖图', hint: '人物、服饰、详情页', size: '768x1024' },
  { value: '2:3', label: '2:3 摄影竖图', hint: '竖版视觉、插画', size: '1024x1536' },
  { value: 'custom', label: '自定义尺寸', hint: '按输入的宽高请求', size: null },
]

const GROK_IMAGE_RATIO_DETAILS: Readonly<Record<GrokImageAspectRatio, Omit<ImageRatioOption, 'value'>>> = {
  '1:1': { label: '1:1 方图', hint: '方形画布', size: null },
  '16:9': { label: '16:9 横版', hint: '横版封面与场景', size: null },
  '9:16': { label: '9:16 竖版', hint: '手机海报与短视频封面', size: null },
  '4:3': { label: '4:3 横图', hint: '通用内容配图', size: null },
  '3:4': { label: '3:4 竖图', hint: '人物与商品详情', size: null },
  '3:2': { label: '3:2 摄影横图', hint: '摄影与场景图', size: null },
  '2:3': { label: '2:3 摄影竖图', hint: '竖版视觉与插画', size: null },
  '2:1': { label: '2:1 宽幅', hint: '宽幅横版画面', size: null },
  '1:2': { label: '1:2 长图', hint: '纵向长幅画面', size: null },
  '19.5:9': { label: '19.5:9 手机横屏', hint: '宽屏手机横版', size: null },
  '9:19.5': { label: '9:19.5 手机竖屏', hint: '全面屏手机竖版', size: null },
  '20:9': { label: '20:9 超宽横屏', hint: '超宽手机横版', size: null },
  '9:20': { label: '9:20 超长竖屏', hint: '超长手机竖版', size: null },
  auto: { label: '自动比例', hint: '由模型自动决定', size: null },
}

export const GROK_IMAGE_RATIO_OPTIONS: readonly ImageRatioOption[] = GROK_IMAGE_ASPECT_RATIOS.map((value) => ({
  value,
  ...GROK_IMAGE_RATIO_DETAILS[value],
}))

const GROK_IMAGE_ASPECT_RATIO_SET = new Set<string>(GROK_IMAGE_ASPECT_RATIOS)
const GROK_VIDEO_ASPECT_RATIO_SET = new Set<string>(GROK_VIDEO_ASPECT_RATIOS)

export const VIDEO_RATIO_OPTIONS: readonly ImageRatioOption[] = IMAGE_RATIO_OPTIONS.filter((option) => (
  GROK_VIDEO_ASPECT_RATIO_SET.has(option.value)
))

export const IMAGE_QUALITY_OPTIONS: ReadonlyArray<{ value: KittyImageQuality; label: string }> = [
  { value: 'auto', label: '自动' },
  { value: 'low', label: '草稿' },
  { value: 'medium', label: '标准' },
  { value: 'high', label: '高清' },
]

export const IMAGE_RESOLUTION_OPTIONS = ['1k', '2k'] as const
export const VIDEO_RESOLUTION_OPTIONS: readonly GrokVideoResolution[] = ['480p', '720p', '1080p']
export const VIDEO_DURATION_OPTIONS = [5, 8, 10, 12, 15] as const

const GPT_IMAGE_OPTIONS: readonly MediaModelOption[] = [{
  value: KITTY_IMAGE_MODEL,
  model: KITTY_IMAGE_MODEL,
  labelSuffix: 'gptImage2',
  descriptionSuffix: 'gptImage2',
}]

const GROK_IMAGE_OPTIONS: readonly MediaModelOption[] = [
  {
    value: GROK_IMAGE_QUALITY_MODEL,
    model: GROK_IMAGE_QUALITY_MODEL,
    labelSuffix: 'grokQuality',
    descriptionSuffix: 'grokQuality',
  },
  {
    value: GROK_IMAGE_MODEL,
    model: GROK_IMAGE_MODEL,
    labelSuffix: 'grokStandard',
    descriptionSuffix: 'grokStandard',
  },
]

const GROK_VIDEO_OPTIONS: readonly MediaModelOption<GrokVideoModel>[] = [
  {
    value: GROK_VIDEO_MODEL,
    model: GROK_VIDEO_MODEL,
    labelSuffix: 'grokVideo',
    descriptionSuffix: 'grokVideo',
  },
  {
    value: GROK_VIDEO_15_MODEL,
    model: GROK_VIDEO_15_MODEL,
    labelSuffix: 'grokVideo15',
    descriptionSuffix: 'grokVideo15',
  },
]

function referenceFileAllowed(file: File): boolean {
  if (ALLOWED_REFERENCE_TYPES.has(file.type.toLowerCase())) return true
  return !file.type && /\.(png|jpe?g|webp)$/i.test(file.name)
}

function createPreviewURL(file: File): string {
  return typeof URL !== 'undefined' && typeof URL.createObjectURL === 'function'
    ? URL.createObjectURL(file)
    : ''
}

function revokePreviewURL(url: string): void {
  if (url && typeof URL !== 'undefined' && typeof URL.revokeObjectURL === 'function') {
    URL.revokeObjectURL(url)
  }
}

function fallbackCopy(text: string): boolean {
  if (typeof document === 'undefined') return false
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'true')
  textarea.style.cssText = 'position:fixed;left:0;top:0;width:1px;height:1px;opacity:0;pointer-events:none'
  document.body.appendChild(textarea)
  textarea.focus({ preventScroll: true })
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)
  try {
    return typeof document.execCommand === 'function' && document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

export function isVideoStudioMode(mode: ImageStudioMode): mode is 'text-video' | 'image-video' {
  return mode === 'text-video' || mode === 'image-video'
}

function normalizeVideoAspectRatio(value: unknown): GrokVideoAspectRatio {
  const ratio = typeof value === 'string' ? value.trim() : ''
  return GROK_VIDEO_ASPECT_RATIO_SET.has(ratio) ? ratio as GrokVideoAspectRatio : '16:9'
}

function normalizeGrokImageAspectRatio(value: unknown): GrokImageAspectRatio {
  const ratio = typeof value === 'string' ? value.trim() : ''
  return GROK_IMAGE_ASPECT_RATIO_SET.has(ratio) ? ratio as GrokImageAspectRatio : '1:1'
}

export function validateCustomImageSize(
  width: number,
  height: number,
  translate: Translate = fallbackTranslate,
): string {
  if (!Number.isInteger(width) || !Number.isInteger(height)) return translate('native.studio.error.sizeInteger')
  if (width <= 0 || height <= 0) return translate('native.studio.error.sizePositive')
  if (Math.max(width, height) > 3840) return translate('native.studio.error.sizeMax')
  if (width % 16 !== 0 || height % 16 !== 0) return translate('native.studio.error.sizeMultiple')
  if (Math.max(width, height) / Math.min(width, height) > 3) return translate('native.studio.error.sizeRatio')
  const pixels = width * height
  if (pixels < 655360 || pixels > 8294400) return translate('native.studio.error.sizePixels')
  return ''
}

export function useMediaStudio(
  translate: Translate = fallbackTranslate,
  options: MediaStudioOptions,
) {
  const form = reactive<ImageStudioForm>({
    mode: 'text',
    prompt: '',
    imageModel: KITTY_IMAGE_MODEL,
    imageResolution: '1k',
    ratio: '5:4',
    count: 1,
    quality: 'auto',
    videoModel: GROK_VIDEO_MODEL,
    videoResolution: '480p',
    videoDuration: 8,
    customWidth: 1024,
    customHeight: 1024,
  })

  const imageReferences = shallowRef<ImageReference[]>([])
  const videoReferences = shallowRef<ImageReference[]>([])
  const generationError = shallowRef('')
  const referenceError = shallowRef('')
  const copyError = shallowRef('')
  const copiedValue = shallowRef('')

  let copyTimer: ReturnType<typeof setTimeout> | null = null
  let referenceSequence = 0

  const selectedPlatform = computed(() => options.mediaPlatform().trim().toLowerCase())
  const isRunning = computed(() => options.isRunning())
  const isVideoMode = computed(() => isVideoStudioMode(form.mode))
  const requiresReference = computed(() => form.mode === 'image' || form.mode === 'image-video')
  const isGrokImage = computed(() => selectedPlatform.value === 'grok')

  const availableImageModels = computed<readonly MediaModelOption[]>(() => (
    isGrokImage.value ? GROK_IMAGE_OPTIONS : GPT_IMAGE_OPTIONS
  ))
  const availableVideoModels = computed<readonly MediaModelOption<GrokVideoModel>[]>(() => GROK_VIDEO_OPTIONS)
  const selectedImageModel = computed<MediaModelOption>(() => (
    availableImageModels.value.find((model) => model.value === form.imageModel)
      || availableImageModels.value[0]
      || GPT_IMAGE_OPTIONS[0]
  ))
  const selectedVideoModel = computed<MediaModelOption<GrokVideoModel>>(() => (
    availableVideoModels.value.find((model) => model.value === form.videoModel)
      || GROK_VIDEO_OPTIONS[0]
  ))
  const selectedModel = computed(() => isVideoMode.value
    ? selectedVideoModel.value
    : selectedImageModel.value)
  const imageCountOptions = computed(() => imageGenerationCountOptions(selectedPlatform.value))

  const referenceLimit = computed(() => {
    if (form.mode === 'image-video') return VIDEO_REFERENCE_LIMIT
    if (form.mode === 'image' && isGrokImage.value) return GROK_IMAGE_REFERENCE_LIMIT
    return IMAGE_REFERENCE_LIMIT
  })
  const activeReferencePool = computed(() => isVideoMode.value
    ? videoReferences.value
    : imageReferences.value)
  const references = computed(() => activeReferencePool.value.slice(0, referenceLimit.value))
  const usesSourceImageRatio = computed(() => !isVideoMode.value
    && isGrokImage.value
    && form.mode === 'image'
    && references.value.length === 1)
  const availableVideoResolutions = computed<readonly GrokVideoResolution[]>(() => (
    (form.mode === 'text-video' || form.mode === 'image-video')
      && form.videoModel === GROK_VIDEO_15_MODEL
      ? VIDEO_RESOLUTION_OPTIONS
      : VIDEO_RESOLUTION_OPTIONS.slice(0, 2)
  ))
  const sizeError = computed(() => !isVideoMode.value && !isGrokImage.value && form.ratio === 'custom'
    ? validateCustomImageSize(Number(form.customWidth), Number(form.customHeight), translate)
    : '')
  const requestSize = computed(() => {
    if (isVideoMode.value) return `${form.videoResolution} · ${form.videoDuration}s · ${form.ratio}`
    if (usesSourceImageRatio.value) {
      return `${form.imageResolution.toUpperCase()} · ${translate('native.studio.ratioOption.source')}`
    }
    if (isGrokImage.value) return `${form.imageResolution.toUpperCase()} · ${form.ratio}`
    if (form.ratio === 'custom') return `${Math.round(Number(form.customWidth))}x${Math.round(Number(form.customHeight))}`
    return IMAGE_RATIO_OPTIONS.find((option) => option.value === form.ratio)?.size || '1024x1024'
  })
  const canGenerate = computed(() => !isRunning.value
    && !!form.prompt.trim()
    && !!selectedPlatform.value
    && (!isVideoMode.value || selectedPlatform.value === 'grok')
    && !sizeError.value
    && (!requiresReference.value || references.value.length > 0))

  function setActiveReferencePool(items: ImageReference[]): void {
    if (isVideoMode.value) videoReferences.value = items
    else imageReferences.value = items
  }

  function syncReferenceLimitMessage(): void {
    referenceError.value = activeReferencePool.value.length > referenceLimit.value
      ? translate('native.studio.error.referenceLimited', { limit: referenceLimit.value })
      : ''
  }

  function syncImageModelForContext(): void {
    if (isVideoMode.value) return
    if (!availableImageModels.value.some((option) => option.value === form.imageModel)) {
      form.imageModel = availableImageModels.value[0]?.value || KITTY_IMAGE_MODEL
    }
  }

  function syncVideoResolutionForContext(): void {
    if (!availableVideoResolutions.value.includes(form.videoResolution)) form.videoResolution = '480p'
  }

  function syncImageRatioForContext(): void {
    if (isVideoMode.value) return
    const options = isGrokImage.value ? GROK_IMAGE_RATIO_OPTIONS : IMAGE_RATIO_OPTIONS
    if (!options.some((option) => option.value === form.ratio)) {
      form.ratio = isGrokImage.value ? '1:1' : '5:4'
    }
  }

  function syncImageCountForContext(): void {
    form.count = normalizeImageGenerationCount(form.count, selectedPlatform.value)
  }

  function setMode(mode: ImageStudioMode): void {
    if (isRunning.value || form.mode === mode) return
    const previousMode = form.mode
    form.mode = mode
    if (isVideoStudioMode(mode)) {
      form.ratio = isVideoStudioMode(previousMode)
        ? normalizeVideoAspectRatio(form.ratio)
        : '16:9'
    }
    if (mode === 'image-video' && previousMode !== 'image-video') form.videoModel = GROK_VIDEO_15_MODEL
    if (mode === 'text-video' && previousMode !== 'text-video') form.videoModel = GROK_VIDEO_MODEL
    syncVideoResolutionForContext()
    syncImageModelForContext()
    syncImageRatioForContext()
    syncReferenceLimitMessage()
    generationError.value = ''
  }

  function addReferenceFiles(fileList: FileList | File[]): void {
    if (isRunning.value) return
    referenceError.value = ''
    const incoming = Array.from(fileList)
    const pool = activeReferencePool.value
    const remainingSlots = referenceLimit.value - pool.length
    if (remainingSlots <= 0) {
      referenceError.value = translate('native.studio.error.referenceLimit', { limit: referenceLimit.value })
      return
    }

    const accepted: ImageReference[] = []
    let invalidType = false
    let oversized = false
    for (const file of incoming) {
      if (!referenceFileAllowed(file)) {
        invalidType = true
        continue
      }
      if (file.size > MAX_REFERENCE_BYTES) {
        oversized = true
        continue
      }
      if (accepted.length >= remainingSlots) break
      referenceSequence += 1
      accepted.push({
        id: `reference-${referenceSequence}`,
        file,
        previewUrl: createPreviewURL(file),
      })
    }
    setActiveReferencePool([...pool, ...accepted])

    if (incoming.length > remainingSlots) {
      referenceError.value = translate('native.studio.error.referenceTrimmed', { limit: referenceLimit.value })
    } else if (oversized) {
      referenceError.value = translate('native.studio.error.referenceSize')
    } else if (invalidType) {
      referenceError.value = translate('native.studio.error.referenceType')
    }
  }

  function removeReference(id: string): void {
    if (isRunning.value) return
    const pool = activeReferencePool.value
    const target = pool.find((item) => item.id === id)
    if (target) revokePreviewURL(target.previewUrl)
    setActiveReferencePool(pool.filter((item) => item.id !== id))
    referenceError.value = ''
  }

  function clearAllReferences(): void {
    imageReferences.value.forEach((item) => revokePreviewURL(item.previewUrl))
    videoReferences.value.forEach((item) => revokePreviewURL(item.previewUrl))
    imageReferences.value = []
    videoReferences.value = []
    referenceError.value = ''
  }

  function buildApplicationPayload(): RunnerInputPayload | null {
    generationError.value = ''

    const mode = form.mode
    const videoMode = isVideoStudioMode(mode)
    const prompt = form.prompt.trim()
    if (!prompt) {
      generationError.value = translate('native.studio.error.prompt')
      return null
    }
    if (!selectedPlatform.value) {
      generationError.value = translate('native.studio.error.key')
      return null
    }
    if (videoMode && selectedPlatform.value !== 'grok') {
      generationError.value = translate('native.studio.error.videoKey')
      return null
    }
    const referenceFiles = references.value.map((item) => item.file)
    if (requiresReference.value && !referenceFiles.length) {
      generationError.value = translate(mode === 'image-video'
        ? 'native.studio.error.videoReferenceRequired'
        : 'native.studio.error.referenceRequired')
      return null
    }
    if (sizeError.value) {
      generationError.value = sizeError.value
      return null
    }
    if (videoMode) syncVideoResolutionForContext()

    const count = normalizeImageGenerationCount(form.count, selectedPlatform.value)
    const values: Record<string, unknown> = {
      prompt,
      mode: videoMode
        ? mode === 'image-video' ? 'image_to_video' : 'text_to_video'
        : mode === 'image' ? 'image_edit' : 'text_to_image',
      model: videoMode ? form.videoModel : selectedImageModel.value.model,
      n: count,
      reference_count: referenceFiles.length,
    }
    if (videoMode) {
      Object.assign(values, {
        duration: Math.max(1, Math.min(15, Math.round(Number(form.videoDuration)) || 8)),
        resolution: form.videoResolution,
        aspect_ratio: normalizeVideoAspectRatio(form.ratio),
      })
    } else if (isGrokImage.value) {
      Object.assign(values, {
        resolution: form.imageResolution,
        ...(!usesSourceImageRatio.value
          ? { aspect_ratio: normalizeGrokImageAspectRatio(form.ratio) }
          : {}),
      })
    } else {
      Object.assign(values, {
        size: requestSize.value,
        quality: form.quality,
      })
    }
    return { values, files: referenceFiles }
  }

  async function copyText(value: string): Promise<boolean> {
    const text = value.trim()
    copyError.value = ''
    if (!text) return false

    let copied = false
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(text)
        copied = true
      } catch {
        copied = fallbackCopy(text)
      }
    } else {
      copied = fallbackCopy(text)
    }

    if (!copied) {
      copyError.value = translate('native.studio.error.copy')
      return false
    }
    copiedValue.value = text
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copiedValue.value = ''
      copyTimer = null
    }, 1600)
    return true
  }

  watch(selectedPlatform, () => {
    syncImageModelForContext()
    syncImageCountForContext()
    syncImageRatioForContext()
    syncReferenceLimitMessage()
  }, { immediate: true })
  watch(() => form.videoModel, syncVideoResolutionForContext)

  onBeforeUnmount(() => {
    clearAllReferences()
    if (copyTimer) clearTimeout(copyTimer)
  })

  return {
    form,
    references,
    generationError,
    referenceError,
    copyError,
    copiedValue,
    selectedModel,
    imageCountOptions,
    selectedPlatform,
    availableImageModels,
    availableVideoModels,
    availableVideoResolutions,
    isVideoMode,
    requiresReference,
    isGrokImage,
    usesSourceImageRatio,
    referenceLimit,
    sizeError,
    requestSize,
    canGenerate,
    setMode,
    addReferenceFiles,
    removeReference,
    buildApplicationPayload,
    copyText,
  }
}
