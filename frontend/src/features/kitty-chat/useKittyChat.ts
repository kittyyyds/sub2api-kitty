import { computed, onBeforeUnmount, shallowRef } from 'vue'

import { keysAPI } from '@/api/keys'
import { fileToDataURL, fileToText, isAbortError, listGatewayModels } from '@/features/ai-workbench/gateway'
import type { ApiKey } from '@/types'

import { streamChatCompletion } from './chatGateway'
import {
  loadChatSettings,
  loadConversations,
  saveChatSettings,
  saveConversations,
} from './storage'
import type {
  ChatAPIMessage,
  ChatAPITextContent,
  ChatConversation,
  ChatMessage,
  ChatSettings,
  ChatUsage,
} from './types'

const MAX_ATTACHMENTS = 5
const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024
const MAX_TEXT_ATTACHMENT_BYTES = 2 * 1024 * 1024
const STREAM_PERSIST_DELAY_MS = 300

function newId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message.trim() ? error.message : fallback
}

function newConversation(keyId = '', model = ''): ChatConversation {
  const now = Date.now()
  return {
    id: newId('conversation'),
    title: '新对话',
    keyId,
    model,
    messages: [],
    createdAt: now,
    updatedAt: now,
  }
}

function titleFromMessage(content: string): string {
  const normalized = content.replace(/\s+/g, ' ').trim()
  return normalized.length > 30 ? `${normalized.slice(0, 30)}...` : normalized || '新对话'
}

