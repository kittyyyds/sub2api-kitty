import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import DocumentationPanel from '../DocumentationPanel.vue'
import { createTestI18n } from '@/test/createTestI18n'

describe('DocumentationPanel', () => {
  it('switches Claude Code and Codex commands with the selected operating system', async () => {
    const wrapper = mount(DocumentationPanel, { global: { plugins: [createTestI18n()] } })

    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      '$env:ANTHROPIC_AUTH_TOKEN="sk-your-key"'
    )
    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      'ANTHROPIC_BASE_URL="https://kittyapi.xyz"'
    )

    await wrapper.get('[data-testid="tool-codex"]').trigger('click')
    await wrapper.get('[data-testid="system-linux"]').trigger('click')

    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      'export OPENAI_API_KEY="sk-your-key"'
    )
    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain(
      'OPENAI_BASE_URL="https://kittyapi.xyz/v1"'
    )
  })

  it('provides OpenAI examples and actionable troubleshooting content', async () => {
    const wrapper = mount(DocumentationPanel, { global: { plugins: [createTestI18n()] } })

    await wrapper.get('[data-testid="tool-openai"]').trigger('click')
    await wrapper.get('[data-testid="sdk-python"]').trigger('click')

    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain('from openai import OpenAI')
    expect(wrapper.get('.guide-workbench__panel pre').text()).toContain('api_key="sk-your-key"')
    expect(wrapper.findAll('.troubleshooting-grid details')).toHaveLength(6)
    expect(wrapper.text()).toContain('401')
    expect(wrapper.text()).toContain('环境变量未生效')
  })
})
