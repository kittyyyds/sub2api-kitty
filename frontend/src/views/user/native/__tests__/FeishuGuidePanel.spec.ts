import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import FeishuGuidePanel from '../FeishuGuidePanel.vue'

const guideUrl = 'https://tb.kittyyyds.xyz/docs/register/'

describe('FeishuGuidePanel', () => {
  it('embeds the configured guide and exposes a safe new-window link', () => {
    const wrapper = mount(FeishuGuidePanel)
    const frame = wrapper.get('[data-testid="feishu-guide-frame"]')
    const openLink = wrapper.get('[data-testid="feishu-guide-open"]')

    expect(frame.attributes('src')).toBe(guideUrl)
    expect(frame.attributes('title')).toBe('Kitty API 新手必看文档')
    expect(openLink.attributes('href')).toBe(guideUrl)
    expect(openLink.attributes('target')).toBe('_blank')
    expect(openLink.attributes('rel')).toContain('noopener')
  })

  it('removes the loading state after the iframe loads', async () => {
    const wrapper = mount(FeishuGuidePanel)

    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    await wrapper.get('[data-testid="feishu-guide-frame"]').trigger('load')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })
})
