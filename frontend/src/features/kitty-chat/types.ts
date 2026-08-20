export type ChatRole = 'system' | 'user' | 'assistant'

export interface ChatAttachmentMeta {
  name: string
  type: string
  size: number
}

export interface ChatMessage {
  id: string
  role: ChatRole
  content: string
  reasoning?: string
  attachments?: ChatAttachmentMeta[]
  createdAt: number
  error?: string
}

export interface ChatConversation {
  id: string
  title: string
  keyId: string
  model: string
  messages: ChatMessage[]
  createdAt: number
  updatedAt: number
}

export interface ChatSettings {
  systemPrompt: string
  temperature: number
  topP: number
  maxTokens: number
  reasoningEffort: 'auto' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'
}

export interface ChatUsage {
  promptTokens: number
  completionTokens: number
  totalTokens: number
}

export type ChatAPITextContent = string | Array<
  | { type: 'text'; text: string }
  | { type: 'image_url'; image_url: { url: string } }
>

export interface ChatAPIMessage {
  role: ChatRole
  content: ChatAPITextContent
}

export interface ChatStreamCallbacks {
  onText: (text: string) => void
  onReasoning?: (text: string) => void
  onUsage?: (usage: ChatUsage) => void
}
