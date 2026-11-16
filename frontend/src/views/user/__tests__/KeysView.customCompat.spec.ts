import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import { keysAPI } from '@/api'
import type { ApiKey, Group, GroupPlatform } from '@/types'
import KeysView from '../KeysView.vue'

const {
  listKeys,
  updateKey,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  showError,
  showSuccess,
  copyToClipboard,
  isCurrentStep,
  nextStep,
  getMatrix,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  updateKey: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
  getMatrix: vi.fn(),
}))

const messages: Record<string, string> = {
  'common.actions': 'Actions',
  'common.name': 'Name',
  'common.refresh': 'Refresh',
  'common.status': 'Status',
  'keys.apiKey': 'API Key',
  'keys.allGroups': 'All Groups',
  'keys.allStatus': 'All Status',
  'keys.columnSettings': 'Column Settings',
  'keys.createKey': 'Create API Key',
  'keys.created': 'Created',
  'keys.expiresAt': 'Expires',
  'keys.group': 'Group',
  'keys.fallbackGroupShortLabel': 'Fallback',
  'keys.noFallbackGroup': 'No fallback group',
  'keys.selectFallbackGroup': 'Select fallback',
  'keys.clickToChangeFallbackGroup': 'Click to change fallback group',
  'keys.fallbackGroupChangedSuccess': 'Fallback group changed successfully',
  'keys.groupChangedFallbackClearedSuccess': 'Primary group changed and fallback cleared',
  'keys.id': 'ID',
  'keys.currentConcurrency': 'Current Concurrency',
  'keys.lastUsedAt': 'Last Used',
  'keys.lastUsedIP': 'Last Used IP',
  'keys.rateLimitColumn': 'Rate Limit',
  'keys.searchPlaceholder': 'Search name or key...',
  'keys.status.active': 'Active',
  'keys.status.expired': 'Expired',
  'keys.status.inactive': 'Inactive',
  'keys.status.quota_exhausted': 'Quota exhausted',
  'keys.usage': 'Usage',
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: vi.fn(),
    regenerate: vi.fn(),
    update: updateKey,
    delete: vi.fn(),
    toggleStatus: vi.fn(),
  },
  authAPI: {
    getPublicSettings,
  },
  usageAPI: {
    getDashboardApiKeysUsage,
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 1 } }),
}))

vi.mock('@/api/channelMonitorV2', () => ({ getMatrix }))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep,
    nextStep,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const createApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-tes...1234',
  name: 'test-key',
  group_id: null,
  fallback_group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-06-27T00:00:00Z',
  updated_at: '2026-06-27T00:00:00Z',
  current_concurrency: 3,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

const createGroup = (id: number, name: string, platform: GroupPlatform = 'openai'): Group =>
  ({
    id,
    name,
    platform,
    description: null,
    rate_multiplier: 1,
    subscription_type: 'standard',
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    status: 'active',
  }) as Group

const AppLayoutStub = {
  template: '<div><slot /></div>',
}

const TablePageLayoutStub = {
  template: `
    <div>
      <slot name="filters" />
      <slot name="actions" />
      <slot name="table" />
      <slot name="pagination" />
    </div>
  `,
}

