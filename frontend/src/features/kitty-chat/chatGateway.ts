import { buildGatewayUrl } from '@/api/url'
import { consumeSSE } from '@/features/ai-workbench/sse'
import { gatewayAuthorization, readGatewayError } from '@/features/ai-workbench/gateway'

import type {
  ChatAPIMessage,
  ChatSettings,
  ChatStreamCallbacks,
  ChatUsage,
} from './types'

interface StreamChatOptions {
  apiKey: string
  model: string
  messages: ChatAPIMessage[]
  settings: ChatSettings
  signal?: AbortSignal
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function textFromDelta(value: unknown): string {
  if (typeof value === 'string') return value
  if (!Array.isArray(value)) return ''
  return value.map((part) => {
    if (!isRecord(part)) return ''
    return typeof part.text === 'string' ? part.text : typeof part.content === 'string' ? part.content : ''
  }).join('')
}

function normalizeUsage(value: unknown): ChatUsage | null {
  if (!isRecord(value)) return null
  const promptTokens = Number(value.prompt_tokens ?? value.input_tokens) || 0
  const completionTokens = Number(value.completion_tokens ?? value.output_tokens) || 0
  const totalTokens = Number(value.total_tokens) || promptTokens + completionTokens
  return { promptTokens, completionTokens, totalTokens }
}

function bodyForRequest(options: StreamChatOptions): Record<string, unknown> {
  const body: Record<string, unknown> = {
    model: options.model,
    messages: options.messages,
    stream: true,
    stream_options: { include_usage: true },
    temperature: options.settings.temperature,
    top_p: options.settings.topP,
    max_tokens: options.settings.maxTokens,
  }
  if (options.settings.reasoningEffort !== 'auto' && options.settings.reasoningEffort !== 'none') {
    body.reasoning_effort = options.settings.reasoningEffort
  }
  return body
}

function unsupportedParameter(error: Error, body: Record<string, unknown>): string {
  const message = error.message
  const quoted = message.match(/(?:parameter|param)[^\w]+[`'"]?([a-z][a-z0-9_]*)/i)?.[1]
  if (quoted && quoted in body) return quoted
  return ['reasoning_effort', 'temperature', 'top_p', 'max_tokens', 'stream_options']
    .find((key) => key in body && new RegExp(key, 'i').test(message)) || ''
}

function canRetryUnsupported(response: Response, error: Error): boolean {
  return response.status === 400 || response.status === 422
    ? /unsupported|unknown|not supported|invalid.*parameter|不支持/i.test(error.message)
    : false
}

export async function streamChatCompletion(
  options: StreamChatOptions,
  callbacks: ChatStreamCallbacks,
): Promise<void> {
  const body = bodyForRequest(options)
  const removed = new Set<string>()

  while (true) {
    const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
      method: 'POST',
      headers: {
        ...gatewayAuthorization(options.apiKey),
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(body),
      signal: options.signal,
    })

    if (!response.ok) {
      const error = await readGatewayError(response, '聊天请求失败')
      const parameter = unsupportedParameter(error, body)
      if (parameter && !removed.has(parameter) && canRetryUnsupported(response, error)) {
        removed.add(parameter)
        delete body[parameter]
        continue
      }
      throw error
    }

    await consumeSSE(response, ({ data }) => {
      if (!data || data === '[DONE]') return
      let payload: unknown
      try {
        payload = JSON.parse(data)
      } catch {
        return
      }
      if (!isRecord(payload)) return

      const usage = normalizeUsage(payload.usage)
      if (usage) callbacks.onUsage?.(usage)

      const choices = Array.isArray(payload.choices) ? payload.choices : []
      for (const choice of choices) {
        if (!isRecord(choice) || !isRecord(choice.delta)) continue
        const text = textFromDelta(choice.delta.content)
        if (text) callbacks.onText(text)
        const reasoning = textFromDelta(choice.delta.reasoning_content ?? choice.delta.reasoning)
        if (reasoning) callbacks.onReasoning?.(reasoning)
      }
    })
    return
  }
}
