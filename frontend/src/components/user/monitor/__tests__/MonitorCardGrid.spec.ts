import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { UserMonitorView } from '@/api/channelMonitor'
import MonitorCardGrid from '../MonitorCardGrid.vue'
import MonitorTimeline from '../MonitorTimeline.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, number>) => {
        if (key === 'monitorCommon.providers.openai') return 'OpenAI'
        if (key === 'monitorCommon.providers.anthropic') return 'Anthropic'
        if (key === 'monitorCommon.providers.grok') return 'Grok'
        if (key === 'channelStatus.providerSections.other') return 'Other'
        if (key === 'monitorCommon.monitorCount') return `${params?.n ?? 0} items`
        return key
      },
    }),
  }
})

const MonitorCardStub = defineComponent({
  name: 'MonitorCard',
  props: {
    item: { type: Object, required: true },
    availabilityValue: { type: Number, default: null },
    rateMultiplier: { type: Number, default: null },
  },
  emits: ['click'],
  template: '<button data-test="monitor-card" @click="$emit(\'click\')" />',
})

function monitor(overrides: Partial<UserMonitorView>): UserMonitorView {
  return {
    id: 1,
    name: 'Stable channel',
    provider: 'openai',
    group_name: 'Codex Stable',
    primary_model: 'gpt-5.6-sol',
    primary_status: 'operational',
    primary_latency_ms: 1800,
    primary_ping_latency_ms: 150,
    availability_7d: 99,
    extra_models: [],
    timeline: [],
    ...overrides,
  }
}

describe('MonitorCardGrid', () => {
  it('groups providers and resolves the effective rate by normalized group name', () => {
    const wrapper = mount(MonitorCardGrid, {
      props: {
        items: [
          monitor({ id: 1, provider: 'openai', group_name: 'Codex Stable' }),
          monitor({ id: 2, provider: 'anthropic', group_name: 'Claude Max' }),
          monitor({ id: 3, provider: 'grok', group_name: 'Grok Public' }),
          monitor({ id: 4, provider: 'gemini', group_name: 'Gemini Public' }),
        ],
        window: '7d',
        loading: false,
        detailCache: {},
        rateByGroupName: {
          'codex stable': 0.14,
          'claude max': 1.85,
          'grok public': 0.7,
          'gemini public': 0.5,
        },
      },
      global: {
        stubs: {
          MonitorCard: MonitorCardStub,
          ProviderIcon: true,
        },
      },
    })

    expect(
      wrapper.findAll('[data-monitor-provider-section]').map(section => section.attributes('data-provider')),
    ).toEqual(['openai', 'anthropic', 'grok', 'other'])

    const cards = wrapper.findAllComponents(MonitorCardStub)
    expect(cards.map(card => card.props('rateMultiplier'))).toEqual([0.14, 1.85, 0.7, 0.5])
  })
})

describe('MonitorTimeline', () => {
  it('always renders a stable 60-column timeline', () => {
    const wrapper = mount(MonitorTimeline, {
      props: {
        buckets: [
          {
            status: 'operational',
            latency_ms: 1200,
            ping_latency_ms: 150,
            checked_at: new Date().toISOString(),
          },
        ],
      },
    })

    const grid = wrapper.get('[data-monitor-timeline] > .grid')
    expect(grid.element.children).toHaveLength(60)
    expect(grid.attributes('style')).toContain('repeat(60, minmax(0, 1fr))')
  })
})
