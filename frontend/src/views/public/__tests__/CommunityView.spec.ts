import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CommunityView from '../CommunityView.vue'

const api = vi.hoisted(() => ({ guide: vi.fn() }))
const auth = vi.hoisted(() => ({ isAuthenticated: false }))
vi.mock('@/api/community', () => ({ getCommunityGuide: api.guide }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ brandLogoUrl: '/logo.svg' }) }))
vi.mock('@/composables/useTheme', () => ({ useTheme: () => ({ isDark: false, toggleTheme: vi.fn() }) }))
vi.mock('@/config/customerContact', () => ({ communityGroupContact: { href: 'https://qm.qq.com/q/aP8eapBPpu', qrSrc: '/sites/kittyapi/images/heyroute/community/qq-qr.png', valueKind: 'number', value: '494038' } }))
const options = { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } }
const active = () => ({ enabled: true, chat_enabled: true, auto_email_binding: true, join_review: { min_spent: 10, question: '已消费10元用户邮箱？' }, benefits: [{ id: 'checkin', title: '每日签到', highlight: '0.1–1 站内余额', lines: ['每天每账号一次'], command: '签到' }], commands: [{ command: '签到', description: '领取今日签到奖励' }] })

describe('Community page', () => {
  beforeEach(() => { vi.resetAllMocks(); auth.isAuthenticated = false; api.guide.mockResolvedValue(active()) })
  it('uses the selected public community group and current rules', async () => {
    const w = mount(CommunityView, options)
    await flushPromises()
    expect(w.text()).toContain('494038')
    expect(w.get('a[target="_blank"]').attributes('href')).toBe('https://qm.qq.com/q/aP8eapBPpu')
    expect(w.get('[data-benefit="checkin"]').text()).toContain('0.1–1 站内余额')
    expect(w.text()).toContain('核对 QQ 身份，确认绑定')
    expect(api.guide).toHaveBeenCalledTimes(1)
  })
  it('requires separate website confirmation for the binding-code method', async () => {
    const w = mount(CommunityView, options)
    await flushPromises()
    await w.get('#code-tab').trigger('click')
    expect(w.get('[role="tabpanel"]').text()).toContain('点击「确认绑定此 QQ 身份」')
    expect(w.get('[role="tabpanel"]').text()).toContain('10 分钟有效')
    expect(api.guide).toHaveBeenCalledTimes(1)
  })
  it('hides stale benefits on error and refreshes from new server rules', async () => {
    const w = mount(CommunityView, options)
    await flushPromises()
    api.guide.mockRejectedValueOnce(new Error('unavailable'))
    await w.findAll('button').find(b => b.text() === '刷新活动')!.trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('暂时无法读取')
    expect(w.find('[data-benefit="checkin"]').exists()).toBe(false)
    const next = active(); next.benefits[0].highlight = '0.2–0.8 站内余额'
    api.guide.mockResolvedValueOnce(next)
    await w.findAll('button').find(b => b.text() === '刷新活动')!.trigger('click')
    await flushPromises()
    expect(w.get('[data-benefit="checkin"]').text()).toContain('0.2–0.8')
  })
  it('does not advertise disabled rewards or promise automatic email', async () => {
    api.guide.mockResolvedValue({ enabled: false, auto_email_binding: false, benefits: [], commands: [] })
    const w = mount(CommunityView, options)
    await flushPromises()
    expect(w.text()).toContain('机器人目前未开放服务')
    expect(w.find('[data-benefit]').exists()).toBe(false)
    expect(w.get('#email-tab').text()).toBe('群内邮箱确认')
  })
  it('copies commands without sending messages or taking part in activities', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const w = mount(CommunityView, options)
    await flushPromises()
    await w.findAll('[aria-label="复制指令：签到"]')[0].trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('签到')
    expect(w.text()).toContain('已复制「签到」')
    expect(api.guide).toHaveBeenCalledTimes(1)
  })
})