function downloadText(filename: string, content: string, type: string): void {
  const url = URL.createObjectURL(new Blob([content], { type }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function useKittyChat() {
  const stored = loadConversations()
  const conversations = shallowRef<ChatConversation[]>(stored.length ? stored : [newConversation()])
  const activeConversationId = shallowRef(conversations.value[0]?.id || '')
  const availableKeys = shallowRef<ApiKey[]>([])
  const models = shallowRef<string[]>([])
  const selectedKeyId = shallowRef(conversations.value[0]?.keyId || '')
  const selectedModel = shallowRef(conversations.value[0]?.model || '')
  const draft = shallowRef('')
  const attachments = shallowRef<File[]>([])
  const settings = shallowRef<ChatSettings>(loadChatSettings())
  const usage = shallowRef<ChatUsage | null>(null)
  const loadingKeys = shallowRef(false)
  const loadingModels = shallowRef(false)
  const streaming = shallowRef(false)
  const error = shallowRef('')
  let generationController: AbortController | null = null
  let modelsController: AbortController | null = null
  let persistenceTimer: ReturnType<typeof setTimeout> | null = null

  const activeConversation = computed(() => (
    conversations.value.find((conversation) => conversation.id === activeConversationId.value)
      || conversations.value[0]
      || null
  ))
  const selectedKey = computed(() => availableKeys.value.find((key) => String(key.id) === selectedKeyId.value) || null)
  const canSend = computed(() => !streaming.value
    && !!selectedKey.value
    && !!selectedModel.value
    && (!!draft.value.trim() || attachments.value.length > 0))
  const endpointHost = computed(() => typeof window === 'undefined' ? '' : window.location.host)

  function flushPersistence(): void {
    if (persistenceTimer !== null) {
      globalThis.clearTimeout(persistenceTimer)
      persistenceTimer = null
    }
    saveConversations(conversations.value)
  }

  function schedulePersistence(): void {
    if (persistenceTimer !== null) return
    persistenceTimer = globalThis.setTimeout(() => {
      persistenceTimer = null
      saveConversations(conversations.value)
    }, STREAM_PERSIST_DELAY_MS)
  }

  function persist(next: ChatConversation[], deferred = false): void {
    conversations.value = [...next].sort((a, b) => b.updatedAt - a.updatedAt)
    if (deferred) schedulePersistence()
    else flushPersistence()
  }

  function updateConversation(
    id: string,
    updater: (conversation: ChatConversation) => ChatConversation,
    deferred = false,
  ): void {
    persist(
      conversations.value.map((conversation) => conversation.id === id ? updater(conversation) : conversation),
      deferred,
    )
  }

  function updateActive(updater: (conversation: ChatConversation) => ChatConversation): void {
    const current = activeConversation.value
    if (current) updateConversation(current.id, updater)
  }

  function syncConversationSelection(): void {
    updateActive((conversation) => ({
      ...conversation,
      keyId: selectedKeyId.value,
      model: selectedModel.value,
      updatedAt: Date.now(),
    }))
  }

  async function loadModels(): Promise<void> {
    modelsController?.abort()
    const key = selectedKey.value
    if (!key) {
      models.value = []
      selectedModel.value = ''
      return
    }
    const controller = new AbortController()
    modelsController = controller
    loadingModels.value = true
    error.value = ''
    try {
      const response = await listGatewayModels(key.key, key.group?.platform || '', controller.signal)
      models.value = response.map((model) => model.id)
      if (!models.value.includes(selectedModel.value)) {
        selectedModel.value = models.value[0] || ''
      }
      syncConversationSelection()
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        models.value = []
        selectedModel.value = ''
        error.value = errorMessage(loadError, '模型列表加载失败。')
      }
    } finally {
      if (modelsController === controller) {
        modelsController = null
        loadingModels.value = false
      }
    }
  }

  async function setSelectedKey(value: string): Promise<void> {
    if (streaming.value || selectedKeyId.value === value) return
    selectedKeyId.value = value
    selectedModel.value = ''
    syncConversationSelection()
    await loadModels()
  }

  function setSelectedModel(value: string): void {
    if (streaming.value || !models.value.includes(value)) return
    selectedModel.value = value
    syncConversationSelection()
  }

  async function loadKeys(): Promise<void> {
    loadingKeys.value = true
    error.value = ''
    try {
      const response = await keysAPI.list(1, 100, { status: 'active' })
      availableKeys.value = response.items.filter((key) => key.status === 'active')
      if (!availableKeys.value.some((key) => String(key.id) === selectedKeyId.value)) {
        selectedKeyId.value = availableKeys.value[0] ? String(availableKeys.value[0].id) : ''
      }
      syncConversationSelection()
      await loadModels()
    } catch (loadError) {
      availableKeys.value = []
      models.value = []
      error.value = errorMessage(loadError, 'API Key 加载失败。')
    } finally {
      loadingKeys.value = false
    }
  }

  function createConversation(): void {
    if (streaming.value) return
    const conversation = newConversation(selectedKeyId.value, selectedModel.value)
    persist([conversation, ...conversations.value])
    activeConversationId.value = conversation.id
    draft.value = ''
    attachments.value = []
    usage.value = null
    error.value = ''
  }

  async function selectConversation(id: string): Promise<void> {
    if (streaming.value) return
    const conversation = conversations.value.find((item) => item.id === id)
    if (!conversation) return
    activeConversationId.value = id
    selectedKeyId.value = conversation.keyId
    selectedModel.value = conversation.model
    attachments.value = []
    usage.value = null
    error.value = ''
    if (!availableKeys.value.some((key) => String(key.id) === selectedKeyId.value)) {
      selectedKeyId.value = availableKeys.value[0] ? String(availableKeys.value[0].id) : ''
    }
    await loadModels()
  }

  function deleteConversation(id: string): void {
    if (streaming.value) return
    const remaining = conversations.value.filter((conversation) => conversation.id !== id)
    if (!remaining.length) remaining.push(newConversation(selectedKeyId.value, selectedModel.value))
    persist(remaining)
    if (activeConversationId.value === id) {
      activeConversationId.value = remaining[0].id
      void selectConversation(remaining[0].id)
    }
  }

  function clearConversations(): void {
    if (streaming.value) return
    const conversation = newConversation(selectedKeyId.value, selectedModel.value)
    persist([conversation])
    activeConversationId.value = conversation.id
    draft.value = ''
    attachments.value = []
  }

  function addAttachments(files: FileList | File[]): void {
    error.value = ''
    const incoming = Array.from(files)
    const accepted: File[] = []
    for (const file of incoming) {
      if (file.size > MAX_ATTACHMENT_BYTES) {
        error.value = `${file.name} 超过 10 MB，未添加。`
        continue
      }
      const isImage = file.type.startsWith('image/')
      const isText = file.type.startsWith('text/')
        || /\.(md|json|csv|ts|tsx|js|jsx|py|go|rs|java|html|css|xml|ya?ml|toml|ini|log)$/i.test(file.name)
      if (!isImage && !isText) {
        error.value = `${file.name} 暂不支持；可上传图片、代码或文本文件。`
        continue
      }
      accepted.push(file)
    }
    attachments.value = [...attachments.value, ...accepted].slice(0, MAX_ATTACHMENTS)
  }

  function removeAttachment(index: number): void {
    attachments.value = attachments.value.filter((_, itemIndex) => itemIndex !== index)
  }

  async function requestContent(text: string, files: File[]): Promise<ChatAPITextContent> {
    let expandedText = text.trim()
    const imageParts: Array<{ type: 'image_url'; image_url: { url: string } }> = []
    for (const file of files) {
      if (file.type.startsWith('image/')) {
        imageParts.push({ type: 'image_url', image_url: { url: await fileToDataURL(file) } })
      } else if (file.size <= MAX_TEXT_ATTACHMENT_BYTES) {
        const fileText = await fileToText(file)
        expandedText += `\n\n--- 文件：${file.name} ---\n${fileText}`
      } else {
        expandedText += `\n\n[文件 ${file.name} 超过 2 MB，未展开正文]`
      }
    }
    if (!imageParts.length) return expandedText
    return [
      { type: 'text', text: expandedText || '请分析上传的图片。' },
      ...imageParts,
    ]
  }

  function apiMessages(messages: ChatMessage[], latestUserContent?: ChatAPITextContent): ChatAPIMessage[] {
    const result: ChatAPIMessage[] = []
    if (settings.value.systemPrompt.trim()) {
      result.push({ role: 'system', content: settings.value.systemPrompt.trim() })
    }
    messages.forEach((message, index) => {
      if (message.error || (message.role === 'assistant' && !message.content)) return
      const isLatestUser = latestUserContent !== undefined
        && message.role === 'user'
        && index === messages.length - 1
      result.push({
        role: message.role,
        content: isLatestUser ? latestUserContent : message.content,
      })
    })
    return result
  }

  function appendToMessage(conversationId: string, messageId: string, field: 'content' | 'reasoning', chunk: string): void {
    updateConversation(conversationId, (conversation) => ({
      ...conversation,
      updatedAt: Date.now(),
      messages: conversation.messages.map((message) => message.id === messageId
        ? { ...message, [field]: `${message[field] || ''}${chunk}` }
        : message),
    }), true)
  }

  async function completeConversation(
    conversationId: string,
    assistantId: string,
    messages: ChatAPIMessage[],
  ): Promise<void> {
    const key = selectedKey.value
    if (!key || !selectedModel.value) return
    generationController?.abort()
    const controller = new AbortController()
    generationController = controller
    streaming.value = true
    usage.value = null
    error.value = ''
    try {
      await streamChatCompletion({
        apiKey: key.key,
        model: selectedModel.value,
        messages,
        settings: settings.value,
        signal: controller.signal,
      }, {
        onText: (chunk) => appendToMessage(conversationId, assistantId, 'content', chunk),
        onReasoning: (chunk) => appendToMessage(conversationId, assistantId, 'reasoning', chunk),
        onUsage: (value) => { usage.value = value },
      })
    } catch (completionError) {
      if (!isAbortError(completionError)) {
        const message = errorMessage(completionError, '消息发送失败。')
        error.value = message
        updateConversation(conversationId, (conversation) => ({
          ...conversation,
          messages: conversation.messages.map((item) => item.id === assistantId ? { ...item, error: message } : item),
          updatedAt: Date.now(),
        }))
      }
    } finally {
      flushPersistence()
      if (generationController === controller) generationController = null
      streaming.value = false
    }
  }

  async function sendMessage(): Promise<void> {
    if (!canSend.value) return
    const conversation = activeConversation.value
    if (!conversation) return
    const text = draft.value.trim()
    const files = [...attachments.value]
    draft.value = ''
    attachments.value = []

    let content: ChatAPITextContent
    try {
      content = await requestContent(text, files)
    } catch (attachmentError) {
      error.value = errorMessage(attachmentError, '附件读取失败。')
      draft.value = text
      attachments.value = files
      return
    }

    const now = Date.now()
    const userMessage: ChatMessage = {
      id: newId('message'),
      role: 'user',
      content: text || '请分析上传的内容。',
      attachments: files.map((file) => ({ name: file.name, type: file.type, size: file.size })),
      createdAt: now,
    }
    const assistantMessage: ChatMessage = {
      id: newId('message'),
      role: 'assistant',
      content: '',
      createdAt: now + 1,
    }
    const nextMessages = [...conversation.messages, userMessage, assistantMessage]
    updateConversation(conversation.id, (current) => ({
      ...current,
      title: current.messages.length ? current.title : titleFromMessage(userMessage.content),
      keyId: selectedKeyId.value,
      model: selectedModel.value,
      messages: nextMessages,
      updatedAt: now,
    }))
    await completeConversation(
      conversation.id,
      assistantMessage.id,
      apiMessages(nextMessages.slice(0, -1), content),
    )
  }

  async function regenerateLast(): Promise<void> {
    if (streaming.value) return
    const conversation = activeConversation.value
    if (!conversation) return
    let lastAssistantIndex = -1
    for (let index = conversation.messages.length - 1; index >= 0; index -= 1) {
      if (conversation.messages[index]?.role === 'assistant') {
        lastAssistantIndex = index
        break
      }
    }
    if (lastAssistantIndex < 0) return
    const sourceMessages = conversation.messages.slice(0, lastAssistantIndex)
    const assistant: ChatMessage = {
      id: newId('message'),
      role: 'assistant',
      content: '',
      createdAt: Date.now(),
    }
    updateConversation(conversation.id, (current) => ({
      ...current,
      messages: [...sourceMessages, assistant],
      updatedAt: Date.now(),
    }))
    await completeConversation(conversation.id, assistant.id, apiMessages(sourceMessages))
  }

  function stopGeneration(): void {
    generationController?.abort()
    generationController = null
    streaming.value = false
    flushPersistence()
  }

  function updateSettings(value: ChatSettings): void {
    settings.value = { ...value }
    saveChatSettings(settings.value)
  }

  function exportConversation(format: 'markdown' | 'json'): void {
    const conversation = activeConversation.value
    if (!conversation) return
    const safeTitle = conversation.title.replace(/[\\/:*?"<>|]/g, '-').slice(0, 50) || 'kitty-chat'
    if (format === 'json') {
      downloadText(`${safeTitle}.json`, JSON.stringify(conversation, null, 2), 'application/json;charset=utf-8')
      return
    }
    const markdown = conversation.messages.map((message) => {
      const heading = message.role === 'user' ? '用户' : message.role === 'assistant' ? '助手' : '系统'
      const reasoning = message.reasoning ? `\n\n<details><summary>思考过程</summary>\n\n${message.reasoning}\n\n</details>` : ''
      return `## ${heading}\n\n${message.content}${reasoning}`
    }).join('\n\n---\n\n')
    downloadText(`${safeTitle}.md`, `# ${conversation.title}\n\n${markdown}\n`, 'text/markdown;charset=utf-8')
  }

  onBeforeUnmount(() => {
    generationController?.abort()
    modelsController?.abort()
    flushPersistence()
  })

  return {
    conversations,
    activeConversation,
    activeConversationId,
    availableKeys,
    models,
    selectedKeyId,
    selectedModel,
    draft,
    attachments,
    settings,
    usage,
    loadingKeys,
    loadingModels,
    streaming,
    error,
    canSend,
    endpointHost,
    loadKeys,
    setSelectedKey,
    setSelectedModel,
    createConversation,
    selectConversation,
    deleteConversation,
    clearConversations,
    addAttachments,
    removeAttachment,
    sendMessage,
    regenerateLast,
    stopGeneration,
    updateSettings,
    exportConversation,
  }
}
