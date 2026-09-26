import { describe, expect, it } from 'vitest'

import { consumeSSE, parseSSEBlock } from '../sse'

describe('SSE parser', () => {
  it('parses event, id and multiline data fields', () => {
    expect(parseSSEBlock('event: chunk\nid: 7\ndata: first\ndata: second')).toEqual({
      event: 'chunk',
      id: '7',
      data: 'first\nsecond',
    })
  })

  it('handles messages split across response chunks', async () => {
    const encoder = new TextEncoder()
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('data: {"a":'))
        controller.enqueue(encoder.encode('1}\r\n\r\ndata: [DONE]\n\n'))
        controller.close()
      },
    })
    const messages: string[] = []
    await consumeSSE(new Response(stream), (message) => messages.push(message.data))
    expect(messages).toEqual(['{"a":1}', '[DONE]'])
  })
})
