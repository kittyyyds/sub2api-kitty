import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  listKeys: vi.fn(),
  listModels: vi.fn(),
  saveConversations: vi.fn(),
  streamChatCompletion: vi.fn(),
}))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: mocks.listKeys },
}))

vi.mock('@/features/ai-workbench/gateway', () => ({
  fileToDataURL: vi.fn(),
  fileToText: vi.fn(),
  isAbortError: (error: unknown) => error instanceof DOMException && error.name === 'AbortError',
  listGatewayModels: mocks.listModels,
}))

vi.mock('../chatGateway', () => ({
  streamChatCompletion: mocks.streamChatCompletion,
}))

vi.mock('../storage', () => ({
  loadChatSettings: () => ({
    systemPrompt: '',
    temperature: 0.7,
    topP: 1,
    maxTokens: 8192,
    reasoningEffort: 'auto',
  }),
  loadConversations: () => [],
  saveChatSettings: vi.fn(),
  saveConversations: mocks.saveConversations,
}))

import { useKittyChat } from '../useKittyChat'

type KittyChat = ReturnType<typeof useKittyChat>

function mountChat(): { chat: KittyChat; wrapper: VueWrapper } {
  let chat: KittyChat | undefined
  const Harness = defineComponent({
    setup() {
      chat = useKittyChat()
      return () => h('div')
    },
  })
  const wrapper = mount(Harness)
  return { chat: chat!, wrapper }
}

describe('useKittyChat streaming persistence', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listKeys.mockResolvedValue({
      items: [{
        id: 1,
        key: 'sk-local-test',
        name: 'Test Key',
        status: 'active',
        group: { name: 'OpenAI', platform: 'openai' },
      }],
    })
    mocks.listModels.mockResolvedValue([{ id: 'gpt-test', ownedBy: 'openai' }])
    mocks.streamChatCompletion.mockImplementation(async (_options, callbacks) => {
      for (let index = 0; index < 120; index += 1) callbacks.onText('x')
    })
  })

  it('does not synchronously write the full conversation for every stream chunk', async () => {
    const { chat, wrapper } = mountChat()
    await chat.loadKeys()
    chat.draft.value = 'hello'

    await chat.sendMessage()

    const assistant = chat.activeConversation.value?.messages.at(-1)
    expect(assistant?.content).toBe('x'.repeat(120))
    expect(mocks.saveConversations.mock.calls.length).toBeLessThan(10)
    expect(mocks.saveConversations).toHaveBeenLastCalledWith(chat.conversations.value)

    wrapper.unmount()
  })
})
