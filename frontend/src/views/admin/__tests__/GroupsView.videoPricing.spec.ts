import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GroupsView from '@/views/admin/GroupsView.vue'
import VideoGroupPricingEditor from '@/components/admin/channel/VideoGroupPricingEditor.vue'
import { newVideoGroupPricing, videoPricingFromCatalog } from '@/components/admin/channel/videoGroupPricing'
import type { PricingFormEntry } from '@/components/admin/channel/types'
import type { VideoSupplierPrice } from '@/api/admin/videoUpstream'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), create: vi.fn(), update: vi.fn(), showError: vi.fn(), get: vi.fn(),
}))
vi.mock('@/api/admin/groups', () => ({ getSmartWS: vi.fn().mockResolvedValue({ enabled: false, revision: 0 }), setSmartWS: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: mocks.list, create: mocks.create, update: mocks.update,
      getModelAllowlistCandidates: vi.fn().mockResolvedValue([]),
      getUsageSummary: vi.fn().mockResolvedValue([]), getCapacitySummary: vi.fn().mockResolvedValue([]),
      getLiveCapability: vi.fn().mockResolvedValue({ supported: false }), getAll: vi.fn().mockResolvedValue([]),
      duplicate: vi.fn(), delete: vi.fn(), updateSortOrder: vi.fn(),
    },
    accounts: { list: vi.fn(), getById: vi.fn() },
  },
}))
vi.mock('@/api/admin/imageUpstream', () => ({ imageUpstreamAPI: { get: vi.fn().mockResolvedValue({ settings: { auto_sync: true, auto_import_account_models: true, markup: 0, interval_seconds: 60 }, catalog: { models: {} } }) } }))
vi.mock('@/api/admin/videoUpstream', () => ({ videoUpstreamAPI: { get: mocks.get } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: mocks.showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => ({ isCurrentStep: vi.fn(() => false), nextStep: vi.fn() }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const supplier: VideoSupplierPrice = {
  model: 'video-a', billing_mode: 'video', unit_price: 0.25, reference_per_second: 0,
  spec: {
    supported: true, min_seconds: 3, max_seconds: 10, resolutions: ['720p'], ratios: ['16:9'],
    min_images: 0, max_images: 1, max_videos: 0, max_audios: 0, max_reference_seconds: 0, face: false, generate_audio: false,
  },
}
const group = {
  id: 42, name: 'Video', platform: 'video', rate_multiplier: 1, rpm_limit: 0, is_exclusive: false,
  status: 'active', subscription_type: 'standard', daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null,
  account_count: 0, active_account_count: 0, rate_limited_account_count: 0, sort_order: 10,
  model_pricing: [], created_at: '2026-10-09T00:00:00Z', updated_at: '2026-10-09T00:00:00Z',
}
const SelectStub = defineComponent({
  props: ['modelValue', 'options'], emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
})
const wrappers: ReturnType<typeof mountView>[] = []
function mountView() {
  const wrapper = mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: defineComponent({ template: '<main><slot /></main>' }),
        TablePageLayout: defineComponent({ template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>' }),
        DataTable: defineComponent({ props: ['data'], template: '<div><div v-for="row in data" :key="row.id" data-testid="group-row"><slot name="cell-actions" :row="row" /></div></div>' }),
        BaseDialog: defineComponent({ props: { show: Boolean }, template: '<div v-if="show"><slot /><slot name="footer" /></div>' }),
        Select: SelectStub, RouterLink: true, Pagination: true, ConfirmDialog: true, EmptyState: true, PlatformIcon: true, Icon: true,
        GroupCapacityBadge: true, GroupRateMultipliersModal: true, GroupRPMOverridesModal: true, VueDraggable: true,
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}
async function openCreate() {
  const wrapper = mountView()
  await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === 'admin.groups.createGroup')!.trigger('click')
  await flushPromises()
  const platformSelect = wrapper.findAll('#create-group-form select').find(select => select.find('option[value="video"]').exists())!
  await platformSelect.setValue('video')
  await wrapper.get('#create-group-form input[type="text"]').setValue('Video')
  await flushPromises()
  return wrapper
}
async function openEdit(pricing: PricingFormEntry[] = []) {
  mocks.list.mockResolvedValue({ items: [{ ...group, model_pricing: pricing }], total: 1, page: 1, page_size: 20, pages: 1 })
  const wrapper = mountView()
  await flushPromises()
  await wrapper.get('[data-testid="group-row"]').findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
  await flushPromises()
  return wrapper
}

describe('GroupsView video pricing payloads', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    mocks.list.mockResolvedValue({ items: [group], total: 1, page: 1, page_size: 20, pages: 1 })
    mocks.create.mockResolvedValue(group)
    mocks.update.mockResolvedValue(group)
    mocks.get.mockResolvedValue({
      settings: { auto_sync: false, follow_increase: true, follow_decrease: false, markup: { output_per_second: 0, reference_per_second: 0, per_request: 0 } },
      catalog: { models: { 'video-a': supplier }, changes: [] },
    })
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

  it('creates a video group with an empty model pricing array', async () => {
    const wrapper = await openCreate()
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ platform: 'video', model_pricing: [] }))
  })

  it('saves removal of all video models as an empty array', async () => {
    const wrapper = await openEdit([{ ...newVideoGroupPricing(), models: ['fixed'], per_request_price: 0.5 }])
    await wrapper.get('[data-testid="video-pricing-remove"]').trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.update).toHaveBeenCalledWith(42, expect.objectContaining({ model_pricing: [] }))
  })

  it('blocks saving an added blank row', async () => {
    const wrapper = await openCreate()
    await wrapper.get('[data-testid="video-pricing-add"]').trigger('click')
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('videoModule.invalidModel')
    expect(mocks.create).not.toHaveBeenCalled()
  })

  it('creates an unsaved group with a catalog model using upstream source and null customer price', async () => {
    const wrapper = await openCreate()
    await wrapper.get('[data-testid="video-catalog-select"]').setValue(true)
    await wrapper.get('[data-testid="video-catalog-add"]').trigger('click')
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ model_pricing: [expect.objectContaining({ models: ['video-a'], per_request_price: null, video: expect.objectContaining({ price_source: 'upstream', auto_price: true }) })] }))
  })

  it('serializes manual zero and preserves legacy fixed rows and legacy video prices', async () => {
    const legacyVideo = videoPricingFromCatalog(supplier)!
    delete legacyVideo.video!.price_source
    legacyVideo.per_request_price = 0.8
    const legacyFixed = { ...newVideoGroupPricing(), models: ['fixed'], per_request_price: 0.4 }
    const wrapper = await openEdit([legacyVideo, legacyFixed])
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    let pricing = mocks.update.mock.calls[0][1].model_pricing
    expect(pricing[0]).toMatchObject({ per_request_price: 0.8, video: { price_source: 'upstream', auto_price: true } })
    expect(pricing[1]).toMatchObject({ per_request_price: 0.4 })
    expect(pricing[1].video).toBeUndefined()

    const next = await openEdit([videoPricingFromCatalog(supplier)!])
    await next.get('[data-testid="video-pricing-price"]').setValue('0')
    expect(next.findComponent(VideoGroupPricingEditor).props('modelValue')[0].video!.price_source).toBe('manual')
    await next.get('#edit-group-form').trigger('submit')
    await flushPromises()
    pricing = mocks.update.mock.calls.at(-1)![1].model_pricing
    expect(pricing[0]).toMatchObject({ per_request_price: 0, video: { price_source: 'manual', auto_price: false } })
  })

  it('saves a cleared manual price as upstream while retaining the cache for server recalculation', async () => {
    const entry = videoPricingFromCatalog(supplier)!
    entry.per_request_price = 0.8
    entry.video!.price_source = 'manual'
    entry.video!.auto_price = false
    const wrapper = await openEdit([entry])
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.update).toHaveBeenCalledWith(42, expect.objectContaining({ model_pricing: [expect.objectContaining({ per_request_price: 0.8, video: expect.objectContaining({ price_source: 'upstream', auto_price: true }) })] }))
  })

  it.each(['create', 'edit'])('preserves upstream source, null unit price and disabled periodic repricing in %s JSON', async mode => {
    const wrapper = mode === 'create' ? await openCreate() : await openEdit()
    await wrapper.get('[data-testid="video-catalog-select"]').setValue(true)
    await wrapper.get('[data-testid="video-catalog-add"]').trigger('click')
    await wrapper.get('[data-testid="video-auto-price"]').setValue(false)
    await wrapper.get(`#${mode}-group-form`).trigger('submit')
    await flushPromises()
    const payload = mode === 'create' ? mocks.create.mock.calls[0][0] : mocks.update.mock.calls[0][1]
    const wire = JSON.parse(JSON.stringify(payload))
    expect(wire.model_pricing[0]).toMatchObject({ per_request_price: null, video: { price_source: 'upstream', auto_price: false } })
    expect(Object.hasOwn(wire.model_pricing[0], 'per_request_price')).toBe(true)
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it.each(['create', 'edit'])('shows the backend rejection reason and preserves the unsaved %s form', async mode => {
    const wrapper = mode === 'create' ? await openCreate() : await openEdit()
    await wrapper.get('[data-testid="video-catalog-select"]').setValue(true)
    await wrapper.get('[data-testid="video-catalog-add"]').trigger('click')
    const reason = '型号 video-a 没有已核实的上游价格，请先读取价格或保留原定价'
    const api = mode === 'create' ? mocks.create : mocks.update
    api.mockRejectedValueOnce({ status: 400, message: reason })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    await wrapper.get(`#${mode}-group-form`).trigger('submit')
    await flushPromises()
    consoleError.mockRestore()
    expect(mocks.showError).toHaveBeenCalledWith(reason)
    expect(wrapper.find(`#${mode}-group-form`).exists()).toBe(true)
    expect(wrapper.findComponent(VideoGroupPricingEditor).props('modelValue')[0]).toMatchObject({ per_request_price: null, video: { price_source: 'upstream' } })
  })

  it('submits a uniform manual 720p price as both the unit price and the resolution price', async () => {
    const entry = videoPricingFromCatalog({ ...supplier, unit_price: 0.68, resolution_prices: { '720p': 0.68 } })!
    const wrapper = await openEdit([entry])
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('1.0')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    const wire = JSON.parse(JSON.stringify(mocks.update.mock.calls[0][1]))
    expect(wire.model_pricing[0]).toMatchObject({ per_request_price: 1, video: { price_source: 'manual', auto_price: false, resolution_prices: { '720p': 1 } } })
  })

  it.each(['create', 'edit'])('allows %s after clearing manual dimensions and then returning upstream', async mode => {
    const wrapper = mode === 'create' ? await openCreate() : await openEdit()
    await wrapper.get('[data-testid="video-catalog-select"]').setValue(true)
    await wrapper.get('[data-testid="video-catalog-add"]').trigger('click')
    await wrapper.get('[data-testid="video-pricing-price"]').setValue('1')
    await wrapper.get('[data-testid="video-reference-price"]').setValue('')
    await wrapper.get('[data-testid="video-resolution-720p"]').setValue('')
    await wrapper.get(`#${mode}-group-form`).trigger('submit')
    const api = mode === 'create' ? mocks.create : mocks.update
    expect(api).not.toHaveBeenCalled()
    expect(mocks.showError).toHaveBeenCalledWith('videoModule.invalidPrice')
    mocks.showError.mockClear()

    await wrapper.get('[data-testid="video-pricing-price"]').setValue('')
    expect(wrapper.find('[data-testid="video-reference-price"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="video-resolution-720p"]').exists()).toBe(false)
    await wrapper.get(`#${mode}-group-form`).trigger('submit')
    await flushPromises()
    expect(api).toHaveBeenCalledOnce()
    expect(mocks.showError).not.toHaveBeenCalled()
    const payload = mode === 'create' ? mocks.create.mock.calls[0][0] : mocks.update.mock.calls[0][1]
    const wire = JSON.parse(JSON.stringify(payload))
    expect(wire.model_pricing[0]).toMatchObject({ per_request_price: 1, video: { price_source: 'upstream', auto_price: true, reference_per_second: 0, resolution_prices: {} } })
  })
  it('omits manual image overrides when creating an OpenAI group', async () => {
    const wrapper = await openCreate()
    const platform = wrapper.findAll('#create-group-form select').find(select => select.find('option[value="openai"]').exists())!
    await platform.setValue('openai')
    await flushPromises()
    expect(wrapper.find('[data-testid="create-openai-image-model-prices"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="create-openai-image-price-"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="create-image-auto-pricing-notice"]').text()).toContain('上游与自动跟价')
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.create).toHaveBeenCalledOnce()
    expect(mocks.create.mock.calls[0][0]).not.toHaveProperty('image_model_prices')
  })

  it('preserves image adapter and automatic price metadata when editing a group', async () => {
    const image = { provider: 'suanli', upstream_model: 'Image-A', auto_price: true, cost: 0.08, applied_markup: 0.1, applied_prices: { '1K': 0.18, '2K': 0.18, '4K': 0.18 } }
    const pricing = { ...newVideoGroupPricing(), video: undefined, models: ['Image-A'], billing_mode: 'image', per_request_price: 0.18, image }
    mocks.list.mockResolvedValue({ items: [{ ...group, platform: 'openai', allow_image_generation: true, model_pricing: [pricing], image_model_prices: { 'image-a': image.applied_prices } }], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="group-row"]').findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="image-group-pricing"]').text()).toContain('0.08')
    expect(wrapper.find('[data-testid="edit-openai-image-model-prices"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="edit-openai-image-price-"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="edit-image-auto-pricing-notice"]').text()).toContain('上游与自动跟价')
    expect(wrapper.get('[data-testid="image-group-pricing"]').text()).toContain('无需在分组重复填写')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.update).toHaveBeenCalledWith(42, expect.objectContaining({ model_pricing: [expect.objectContaining({ image })] }))
    // The form's old image price snapshot must not overwrite a newer sync.
    expect(mocks.update.mock.calls[0][1]).not.toHaveProperty('image_model_prices')
    expect(JSON.parse(JSON.stringify(mocks.update.mock.calls[0][1]))).not.toHaveProperty('image_model_prices')
  })

})
