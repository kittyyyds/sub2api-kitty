export type StudioMode = 'text-image' | 'image-image' | 'text-video' | 'image-video'
export type StudioResultType = 'image' | 'video'
export type StudioResolution = '1k' | '2k' | '4k'
export type StudioVideoResolution = '480p' | '720p' | '1080p'
export type StudioFormat = 'png' | 'jpeg' | 'webp'
export type StudioModelStrategy = 'openai-image' | 'gemini-image' | 'claude-svg' | 'grok-video' | 'video-gateway'

export interface StudioModelOption {
  id: string
  label: string
  description: string
  strategy: StudioModelStrategy
  type: StudioResultType
}

export interface StudioForm {
  mode: StudioMode
  prompt: string
  keyId: string
  model: string
  count: 1 | 2 | 4
  ratio: '1:1' | '16:9' | '9:16' | '3:2' | '2:3'
  resolution: StudioResolution
  videoResolution: StudioVideoResolution
  videoDuration: number
  format: StudioFormat
  background: 'auto' | 'opaque' | 'transparent'
}

export interface StudioResult {
  id: string
  type: StudioResultType
  source: string
  mimeType: string
  prompt: string
  model: string
  mode: StudioMode
  keyId: string
  createdAt: number
  width?: number
  height?: number
  requestId?: string
  blob?: Blob
}

export interface StudioProgressItem {
  id: string
  index: number
  status: 'queued' | 'running' | 'completed' | 'failed' | 'stopped'
  message: string
}
