import {
  GROK_IMAGE_ASPECT_RATIOS,
  GROK_IMAGE_MODEL,
  GROK_IMAGE_QUALITY_MODEL,
  KITTY_IMAGE_MODEL,
  imageGenerationCountOptions,
  normalizeImageGenerationCount,
} from '@/api/imageGeneration'

export type ImageMediaPlatform = 'grok' | 'openai'

export interface ImageRunnerOption {
  label: string
  value: string
}

export const GROK_IMAGE_RUNNER_MODELS: readonly ImageRunnerOption[] = [
  { label: 'Grok Imagine Image Quality', value: GROK_IMAGE_QUALITY_MODEL },
  { label: 'Grok Imagine Image', value: GROK_IMAGE_MODEL },
]

export const OPENAI_IMAGE_RUNNER_MODELS: readonly ImageRunnerOption[] = [
  { label: 'GPT Image 2', value: KITTY_IMAGE_MODEL },
]

export const GROK_IMAGE_RUNNER_RATIOS: readonly ImageRunnerOption[] = GROK_IMAGE_ASPECT_RATIOS.map((value) => ({
  label: value === 'auto' ? '自动' : value,
  value,
}))

export const OPENAI_IMAGE_RUNNER_RATIOS: readonly ImageRunnerOption[] = [
  { label: '1:1', value: '1:1' },
  { label: '5:4', value: '5:4' },
  { label: '9:16', value: '9:16' },
  { label: '21:9', value: '21:9' },
  { label: '16:9', value: '16:9' },
  { label: '3:2', value: '3:2' },
  { label: '4:3', value: '4:3' },
  { label: '4:5', value: '4:5' },
  { label: '3:4', value: '3:4' },
  { label: '2:3', value: '2:3' },
]

export const OPENAI_IMAGE_QUALITY_OPTIONS: readonly ImageRunnerOption[] = [
  { label: '自动', value: 'auto' },
  { label: '草稿', value: 'low' },
  { label: '标准', value: 'medium' },
  { label: '高清', value: 'high' },
]

const OPENAI_IMAGE_SIZES: Readonly<Record<string, string>> = {
  '1:1': '1024x1024',
  '5:4': '1280x1024',
  '9:16': '864x1536',
  '21:9': '1792x768',
  '16:9': '1536x864',
  '3:2': '1536x1024',
  '4:3': '1024x768',
  '4:5': '1024x1280',
  '3:4': '768x1024',
  '2:3': '1024x1536',
}

export function normalizeImageMediaPlatform(platform?: string): ImageMediaPlatform {
  return platform?.toLowerCase() === 'openai' ? 'openai' : 'grok'
}

export function openAIImageSizeForRatio(ratio: string): string {
  return OPENAI_IMAGE_SIZES[ratio] || OPENAI_IMAGE_SIZES['1:1']
}

export { imageGenerationCountOptions, normalizeImageGenerationCount }
