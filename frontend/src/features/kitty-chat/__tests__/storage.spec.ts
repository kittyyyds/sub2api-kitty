import { describe, expect, it } from 'vitest'

import { loadChatSettings, loadConversations, saveConversations } from '../storage'

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => { values.delete(key) },
    setItem: (key, value) => { values.set(key, value) },
  }
}

describe('Kitty chat local storage', () => {
  it('round trips conversations and drops malformed records', () => {
    const storage = memoryStorage()
    saveConversations([{
      id: 'one',
      title: 'Test',
      keyId: '7',
      model: 'gpt-5',
      createdAt: 1,
      updatedAt: 2,
      messages: [{ id: 'm1', role: 'user', content: 'hello', createdAt: 1 }],
    }], storage)
    expect(loadConversations(storage)).toMatchObject([{
      id: 'one',
      messages: [{ role: 'user', content: 'hello' }],
    }])
  })

  it('clamps settings loaded from storage', () => {
    const storage = memoryStorage()
    storage.setItem('kitty-chat:settings:v1', JSON.stringify({ temperature: 9, topP: -1, maxTokens: 0 }))
    expect(loadChatSettings(storage)).toMatchObject({ temperature: 2, topP: 0, maxTokens: 1 })
  })

  it('keeps the chat usable when browser storage quota is exhausted', () => {
    const storage = memoryStorage()
    storage.setItem = () => { throw new DOMException('Quota exceeded', 'QuotaExceededError') }

    expect(() => saveConversations([{
      id: 'quota-test',
      title: 'Quota test',
      keyId: '7',
      model: 'gpt-5',
      createdAt: 1,
      updatedAt: 2,
      messages: [{ id: 'm1', role: 'assistant', content: 'x'.repeat(20_000), createdAt: 1 }],
    }], storage)).not.toThrow()
  })
})
