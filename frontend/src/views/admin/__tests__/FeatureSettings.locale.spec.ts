import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import type { Component } from 'vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import TieredRoutingView from '../TieredRoutingView.vue'
import TicketsAdminView from '../TicketsAdminView.vue'
import BillingView from '@/views/user/BillingView.vue'
import TicketsView from '@/views/user/TicketsView.vue'
import RiskControlView from '../RiskControlView.vue'

const { showError } = vi.hoisted(() => ({
  showError: vi.fn(),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  tieredRouting: { getSettings: vi.fn().mockResolvedValue({}) },
  tickets: { list: vi.fn().mockResolvedValue({ items: [] }), stats: vi.fn().mockResolvedValue({ open: 3 }) },
  riskControl: { getConfig: vi.fn().mockResolvedValue({ mode: 'off', enabled: false }), getStatus: vi.fn().mockResolvedValue({ mode: 'off' }), listLogs: vi.fn().mockResolvedValue({ items: [], total: 0 }) },
  groups: { getAll: vi.fn().mockResolvedValue([]) },
  proxies: { getAll: vi.fn().mockResolvedValue([]) },
  securityPolicy: { listBuiltin: vi.fn().mockResolvedValue({ count: 10 }), listKeywords: vi.fn().mockResolvedValue({ keywords: [] }) },
} }))
vi.mock('@/api/billing', () => ({ default: { statement: vi.fn().mockResolvedValue({ rows: [], requests: 0, cost: 0 }) } }))
vi.mock('@/api/tickets', () => ({ default: { list: vi.fn().mockResolvedValue({ items: [] }) } }))

function render(component: Component) {
  const missing: string[] = []
  const i18n = createI18n({ legacy: false, locale: 'zh', fallbackLocale: false, messages: { zh, en }, missing: (_locale, key) => { missing.push(key) } })
  const wrapper = mount(component, {
    global: {
      plugins: [i18n],
      stubs: {
        BaseDialog: { props: ['show', 'title'], template: '<section v-if="show" role="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></section>' },
        Icon: true,
        Toggle: { props: ['modelValue'], template: '<input type="checkbox" :checked="modelValue" />' },
        Select: { props: ['options', 'modelValue'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>' },
      },
    },
  })
  return { wrapper, missing, i18n }
}
afterEach(() => { expect(showError).not.toHaveBeenCalled(); vi.clearAllMocks() })

describe('new feature settings in Chinese', () => {
  it.each([
    { view: TicketsView, title: '工单支持', button: '新建工单', fields: ['主题', '问题描述', '发送', '取消'] },
  ])('renders $title and its settings without translation fallbacks', async ({ view, title, button, fields }) => {
    const { wrapper, missing } = render(view)
    await flushPromises()
    expect(wrapper.find('h1').text()).toBe(title)
    const opener = wrapper.findAll('button').find(node => node.text() === button)
    expect(opener).toBeDefined()
    await opener!.trigger('click')
    await flushPromises()
    for (const text of fields) expect(wrapper.text()).toContain(text)
    expect(missing).toEqual([])
    wrapper.unmount()
  })

  it.each([
    { view: TieredRoutingView, title: '分级路由', fields: ['启用分级路由', '高级用户编号', '高级用户最低余额（美元）', '优选池'] },
    { view: TicketsAdminView, title: '工单管理', fields: ['待处理 3', '已回复', '已关闭', '暂无工单'] },
    { view: BillingView, title: '账单', fields: ['请求数', '总费用', '开始日期', '结束日期', '导出 CSV'] },
  ])('renders $title with Chinese labels', async ({ view, title, fields }) => {
    const { wrapper, missing } = render(view)
    await flushPromises()
    expect(wrapper.find('h1').text()).toContain(title)
    for (const text of fields) expect(wrapper.text()).toContain(text)
    expect(missing).toEqual([])
    wrapper.unmount()
  })

  it('renders the security-policy word-list settings and category options in Chinese', async () => {
    const { wrapper, missing } = render(RiskControlView)
    await flushPromises()
    await wrapper.findAll('button').find(node => node.text() === '安全策略词包')!.trigger('click')
    await flushPromises()
    const dialog = wrapper.find('[role="dialog"]')
    for (const label of ['分组安全策略词包', '10 条内置敏感话题关键词', '范围', '指定分组', '分类', '自定义', '添加', '暂无自定义词']) {
      expect(dialog.text()).toContain(label)
    }
    expect(missing).toEqual([])
    wrapper.unmount()
  })
})
