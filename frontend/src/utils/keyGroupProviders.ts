import type { GroupPlatform } from '@/types'

export type KeyGroupProvider = 'anthropic' | 'openai' | 'gemini' | 'grok' | 'domestic' | 'video' | 'other'

export const KEY_GROUP_PROVIDERS = ['anthropic', 'openai', 'gemini', 'grok', 'domestic', 'video', 'other'] as const

// Classify by the configured upstream platform, never by a group's display name.
const PROVIDER_BY_PLATFORM: Record<GroupPlatform, KeyGroupProvider> = {
  anthropic: 'anthropic',
  openai: 'openai',
  kimi: 'domestic',
  zhipu: 'domestic',
  deepseek: 'domestic',
  minimax: 'domestic',
  gemini: 'gemini',
  grok: 'grok',
  video: 'video',
  antigravity: 'other',
  composite: 'other',
  opencode_go: 'other',
  typesafe: 'other'
}

export function getKeyGroupProvider(platform: GroupPlatform): KeyGroupProvider {
  return PROVIDER_BY_PLATFORM[platform] ?? 'other'
}

// Collections use representative provider marks rather than an invented brand logo.
export const KEY_GROUP_PROVIDER_ICONS: Record<KeyGroupProvider, GroupPlatform[]> = {
  anthropic: ['anthropic'],
  openai: ['openai'],
  domestic: ['deepseek', 'kimi'],
  gemini: ['gemini'],
  grok: ['grok'],
  video: ['video'],
  other: ['antigravity', 'opencode_go', 'composite']
}
