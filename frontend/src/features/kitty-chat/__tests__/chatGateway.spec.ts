import { afterEach, describe, expect, it, vi } from 'vitest'

import { streamChatCompletion } from '../chatGateway'
import { DEFAULT_CHAT_SETTINGS } from '../storage'

afterEach(() => vi.unstubAllGlobals())

function streamResponse(chunks: string[]): Response {
  const encoder = new TextEncoder()
  return new Response(new ReadableStream<Uint8Array>({
    start(controller) {
      chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)))
      controller.close()
    },
  }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

describe('chat gateway', () => {
  it('streams content and usage from OpenAI-compatible SSE', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamResponse([
      'data: {"choices":[{"delta":{"content":"你"}}]}\n\n',
      'data: {"choices":[{"delta":{"content":"好"}}],"usage":{"total_tokens":3}}\n\n',
      'data: [DONE]\n\n',
    ])))
    let text = ''
    let total = 0
    await streamChatCompletion({
      apiKey: 'sk-test',
      model: 'gpt-test',
      messages: [{ role: 'user', content: 'hi' }],
      settings: DEFAULT_CHAT_SETTINGS,
    }, {
      onText: (chunk) => { text += chunk },
      onUsage: (usage) => { total = usage.totalTokens },
    })
    expect(text).toBe('你好')
    expect(total).toBe(3)
  })

  it('retries after removing an unsupported optional parameter', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { message: 'Unsupported parameter: temperature' } }), { status: 400 }))
      .mockResolvedValueOnce(streamResponse(['data: [DONE]\n\n']))
    vi.stubGlobal('fetch', fetchMock)
    await streamChatCompletion({
      apiKey: 'sk-test',
      model: 'strict-model',
      messages: [{ role: 'user', content: 'hi' }],
      settings: DEFAULT_CHAT_SETTINGS,
    }, { onText: () => undefined })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    const retriedBody = JSON.parse(fetchMock.mock.calls[1][1].body as string)
    expect(retriedBody).not.toHaveProperty('temperature')
  })
})
