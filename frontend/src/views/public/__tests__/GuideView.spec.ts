import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GuideView from '../GuideView.vue'
const { getPublicGuide } = vi.hoisted(() => ({ getPublicGuide: vi.fn() }))
vi.mock('@/api/guide', () => ({ getPublicGuide }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'KittyAPI' }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false }) }))
vi.mock('@/composables/useTheme', () => ({ useTheme: () => ({ isDark: false, toggleTheme: vi.fn() }) }))
const options = { global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } }
describe('GuideView', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders published article anchors while removing active HTML and script URLs', async () => {
    getPublicGuide.mockResolvedValue({ version: 2, updated_at: '', has_custom_content: true, chapters: [{ slug: 'start', title: '接入教程', content: '## 接入教程\n\n正文\n\n<script>alert(1)</script>\n<img src="x" onerror="alert(1)">\n[bad](javascript:alert(1))' }] })
    const wrapper = mount(GuideView, options)
    await flushPromises()
    expect(wrapper.get('.guide-nav a').attributes('href')).toBe('#start')
    expect(wrapper.get('[data-testid="guide-content"]').text()).toContain('正文')
    expect(wrapper.find('article script').exists()).toBe(false)
    expect(wrapper.find('article img').attributes('onerror')).toBeUndefined()
    expect(wrapper.find('article a[href^="javascript:"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('offers retry after failed loading', async () => {
    getPublicGuide.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ version: 0, updated_at: '', has_custom_content: false })
    const wrapper = mount(GuideView, options)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="guide-content"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
