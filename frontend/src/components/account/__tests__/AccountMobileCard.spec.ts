import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountMobileCard from '../AccountMobileCard.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const setViewport = (desktop: boolean) => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn((media: string) => ({
      matches: desktop, media, addEventListener: vi.fn(), removeEventListener: vi.fn()
    }))
  })
}

const columns: Column[] = ['select', 'name', 'id', 'platform_type', 'capacity', 'status', 'schedulable', 'today_stats', 'groups', 'usage', 'proxy', 'priority', 'scheduler_score', 'rate_multiplier', 'upstream_billing_rate', 'last_used_at', 'created_at', 'expires_at', 'notes', 'actions']
  .map(key => ({ key, label: key, sortable: key === 'name' }))
const row = { id: 7, name: 'BPS account', status: 'active', rate_multiplier: 0.65 }

describe('AccountMobileCard with DataTable cells', () => {
  beforeEach(() => { localStorage.clear(); setViewport(false) })

  it('renders every visible column exactly once and reuses interactive cells and IQ status order', async () => {
    const select = vi.fn()
    const edit = vi.fn()
    const probe = vi.fn()
    const openIQ = vi.fn()
    const original = structuredClone({ row, columns })
    const wrapper = mount(DataTable, {
      props: { columns, data: [row], rowKey: 'id', mobileCardGrid: true, serverSideSort: true },
      slots: {
        'mobile-card': ({ row: account, columns: visible, renderCell }) => h(AccountMobileCard, { account, columns: visible, renderCell, selected: true }),
        'cell-select': ({ row: account }) => h('input', { type: 'checkbox', 'aria-label': account.name, onChange: select }),
        'cell-name': ({ value }) => h('a', { href: 'https://example.com' }, value),
        'cell-status': () => h('div', [h('span', { 'data-test': 'status' }, '正常'), h('button', { 'data-test': 'iq', onClick: openIQ }, '21')]),
        'cell-upstream_billing_rate': () => h('button', { 'data-test': 'probe', onClick: probe }, '模型价表'),
        'cell-actions': ({ row: account }) => h('button', { 'data-test': 'edit', onClick: () => edit(account.id) }, '编辑')
      }
    })

    expect(wrapper.find('table').exists()).toBe(false)
    const card = wrapper.get('[data-testid="account-mobile-card"]')
    expect(card.classes()).toContain('is-selected')
    for (const column of columns) expect(card.findAll(`[data-field="${column.key}"]`)).toHaveLength(1)
    const status = card.get('[data-field="status"]')
    expect(status.findAll('span, button').map(item => item.text())).toEqual(['正常', '21'])
    expect(card.get('[data-field="upstream_billing_rate"]').classes()).toContain('account-mobile-section')
    expect(card.get('[data-field="usage"]').classes()).toContain('account-mobile-section')
    expect(wrapper.find('[data-test="mobile-sort-controls"]').exists()).toBe(true)
    await card.get('input').setValue(true)
    await card.get('[data-test="edit"]').trigger('click')
    await card.get('[data-test="probe"]').trigger('click')
    await card.get('[data-test="iq"]').trigger('click')
    expect(select).toHaveBeenCalledTimes(1)
    expect(edit).toHaveBeenCalledWith(7)
    expect(probe).toHaveBeenCalledTimes(1)
    expect(openIQ).toHaveBeenCalledTimes(1)
    expect({ row, columns }).toEqual(original)
    wrapper.unmount()
  })

  it('respects hidden columns, updates values without writes, and retains unknown future columns', async () => {
    const visible = columns.filter(column => !['rate_multiplier', 'today_stats', 'proxy'].includes(column.key))
    visible.push({ key: 'future_account_state', label: 'Future state' })
    const renderCell = vi.fn((key: string) => h('span', key))
    const wrapper = mount(AccountMobileCard, { props: { account: row, columns: visible, renderCell } })
    for (const hidden of ['rate_multiplier', 'today_stats', 'proxy']) {
      expect(wrapper.find(`[data-field="${hidden}"]`).exists()).toBe(false)
      expect(renderCell).not.toHaveBeenCalledWith(hidden)
    }
    expect(wrapper.get('[data-field="future_account_state"]').text()).toContain('Future state')
    await wrapper.setProps({ account: { id: row.id, status: 'error' }, selected: true })
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['is-error', 'is-selected']))
    expect(row.status).toBe('active')
    expect(wrapper.emitted()).toEqual({})
    wrapper.unmount()
  })

  it('keeps desktop table cells and server-side sorting unchanged when a mobile slot exists', async () => {
    setViewport(true)
    const wrapper = mount(DataTable, {
      props: { columns: [{ key: 'name', label: 'Name', sortable: true }], data: [row], mobileCardGrid: true, serverSideSort: true },
      slots: {
        'mobile-card': () => h('div', { 'data-test': 'mobile-only' }),
        'cell-name': ({ value }) => h('span', { 'data-test': 'desktop-name' }, value)
      }
    })
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.get('[data-test="desktop-name"]').text()).toBe(row.name)
    expect(wrapper.find('[data-test="mobile-only"]').exists()).toBe(false)
    await wrapper.get('th').trigger('click')
    expect(wrapper.emitted('sort')).toBeTruthy()
    wrapper.unmount()
  })

  it('preserves the default mobile fallback when no dedicated card slot is supplied', () => {
    const wrapper = mount(DataTable, {
      props: { columns: [{ key: 'name', label: 'Name' }], data: [row] },
      slots: { 'cell-name': '<span data-test="fallback-name">BPS account</span>' }
    })
    expect(wrapper.find('[data-testid="account-mobile-card"]').exists()).toBe(false)
    expect(wrapper.get('[data-field="name"]').text()).toBe('NameBPS account')
    expect(wrapper.get('[data-test="fallback-name"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
