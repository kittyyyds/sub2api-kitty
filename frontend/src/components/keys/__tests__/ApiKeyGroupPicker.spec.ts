import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Group } from '@/types'
import ApiKeyGroupPicker from '../ApiKeyGroupPicker.vue'

const messages: Record<string, string> = {
  'keys.groupLabel': 'Group',
  'keys.groupPicker.claude': 'Claude',
  'keys.groupPicker.empty': 'No groups',
  'keys.groupPicker.gemini': 'Gemini',
  'keys.groupPicker.grok': 'Grok',
  'keys.groupPicker.openai': 'OpenAI',
  'keys.groupPicker.other': 'Other',
  'keys.groupPicker.platforms': 'Platforms',
  'keys.noGroupFound': 'No matching groups',
  'keys.searchGroup': 'Search groups...',
  'keys.selectGroup': 'Select a group'
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

const makeGroup = (
  id: number,
  name: string,
  platform: Group['platform'],
  description: string
): Group => ({
  id,
  name,
  platform,
  description,
  rate_multiplier: 0.18,
  subscription_type: 'standard',
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1
} as Group)

const groups = [
  makeGroup(1, 'Codex Pro', 'openai', 'Stable coding group'),
  makeGroup(2, 'Claude Max', 'anthropic', 'Long context group'),
  makeGroup(3, 'Gemini Pro', 'gemini', 'Gemini CLI group'),
  makeGroup(4, 'Grok Fast', 'grok', 'Fast Grok group'),
  makeGroup(5, 'Antigravity', 'antigravity', 'Mixed provider group')
]

const GroupBadgeStub = {
  name: 'GroupBadge',
  props: ['name'],
  template: '<span data-test="selected-group">{{ name }}</span>'
}

const GroupOptionItemStub = {
  name: 'GroupOptionItem',
  props: ['name', 'platform', 'description'],
  template: '<span data-test="group-option">{{ name }}|{{ platform }}|{{ description }}</span>'
}

const PlatformIconStub = {
  name: 'PlatformIcon',
  props: ['platform'],
  template: '<span data-test="platform-icon">{{ platform }}</span>'
}

const IconStub = {
  name: 'Icon',
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>'
}

const mountPicker = (modelValue: number | null = null, showPlatformTabs?: boolean) => mount(ApiKeyGroupPicker, {
  props: {
    modelValue,
    ...(showPlatformTabs === undefined ? {} : { showPlatformTabs }),
    groups,
    userRates: { 1: 0.16 }
  },
  global: {
    stubs: {
      GroupBadge: GroupBadgeStub,
      GroupOptionItem: GroupOptionItemStub,
      PlatformIcon: PlatformIconStub,
      Icon: IconStub,
      Teleport: true
    }
  }
})

describe('ApiKeyGroupPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('closes only the picker on Escape without bubbling to its containing dialog', async () => {
    const wrapper = mountPicker()
    const dialogKeydown = vi.fn()
    wrapper.element.addEventListener('keydown', dialogKeydown)
    await wrapper.get('button[aria-haspopup="listbox"]').trigger('click')
    await wrapper.get('input[type="search"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(dialogKeydown).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('groups choices by their real platform and renders platform icons', async () => {
    const wrapper = mountPicker()

    await wrapper.get('button[aria-haspopup="listbox"]').trigger('click')

    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text())).toEqual([
      'openaiOpenAI',
      'anthropicClaude',
      'geminiGemini',
      'grokGrok',
      'deepseekkeys.providers.domestic',
      'videokeys.providers.video',
      'gridOther'
    ])
    expect(wrapper.text()).toContain('Codex Pro|openai|Stable coding group')
    expect(wrapper.text()).not.toContain('Claude Max|anthropic')

    await wrapper.get('[data-category="anthropic"]').trigger('click')

    expect(wrapper.text()).toContain('Claude Max|anthropic|Long context group')
    expect(wrapper.text()).not.toContain('Codex Pro|openai')
  })

  it('puts antigravity groups in Other and filters the active category by search', async () => {
    const wrapper = mountPicker()

    await wrapper.get('button[aria-haspopup="listbox"]').trigger('click')
    await wrapper.get('[data-category="other"]').trigger('click')

    expect(wrapper.text()).toContain('Antigravity|antigravity|Mixed provider group')

    await wrapper.get('input[type="search"]').setValue('missing')
    expect(wrapper.text()).toContain('No matching groups')
  })

  it('opens on the selected group category and emits a single group id', async () => {
    const wrapper = mountPicker(2)

    expect(wrapper.get('[data-test="selected-group"]').text()).toBe('Claude Max')

    await wrapper.get('button[aria-haspopup="listbox"]').trigger('click')
    expect(wrapper.get('[data-category="anthropic"]').attributes('aria-selected')).toBe('true')

    await wrapper.get('[data-group-id="2"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[2]])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
  })

  it('keeps independent video groups in their own edit category', async () => {
    const wrapper = mountPicker()
    await wrapper.setProps({ groups: [...groups, makeGroup(6, 'Video test', 'video', 'Video provider')], modelValue: 6 })
    await wrapper.get('button[aria-haspopup="listbox"]').trigger('click')
    expect(wrapper.get('[data-category="video"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.findAll('[data-group-id]').map(item => item.attributes('data-group-id'))).toEqual(['6'])
    wrapper.unmount()
  })

  it('shows every supplied platform without tabs and searches and selects across them', async () => {
    const wrapper = mountPicker(null, false)
    const otherGroups = groups.filter((group) => [3, 4, 5].includes(group.id))
    await wrapper.setProps({ groups: otherGroups })
    await wrapper.get('button[aria-haspopup="listbox"]').trigger('click')

    expect(wrapper.find('[role="tablist"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-group-id]').map((item) => item.attributes('data-group-id')))
      .toEqual(['3', '4', '5'])

    await wrapper.get('input[type="search"]').setValue(' FAST ')
    expect(wrapper.findAll('[data-group-id]').map((item) => item.attributes('data-group-id')))
      .toEqual(['4'])
    await wrapper.get('input[type="search"]').setValue('mixed provider')
    expect(wrapper.findAll('[data-group-id]').map((item) => item.attributes('data-group-id')))
      .toEqual(['5'])
    await wrapper.get('[data-group-id="5"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[5]])
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
