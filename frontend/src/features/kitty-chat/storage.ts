import type { ChatConversation, ChatMessage, ChatSettings } from './types'

const CONVERSATIONS_KEY = 'kitty-chat:conversations:v1'
const SETTINGS_KEY = 'kitty-chat:settings:v1'
const MAX_CONVERSATIONS = 50
const MAX_MESSAGES = 200
const MAX_CONTENT_LENGTH = 200_000

export const DEFAULT_CHAT_SETTINGS: ChatSettings = {
  systemPrompt: '',
  temperature: 0.7,
  topP: 1,
  maxTokens: 8192,
  reasoningEffort: 'auto',
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function numberOr(value: unknown, fallback: number): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function cleanMessage(value: unknown): ChatMessage | null {
  if (!isRecord(value) || !['system', 'user', 'assistant'].includes(String(value.role))) return null
  const content = typeof value.content === 'string' ? value.content.slice(0, MAX_CONTENT_LENGTH) : ''
  const id = typeof value.id === 'string' && value.id ? value.id : crypto.randomUUID()
  const attachments = Array.isArray(value.attachments)
    ? value.attachments.flatMap((raw) => {
        if (!isRecord(raw) || typeof raw.name !== 'string') return []
        return [{
          name: raw.name.slice(0, 260),
          type: typeof raw.type === 'string' ? raw.type.slice(0, 120) : '',
          size: Math.max(0, numberOr(raw.size, 0)),
        }]
      }).slice(0, 10)
    : undefined
  return {
    id,
    role: value.role as ChatMessage['role'],
    content,
    reasoning: typeof value.reasoning === 'string' ? value.reasoning.slice(0, MAX_CONTENT_LENGTH) : undefined,
    attachments,
    createdAt: numberOr(value.createdAt, Date.now()),
    error: typeof value.error === 'string' ? value.error.slice(0, 1000) : undefined,
  }
}

function cleanConversation(value: unknown): ChatConversation | null {
  if (!isRecord(value) || typeof value.id !== 'string') return null
  const messages = Array.isArray(value.messages)
    ? value.messages.map(cleanMessage).filter((message): message is ChatMessage => !!message).slice(-MAX_MESSAGES)
    : []
  return {
    id: value.id,
    title: typeof value.title === 'string' && value.title.trim() ? value.title.slice(0, 80) : '新对话',
    keyId: typeof value.keyId === 'string' ? value.keyId : '',
    model: typeof value.model === 'string' ? value.model : '',
    messages,
    createdAt: numberOr(value.createdAt, Date.now()),
    updatedAt: numberOr(value.updatedAt, Date.now()),
  }
}

export function loadConversations(storage: Storage = localStorage): ChatConversation[] {
  try {
    const parsed = JSON.parse(storage.getItem(CONVERSATIONS_KEY) || '[]') as unknown
    if (!Array.isArray(parsed)) return []
    return parsed
      .map(cleanConversation)
      .filter((conversation): conversation is ChatConversation => !!conversation)
      .sort((a, b) => b.updatedAt - a.updatedAt)
      .slice(0, MAX_CONVERSATIONS)
  } catch {
    return []
  }
}

export function saveConversations(conversations: ChatConversation[], storage: Storage = localStorage): void {
  const clean = conversations
    .map(cleanConversation)
    .filter((conversation): conversation is ChatConversation => !!conversation)
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, MAX_CONVERSATIONS)
  try {
    storage.setItem(CONVERSATIONS_KEY, JSON.stringify(clean))
  } catch {
    const compact = clean.slice(0, 10).map((conversation) => ({
      ...conversation,
      messages: conversation.messages.slice(-20).map((message) => ({
        ...message,
        content: message.content.slice(-20_000),
        reasoning: message.reasoning?.slice(-10_000),
      })),
    }))
    try {
      storage.setItem(CONVERSATIONS_KEY, JSON.stringify(compact))
    } catch {
      const minimal = compact.slice(0, 5).map((conversation) => ({
        ...conversation,
        messages: conversation.messages.slice(-10).map((message) => ({
          ...message,
          content: message.content.slice(-4_000),
          reasoning: undefined,
        })),
      }))
      try {
        storage.setItem(CONVERSATIONS_KEY, JSON.stringify(minimal))
      } catch {
        // Keep the current in-memory conversation when browser storage is unavailable.
      }
    }
  }
}

export function loadChatSettings(storage: Storage = localStorage): ChatSettings {
  try {
    const value = JSON.parse(storage.getItem(SETTINGS_KEY) || '{}') as unknown
    if (!isRecord(value)) return { ...DEFAULT_CHAT_SETTINGS }
    const reasoning = String(value.reasoningEffort || 'auto') as ChatSettings['reasoningEffort']
    return {
      systemPrompt: typeof value.systemPrompt === 'string' ? value.systemPrompt.slice(0, 20_000) : '',
      temperature: Math.max(0, Math.min(2, numberOr(value.temperature, DEFAULT_CHAT_SETTINGS.temperature))),
      topP: Math.max(0, Math.min(1, numberOr(value.topP, DEFAULT_CHAT_SETTINGS.topP))),
      maxTokens: Math.max(1, Math.min(200_000, Math.round(numberOr(value.maxTokens, DEFAULT_CHAT_SETTINGS.maxTokens)))),
      reasoningEffort: ['auto', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh'].includes(reasoning)
        ? reasoning
        : 'auto',
    }
  } catch {
    return { ...DEFAULT_CHAT_SETTINGS }
  }
}

export function saveChatSettings(settings: ChatSettings, storage: Storage = localStorage): void {
  storage.setItem(SETTINGS_KEY, JSON.stringify(settings))
}
