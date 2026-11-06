import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MonitorStatusBoard from '../MonitorStatusBoard.vue'
import MonitorStatusRow from '../MonitorStatusRow.vue'
import type { UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'

vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorQuotaVisible: () => true }))

const item = (id: number, provider: UserMonitorView['provider']): UserMonitorView => ({
  id, provider, name: `Channel ${id}`, group_name: ' GPT ', primary_model: 'gpt-6.1-sol',
  primary_status: 'operational', primary_latency_ms: 65000, primary_ping_latency_ms: 90,
  availability_7d: 99, extra_models: [], timeline: []
})
const props = { items: [item(1, 'openai'), item(2, 'deepseek')], window: '7d' as const, loading: false, detailCache: {}, rateByGroupName: { gpt: .15 } }
const global = { stubs: { Icon: true, ProviderIcon: true, MonitorTimeline: true, MonitorQuotaView: true } }

describe('MonitorStatusBoard', () => {
  it('includes every provider, filters by model/name and preserves detail selection', async () => {
    const wrapper = mount(MonitorStatusBoard, { props, global })
    expect(wrapper.findAll('[data-monitor-provider-section]')).toHaveLength(2)
    expect(wrapper.get('[data-provider="deepseek"]').exists()).toBe(true)
    expect(wrapper.findComponent(MonitorStatusRow).props('rateMultiplier')).toBe(.15)
    await wrapper.get('input').setValue('Channel 2')
    expect(wrapper.findAllComponents(MonitorStatusRow)).toHaveLength(1)
    await wrapper.get('.status-row-head').trigger('click')
    expect(wrapper.emitted('cardClick')?.[0]).toEqual([props.items[1]])
    wrapper.unmount()
  })

  it('uses the selected history window and shows unknown until its detail arrives', async () => {
    const wrapper = mount(MonitorStatusBoard, { props, global })
    await wrapper.setProps({ window: '30d' })
    expect(wrapper.findComponent(MonitorStatusRow).props('availabilityValue')).toBe(null)
    await wrapper.setProps({ detailCache: { 1: { models: [{ model: 'gpt-6.1-sol', availability_15d: 97, availability_30d: 95 }] } as UserMonitorDetail } })
    expect(wrapper.findComponent(MonitorStatusRow).props('availabilityValue')).toBe(95)
    await wrapper.setProps({ window: '15d' })
    expect(wrapper.findComponent(MonitorStatusRow).props('availabilityValue')).toBe(97)
    wrapper.unmount()
  })

  it('keeps upstream status semantics even with slow latency and renders quota', () => {
    const wrapper = mount(MonitorStatusRow, { props: { item: { ...item(1, 'openai'), latest_quota: {} as UserMonitorView['latest_quota'] }, availabilityValue: 99 }, global })
    expect(wrapper.get('.status-row-chip').text()).toBe('monitorCommon.cardStatus.operational')
    expect(wrapper.find('monitor-quota-view-stub').exists()).toBe(true)
    wrapper.unmount()
  })
})
