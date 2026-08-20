import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import CustomerCodeBlock from '../CustomerCodeBlock.vue'
import { createTestI18n } from '@/test/createTestI18n'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('CustomerCodeBlock', () => {
  it('copies exactly the visible example and reports success', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText }
    })

    const wrapper = mount(CustomerCodeBlock, {
      props: {
        code: 'export OPENAI_API_KEY="sk-your-key"',
        label: 'Codex / Linux',
        language: 'shell'
      },
      global: { plugins: [createTestI18n()] }
    })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('export OPENAI_API_KEY="sk-your-key"')
    expect(wrapper.get('button').text()).toContain('已复制')

    wrapper.unmount()
  })
})
