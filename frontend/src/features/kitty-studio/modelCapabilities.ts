import type { StudioMode, StudioModelOption } from './types'

export const GPT_IMAGE_MODEL = 'gpt-image-2'
export const GROK_IMAGE_MODEL = 'grok-imagine-image'
export const GROK_IMAGE_20_MODEL = 'grok-imagine-image-2.0'
export const GROK_IMAGE_QUALITY_MODEL = 'grok-imagine-image-quality'
export const GROK_VIDEO_MODEL = 'grok-imagine-video'
export const GROK_VIDEO_15_MODEL = 'grok-imagine-video-1.5'

const GPT_IMAGE_OPTION: StudioModelOption = {
  id: GPT_IMAGE_MODEL,
  label: 'GPT Image 2',
  description: 'OpenAI 原生栅格生图',
  strategy: 'openai-image',
  type: 'image',
}

const GROK_IMAGE_OPTIONS: StudioModelOption[] = [
  {
    id: GROK_IMAGE_MODEL,
    label: 'Grok Imagine',
    description: '快速生图与参考图编辑',
    strategy: 'openai-image',
    type: 'image',
  },
  {
    id: GROK_IMAGE_20_MODEL,
    label: 'Grok Imagine 2.0',
    description: '新一代 Grok 原生图片模型',
    strategy: 'openai-image',
    type: 'image',
  },
  {
    id: GROK_IMAGE_QUALITY_MODEL,
    label: 'Grok Imagine（高质量）',
    description: '高质量图片生成与编辑',
    strategy: 'openai-image',
    type: 'image',
  },
]

const GROK_VIDEO_OPTIONS: StudioModelOption[] = [
  {
    id: GROK_VIDEO_MODEL,
    label: 'Grok Imagine Video',
    description: '文生视频与图生视频',
    strategy: 'grok-video',
    type: 'video',
  },
  {
    id: GROK_VIDEO_15_MODEL,
    label: 'Grok Imagine Video 1.5',
    description: '支持更高分辨率的视频模型',
    strategy: 'grok-video',
    type: 'video',
  },
]

function normalizedPlatform(value: string): string {
  return value.trim().toLowerCase()
}

function uniqueOptions(options: StudioModelOption[]): StudioModelOption[] {
  const seen = new Set<string>()
  return options.filter((option) => {
    if (seen.has(option.id)) return false
    seen.add(option.id)
    return true
  })
}

function geminiOption(id: string): StudioModelOption {
  return {
    id,
    label: id,
    description: 'Google 原生图片输出',
    strategy: 'gemini-image',
    type: 'image',
  }
}

function claudeOption(id: string): StudioModelOption {
  return {
    id,
    label: id,
    description: 'Claude SVG 矢量插画',
    strategy: 'claude-svg',
    type: 'image',
  }
}

export function isVideoMode(mode: StudioMode): boolean {
  return mode === 'text-video' || mode === 'image-video'
}

export function requiresReference(mode: StudioMode): boolean {
  return mode === 'image-image' || mode === 'image-video'
}

export function studioModelOptions(
  platform: string,
  availableModelIds: string[],
  mode: StudioMode,
): StudioModelOption[] {
  const normalized = normalizedPlatform(platform)
  if (isVideoMode(mode)) {
    return ['grok', 'composite'].includes(normalized) ? GROK_VIDEO_OPTIONS : []
  }

  const ids = new Set(availableModelIds.map((id) => id.trim()).filter(Boolean))
  const options: StudioModelOption[] = []
  if (normalized === 'openai' || normalized === 'composite' || ids.has(GPT_IMAGE_MODEL)) {
    options.push(GPT_IMAGE_OPTION)
  }
  if (normalized === 'grok' || normalized === 'composite' || [...ids].some((id) => id.startsWith('grok-imagine-image'))) {
    options.push(...GROK_IMAGE_OPTIONS)
  }

  for (const id of ids) {
    if ((normalized === 'gemini' || normalized === 'antigravity' || normalized === 'composite')
      && /(?:gemini|imagen|banana).*(?:image|preview)|(?:image|imagen|banana)/i.test(id)) {
      options.push(geminiOption(id))
    }
    if ((normalized === 'claude' || normalized === 'anthropic' || normalized === 'composite') && /^claude-/i.test(id)) {
      options.push(claudeOption(id))
    }
  }
  return uniqueOptions(options)
}

export function studioModelById(options: StudioModelOption[], id: string): StudioModelOption | null {
  return options.find((option) => option.id === id) || null
}
