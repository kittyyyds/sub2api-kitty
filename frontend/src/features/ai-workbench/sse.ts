export interface SSEMessage {
  event: string
  data: string
  id: string
}

export function parseSSEBlock(block: string): SSEMessage | null {
  const normalized = block.replace(/\r\n/g, '\n').trimEnd()
  if (!normalized.trim()) return null

  let event = ''
  let id = ''
  const data: string[] = []
  for (const line of normalized.split('\n')) {
    if (!line || line.startsWith(':')) continue
    const separator = line.indexOf(':')
    const field = separator === -1 ? line : line.slice(0, separator)
    const rawValue = separator === -1 ? '' : line.slice(separator + 1)
    const value = rawValue.startsWith(' ') ? rawValue.slice(1) : rawValue
    if (field === 'event') event = value
    else if (field === 'id') id = value
    else if (field === 'data') data.push(value)
  }
  if (!data.length && !event && !id) return null
  return { event, id, data: data.join('\n') }
}

export async function consumeSSE(
  response: Response,
  onMessage: (message: SSEMessage) => void | Promise<void>,
): Promise<void> {
  if (!response.body) throw new Error('流式响应没有可读取的内容。')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, '\n')

      let boundary = buffer.indexOf('\n\n')
      while (boundary !== -1) {
        const block = buffer.slice(0, boundary)
        buffer = buffer.slice(boundary + 2)
        const message = parseSSEBlock(block)
        if (message) await onMessage(message)
        boundary = buffer.indexOf('\n\n')
      }
      if (done) break
    }

    const tail = parseSSEBlock(buffer)
    if (tail) await onMessage(tail)
  } finally {
    reader.releaseLock()
  }
}
