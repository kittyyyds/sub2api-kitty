import type { AppPresentation, ApplicationSlug } from './types'

export const APP_PRESENTATIONS: Record<ApplicationSlug, AppPresentation> = {
  'free-media-studio': {
    slug: 'free-media-studio',
    name: '自由媒体工作台',
    description: '直接使用原始提示词，自由选择 GPT Image、Grok 图片或视频模型。',
    category: 'media',
    version: '1.0',
    icon: 'swatches',
    requiresReference: false,
    acceptsMultipleReferences: true,
  },
  'inspiration-image-refiner': {
    slug: 'inspiration-image-refiner',
    name: '灵感精修生图',
    description: '把零散灵感整理成可执行的视觉方案，再生成或精修图片。',
    category: 'image',
    version: '1.0',
    icon: 'lightbulb',
    requiresReference: false,
    acceptsMultipleReferences: true,
  },
  'ecommerce-visual-planner': {
    slug: 'ecommerce-visual-planner',
    name: '电商视觉策划',
    description: '分析商品卖点、受众与投放场景，输出适合转化的商品视觉。',
    category: 'commerce',
    version: '1.0',
    icon: 'shoppingBag',
    requiresReference: true,
    acceptsMultipleReferences: true,
  },
  'grok-video-director': {
    slug: 'grok-video-director',
    name: 'Grok 视频导演',
    description: '先完成镜头与节奏规划，再执行文生视频、图生视频、编辑或续写。',
    category: 'video',
    version: '1.0',
    icon: 'videoCamera',
    requiresReference: false,
    acceptsMultipleReferences: true,
  },
}

export function isSupportedApplicationSlug(value: unknown): value is ApplicationSlug {
  return typeof value === 'string' && Object.prototype.hasOwnProperty.call(APP_PRESENTATIONS, value)
}