const DataTableStub = {
  name: 'DataTable',
  props: { columns: Array, data: Array, selectedKeys: Array, selectable: Boolean, loading: Boolean },
  emits: ['sort', 'update:selectedKeys'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map((col) => col.key).join(',') }}</div>
      <div data-test="columns-meta">{{ JSON.stringify(columns.map((col) => ({ key: col.key, sortable: !!col.sortable }))) }}</div>
      <button data-test="sort-current-concurrency" @click="$emit('sort', 'current_concurrency', 'asc')">
        Sort Current Concurrency
      </button>
      <div v-for="row in data" :key="row.id">
        <div
          v-if="columns.some((col) => col.key === 'id')"
          data-test="key-id"
        >
          <slot name="cell-id" :value="row.id" :row="row" />
        </div>
        <div data-test="key-mask"><slot name="cell-key" :value="row.key" :row="row" /></div>
        <slot name="cell-name" :value="row.name" :row="row" />
        <div data-test="group-cell">
          <slot name="cell-group" :value="row.group" :row="row" />
        </div>
        <slot name="cell-actions" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
        <div data-test="usage">
          <slot name="cell-usage" :row="row" />
        </div>
        <div
          v-if="columns.some((col) => col.key === 'last_used_ip')"
          data-test="last-used-ip"
        >
          <slot name="cell-last_used_ip" :value="row.last_used_ip" :row="row" />
        </div>
      </div>
      <slot name="empty" />
    </div>
  `,
}

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'open-change'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"></select>',
}

const SearchInputStub = {
  name: 'SearchInput',
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
}

const PaginationStub = {
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: `
    <div>
      <button data-test="page-size-50" @click="$emit('update:pageSize', 50)">50</button>
      <button data-test="page-2" @click="$emit('update:page', 2)">Page 2</button>
    </div>
  `,
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const GroupBadgeStub = {
  props: ['name'],
  template: '<span data-test="group-badge">{{ name }}</span>',
}

const GroupOptionItemStub = {
  props: ['name'],
  template: '<span data-test="group-option-item">{{ name }}<slot name="status" /></span>',
}

const mountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: {
          props: ['show', 'title'],
          emits: ['close'],
          template: '<div v-if="show" role="dialog"><button data-test="close-dialog" @click="$emit(\'close\')">Close</button><slot /><slot name="footer" /></div>',
        },
        ConfirmDialog: true,
        TotpStepUpDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: IconStub,
        UseKeyModal: true,
        BulkEditKeysModal: true,
        TestKeyModal: true,
        EndpointPopover: true,
        GroupBadge: GroupBadgeStub,
        GroupOptionItem: GroupOptionItemStub,
        GroupChannelStatus: {
          props: ['rows', 'coverage', 'loading', 'unavailable'],
          template: '<span data-group-preview :data-unavailable="unavailable">{{ rows.map(row => row.group_id).join(",") }}</span>',
        },
        Teleport: true,
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

const visibleColumnKeys = (wrapper: VueWrapper) =>
  wrapper.get('[data-test="columns"]').text().split(',').filter(Boolean)

const visibleColumnMeta = (wrapper: VueWrapper): Array<{ key: string; sortable: boolean }> =>
  JSON.parse(wrapper.get('[data-test="columns-meta"]').text())

const getButtonByText = (wrapper: VueWrapper, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) {
    throw new Error(`Button not found: ${text}`)
  }
  return button
}

describe('user KeysView', () => {
  beforeEach(() => {
    localStorage.clear()

    listKeys.mockReset()
    updateKey.mockReset()
    vi.mocked(keysAPI.create).mockReset()
    vi.mocked(keysAPI.regenerate).mockReset()
    getPublicSettings.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()
    getMatrix.mockReset()
    getMatrix.mockResolvedValue({ items: [], coverage: null })

    listKeys.mockResolvedValue({
      items: [createApiKey()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getPublicSettings.mockResolvedValue({})
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailableGroups.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    updateKey.mockResolvedValue(createApiKey())
    isCurrentStep.mockReturnValue(false)
  })

  it.each([
    { initialStatus: 'quota_exhausted', status: 'active', formStatus: 'active' },
    { initialStatus: 'inactive', status: 'inactive', formStatus: 'inactive' },
    { initialStatus: 'active', status: 'active', formStatus: 'inactive' },
  ] as const)('syncs quota reset from $initialStatus to $status with form status $formStatus', async ({ initialStatus, status, formStatus }) => {
    const key: ApiKey = {
      ...createApiKey(), group_id: 1, quota: 10, quota_used: 10,
      status: initialStatus,
    }
    listKeys.mockResolvedValueOnce({ items: [key], total: 1, page: 1, page_size: 20, pages: 1 })
    updateKey.mockResolvedValue({ ...key, status, quota_used: 0 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await wrapper.get('[data-tour="key-form-name"]').setValue('Unsaved name')
    const statusSelect = wrapper.findAllComponents({ name: 'Select' })
      .find((select) => select.props('options').length === 2 &&
        select.props('options')[0].value === 'active')!
    statusSelect.vm.$emit('update:modelValue', 'inactive')
    await wrapper.get('button[title="keys.resetQuotaUsed"]').trigger('click')
    const confirmation = wrapper.findAllComponents({ name: 'ConfirmDialog' })
      .find((dialog) => dialog.props('title') === 'keys.resetQuotaTitle')!
    confirmation.vm.$emit('confirm')
    await flushPromises()

    expect(updateKey).toHaveBeenNthCalledWith(1, key.id, { reset_quota: true })
    expect(wrapper.findComponent({ name: 'DataTable' }).props('data')[0])
      .toMatchObject({ status, quota_used: 0 })
    expect(statusSelect.props('modelValue')).toBe(formStatus)
    expect((wrapper.get('[data-tour="key-form-name"]').element as HTMLInputElement).value)
      .toBe('Unsaved name')

    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenNthCalledWith(2, key.id, expect.objectContaining({ name: 'Unsaved name', status: formStatus }))
    wrapper.unmount()
  })


  it('shows a regenerated secret once and removes it when closed', async () => {
    const secret = 'sk-new-once-123456789abcdef'
    vi.mocked(keysAPI.regenerate).mockResolvedValue({ ...createApiKey(), key: secret, secret_available: true })
    copyToClipboard.mockResolvedValue(true)
    const wrapper = await mountView()
    expect(wrapper.get('[data-test="key-mask"]').text()).toBe('sk-tes...1234')
    expect(wrapper.get('[data-test="key-mask"]').find('button').exists()).toBe(false)
    await getButtonByText(wrapper, 'keys.regenerate').trigger('click')
    expect(keysAPI.regenerate).not.toHaveBeenCalled()
    const confirm = wrapper.findAllComponents({ name: 'ConfirmDialog' }).find(dialog => dialog.props('title') === 'keys.regenerate')!
    expect(confirm.props('show')).toBe(true)
    confirm.vm.$emit('confirm')
    await flushPromises()
    expect(keysAPI.regenerate).toHaveBeenCalledWith(1)
    expect(wrapper.get('[data-testid="one-time-key"]').text()).toBe(secret)
    await getButtonByText(wrapper, 'keys.copyToClipboard').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(secret, 'keys.copied')
    await getButtonByText(wrapper, 'keys.secretSaved').trigger('click')
    expect(wrapper.find('[data-testid="one-time-key"]').exists()).toBe(false)
    await getButtonByText(wrapper, 'keys.useKey').trigger('click')
    expect(wrapper.findComponent({ name: 'UseKeyModal' }).props('apiKey')).toBe('')
    expect(wrapper.html()).not.toContain(secret)
    expect(JSON.stringify(localStorage)).not.toContain(secret)
    wrapper.unmount()
  })

  it('does not treat an idempotent replay as a revealed secret', async () => {
    vi.mocked(keysAPI.regenerate).mockResolvedValue(createApiKey())
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'keys.regenerate').trigger('click')
    const confirm = wrapper.findAllComponents({ name: 'ConfirmDialog' }).find(dialog => dialog.props('title') === 'keys.regenerate')!
    confirm.vm.$emit('confirm')
    await flushPromises()
    expect(wrapper.find('[data-testid="one-time-key"]').exists()).toBe(false)
    expect(showError).toHaveBeenCalledWith('keys.secretUnavailable')
    wrapper.unmount()
  })

  it('uses the default API key columns with low-frequency columns hidden', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'group',
      'current_concurrency',
      'usage',
      'expires_at',
      'status',
      'created_at',
      'actions',
    ])
    expect(visibleColumnKeys(wrapper)).not.toContain('rate_limit')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_at')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_ip')
    expect(visibleColumnKeys(wrapper)).not.toContain('id')
  })

  it('opens testing for the selected key and refreshes usage after testing', async () => {
    const key = { ...createApiKey(), group_id: 10 }
    listKeys.mockResolvedValue({ items: [key], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = await mountView()
    await wrapper.get('[data-test="test-key-1"]').trigger('click')
    const dialog = wrapper.findComponent({ name: 'TestKeyModal' })
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('apiKey').id).toBe(1)
    const calls = listKeys.mock.calls.length
    dialog.vm.$emit('tested')
    await flushPromises()
    expect(listKeys).toHaveBeenCalledTimes(calls + 1)
    dialog.vm.$emit('close')
    await nextTick()
    expect(dialog.props('show')).toBe(false)
    wrapper.unmount()
  })

  it.each([
    { status: 'inactive', group_id: 10 },
    { status: 'quota_exhausted', group_id: 10 },
    { status: 'expired', group_id: 10 },
    { status: 'active', group_id: null },
  ])('disables testing unavailable keys: %o', async config => {
    listKeys.mockResolvedValue({ items: [{ ...createApiKey(), ...config }], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = await mountView()
    expect(wrapper.get('[data-test="test-key-1"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('opens bulk editing with only selected visible keys', async () => {
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    expect(table.props('selectable')).toBe(true)
    table.vm.$emit('update:selectedKeys', [1, 99])
    await nextTick()
    await wrapper.get('[data-test="bulk-edit-keys"]').trigger('click')
    const modal = wrapper.findComponent({ name: 'BulkEditKeysModal' })
    expect(modal.props('show')).toBe(true)
    expect(modal.props('selectedKeys').map((key: ApiKey) => key.id)).toEqual([1])
    wrapper.unmount()
  })

  it.each(['filter', 'page size', 'sort'])('clears selection on %s changes', async (change) => {
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    if (change === 'filter') {
      wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    } else if (change === 'page size') {
      await wrapper.get('[data-test="page-size-50"]').trigger('click')
    } else {
      table.vm.$emit('sort', 'created_at', 'asc')
    }
    await flushPromises()
    expect(table.props('selectedKeys')).toEqual([])
    expect(wrapper.find('[data-test="bulk-edit-keys"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('removes successful keys from the selection and refreshes the table', async () => {
    listKeys.mockResolvedValue({
      items: [createApiKey(), { ...createApiKey(), id: 2, name: 'Second' }],
      total: 2, pages: 1
    })
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1, 2])
    await nextTick()
    await wrapper.get('[data-test="bulk-edit-keys"]').trigger('click')
    wrapper.findComponent({ name: 'BulkEditKeysModal' }).vm.$emit('updated', [1])
    await flushPromises()
    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(table.props('selectedKeys')).toEqual([2])
    wrapper.unmount()
  })

  it('drops keys that are no longer visible after a refresh', async () => {
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    listKeys.mockResolvedValue({ items: [], total: 0, pages: 0 })
    await wrapper.get('button[title="Refresh"]').trigger('click')
    await flushPromises()
    expect(table.props('selectedKeys')).toEqual([])
    wrapper.unmount()
  })

  it('shows a hidden column when toggled and persists the preference', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Rate Limit').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('rate_limit')
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['id', 'last_used_at', 'last_used_ip'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('shows the API key ID column when toggled', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'ID').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('id')
    expect(wrapper.get('[data-test="key-id"]').text()).toBe('#1')
    expect(visibleColumnMeta(wrapper).find((column) => column.key === 'id')?.sortable).toBe(true)
  })

  it('shows the last used IP column when toggled', async () => {
    listKeys.mockResolvedValueOnce({
      items: [{ ...createApiKey(), last_used_ip: '203.0.113.10' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('last_used_ip')
    expect(wrapper.get('[data-test="last-used-ip"]').text()).toBe('203.0.113.10')
  })

  it('restores column preferences from localStorage on mount', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['group', 'created_at']))
    localStorage.setItem('api-key-column-settings-version', '1')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'current_concurrency',
      'usage',
      'rate_limit',
      'expires_at',
      'status',
      'last_used_at',
      'actions',
    ])
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['group', 'created_at', 'last_used_ip', 'id'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('does not include always-visible columns in the toggleable menu', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await nextTick()

    const columnMenuText = wrapper.text()
    expect(columnMenuText).toContain('API Key')
    expect(columnMenuText).toContain('ID')
    expect(columnMenuText).toContain('Current Concurrency')
    expect(columnMenuText).toContain('Rate Limit')
    expect(columnMenuText).toContain('Last Used IP')
    expect(columnMenuText).not.toContain('Name')
    expect(columnMenuText).not.toContain('Actions')
  })


  it('marks current concurrency as sortable', async () => {
    const wrapper = await mountView()

    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency'
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it.each([
    { key: 'current_concurrency', order: 'asc' },
    { key: 'group', order: 'asc' },
    { key: 'group', order: 'desc' },
  ] as const)('keeps filters and resets pagination and selection when sorting $key $order', async ({ key, order }) => {
    getAvailableGroups.mockResolvedValue([{ id: 42, name: 'OpenAI' }])
    const wrapper = await mountView()

    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await flushPromises()

    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()

    const selects = wrapper.findAllComponents({ name: 'Select' })
    await selects[0].vm.$emit('update:modelValue', 42)
    await flushPromises()
    await selects[1].vm.$emit('update:modelValue', 'active')
    await flushPromises()

    await wrapper.get('[data-test="page-2"]').trigger('click')
    await flushPromises()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    expect(table.props('selectedKeys')).toEqual([1])
    expect(visibleColumnMeta(wrapper).find((column) => column.key === key)?.sortable).toBe(true)
    listKeys.mockClear()

    table.vm.$emit('sort', key, order)
    await flushPromises()

    expect(table.props('selectedKeys')).toEqual([])
    expect(listKeys).toHaveBeenLastCalledWith(
      1,
      50,
      {
        search: 'target',
        status: 'active',
        group_id: 42,
        sort_by: key,
        sort_order: order,
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

    it('renders the key list without waiting for slow usage stats', async () => {
      let resolveUsage!: (value: { stats: Record<string, unknown> }) => void
      getDashboardApiKeysUsage.mockReturnValue(new Promise((resolve) => { resolveUsage = resolve }))
      const wrapper = await mountView()

      const table = wrapper.findComponent({ name: 'DataTable' })
      expect(table.props('loading')).toBe(false)
      expect(table.props('data')).toHaveLength(1)
      expect(wrapper.find('[data-test="usage-pending"]').exists()).toBe(true)

      resolveUsage({
        stats: { 1: { api_key_id: 1, today_actual_cost: 0.5, total_actual_cost: 12.25 } },
      })
      await flushPromises()
      expect(wrapper.find('[data-test="usage-pending"]').exists()).toBe(false)
      expect(wrapper.get('[data-test="usage"]').text()).toContain('$12.2500')
    })

    it('stops showing usage placeholders when the usage request fails', async () => {
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
      getDashboardApiKeysUsage.mockRejectedValue(new Error('timeout'))
      const wrapper = await mountView()

      expect(wrapper.find('[data-test="usage-pending"]').exists()).toBe(false)
      expect(wrapper.get('[data-test="usage"]').text()).toContain('$0.0000')
      consoleError.mockRestore()
    })
})
